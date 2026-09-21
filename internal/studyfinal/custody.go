package studyfinal

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"io"
	"sort"
	"time"
)

type GroundTruthRecord struct {
	ConditionID    string            `json:"condition_id"`
	Answers        map[string]string `json:"answers"`
	Disposition    string            `json:"disposition"`
	EvidenceSHA256 []string          `json:"evidence_sha256"`
}

type CommitmentRecord struct {
	ConditionID string    `json:"condition_id"`
	Commitment  string    `json:"commitment"`
	Custodian   string    `json:"custodian"`
	SealedAt    time.Time `json:"sealed_at"`
}

type CustodyPackage struct {
	Commitments []CommitmentRecord `json:"commitments"`
}

type encryptedLabel struct {
	conditionID string
	nonce       []byte
	ciphertext  []byte
}

type sealedPlaintext struct {
	GroundTruth     GroundTruthRecord `json:"ground_truth"`
	CommitmentNonce []byte            `json:"commitment_nonce"`
}

// CustodianVault intentionally has no exported fields and is never accepted by Runner.
type CustodianVault struct {
	key     []byte
	records []encryptedLabel
}

func SealLabels(labels []GroundTruthRecord, custodian string, randomSource io.Reader, sealedAt time.Time) (CustodyPackage, *CustodianVault, error) {
	if custodian == "" || sealedAt.IsZero() {
		return CustodyPackage{}, nil, fmt.Errorf("custodian and sealed time required")
	}
	if randomSource == nil {
		randomSource = rand.Reader
	}
	key := make([]byte, 32)
	if _, err := io.ReadFull(randomSource, key); err != nil {
		return CustodyPackage{}, nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return CustodyPackage{}, nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return CustodyPackage{}, nil, err
	}
	labels = append([]GroundTruthRecord(nil), labels...)
	sort.Slice(labels, func(i, j int) bool { return labels[i].ConditionID < labels[j].ConditionID })
	public := CustodyPackage{}
	vault := &CustodianVault{key: key}
	seen := map[string]bool{}
	seenCommitmentNonce, seenEncryptionNonce := map[string]bool{}, map[string]bool{}
	for _, label := range labels {
		if err := validateGroundTruth(label); err != nil {
			return CustodyPackage{}, nil, err
		}
		if seen[label.ConditionID] {
			return CustodyPackage{}, nil, fmt.Errorf("duplicate label condition")
		}
		commitmentNonce := make([]byte, 32)
		encryptionNonce := make([]byte, aead.NonceSize())
		if _, err := io.ReadFull(randomSource, commitmentNonce); err != nil {
			return CustodyPackage{}, nil, err
		}
		if _, err := io.ReadFull(randomSource, encryptionNonce); err != nil {
			return CustodyPackage{}, nil, err
		}
		if seenCommitmentNonce[string(commitmentNonce)] || seenEncryptionNonce[string(encryptionNonce)] {
			return CustodyPackage{}, nil, fmt.Errorf("random source repeated a custody nonce")
		}
		seenCommitmentNonce[string(commitmentNonce)], seenEncryptionNonce[string(encryptionNonce)] = true, true
		labelBytes, err := canonicalJSON(label)
		if err != nil {
			return CustodyPackage{}, nil, err
		}
		commitment := digestBytes(append(append([]byte("random-label-commitment-v1\x00"), commitmentNonce...), labelBytes...))
		plaintext, err := canonicalJSON(sealedPlaintext{GroundTruth: label, CommitmentNonce: commitmentNonce})
		if err != nil {
			return CustodyPackage{}, nil, err
		}
		ciphertext := aead.Seal(nil, encryptionNonce, plaintext, []byte(label.ConditionID))
		public.Commitments = append(public.Commitments, CommitmentRecord{
			ConditionID: label.ConditionID, Commitment: commitment, Custodian: custodian, SealedAt: sealedAt.UTC(),
		})
		vault.records = append(vault.records, encryptedLabel{conditionID: label.ConditionID, nonce: encryptionNonce, ciphertext: ciphertext})
		seen[label.ConditionID] = true
	}
	return public, vault, nil
}

func validateGroundTruth(label GroundTruthRecord) error {
	if label.ConditionID == "" || len(label.Answers) != len(frozenQuestions) {
		return fmt.Errorf("invalid ground-truth identity or answer count")
	}
	for _, q := range frozenQuestions {
		value, ok := label.Answers[q.ID]
		if !ok || !contains(q.Allowed, value) {
			return fmt.Errorf("invalid ground truth %s/%s", label.ConditionID, q.ID)
		}
	}
	if label.Disposition != "accept" && label.Disposition != "review" && label.Disposition != "reject" {
		return fmt.Errorf("invalid ground-truth disposition")
	}
	if len(label.EvidenceSHA256) == 0 {
		return fmt.Errorf("ground truth evidence digest required")
	}
	for _, digest := range label.EvidenceSHA256 {
		if len(digest) != 64 {
			return fmt.Errorf("invalid ground-truth evidence digest")
		}
	}
	return nil
}

func (v *CustodianVault) Release(gate *PhaseGate, public CustodyPackage, manifestBytes []byte, trustedCollector ed25519.PublicKey) ([]GroundTruthRecord, error) {
	if v == nil || gate == nil || len(v.key) != 32 || len(public.Commitments) != len(v.records) {
		return nil, fmt.Errorf("invalid custody release")
	}
	if err := ValidateCommitmentsAgainstCorpus(gate.dataset, public.Commitments); err != nil {
		return nil, err
	}
	if err := gate.authorizeLabelRelease(manifestBytes, trustedCollector); err != nil {
		return nil, err
	}
	defer func() {
		for i := range v.key {
			v.key[i] = 0
		}
		v.key, v.records = nil, nil
	}()
	block, err := aes.NewCipher(v.key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	commitments := map[string]CommitmentRecord{}
	for _, commitment := range public.Commitments {
		commitments[commitment.ConditionID] = commitment
	}
	labels := make([]GroundTruthRecord, 0, len(v.records))
	for _, record := range v.records {
		plaintext, err := aead.Open(nil, record.nonce, record.ciphertext, []byte(record.conditionID))
		if err != nil {
			return nil, fmt.Errorf("decrypt custodian label: %w", err)
		}
		var sealed sealedPlaintext
		if err := strictDecode(bytes.NewReader(plaintext), &sealed); err != nil {
			return nil, err
		}
		labelBytes, _ := canonicalJSON(sealed.GroundTruth)
		actual := digestBytes(append(append([]byte("random-label-commitment-v1\x00"), sealed.CommitmentNonce...), labelBytes...))
		expected, ok := commitments[record.conditionID]
		if !ok || expected.Commitment != actual {
			return nil, fmt.Errorf("label commitment mismatch")
		}
		labels = append(labels, sealed.GroundTruth)
	}
	return labels, nil
}

func ValidateCommitmentsAgainstCorpus(d Dataset, commitments []CommitmentRecord) error {
	if len(commitments) != len(d.Conditions) {
		return fmt.Errorf("expected %d label commitments, got %d", len(d.Conditions), len(commitments))
	}
	conditions := map[string]bool{}
	for _, condition := range d.Conditions {
		conditions[condition.ID] = true
	}
	seenID, seenCommitment := map[string]bool{}, map[string]bool{}
	for _, commitment := range commitments {
		if !conditions[commitment.ConditionID] || seenID[commitment.ConditionID] || seenCommitment[commitment.Commitment] ||
			len(commitment.Commitment) != 64 || commitment.Custodian == "" || commitment.SealedAt.IsZero() {
			return fmt.Errorf("invalid label commitment registry")
		}
		seenID[commitment.ConditionID], seenCommitment[commitment.Commitment] = true, true
	}
	return nil
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
