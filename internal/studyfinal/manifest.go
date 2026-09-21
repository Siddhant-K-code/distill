package studyfinal

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"time"
)

type ReceiptManifest struct {
	SchemaVersion         string    `json:"schema_version"`
	AuthorizationSHA256   string    `json:"authorization_sha256"`
	ReceiptRegistrySHA256 string    `json:"receipt_registry_sha256"`
	CollectorID           string    `json:"collector_id"`
	CollectorPublicKeyHex string    `json:"collector_public_key_hex"`
	Receipts              []Receipt `json:"receipts"`
	StatementSHA256       string    `json:"statement_sha256"`
	SignatureHex          string    `json:"signature_hex"`
	ManifestSHA256        string    `json:"manifest_sha256"`
}

func receiptManifestStatement(manifest ReceiptManifest) (string, error) {
	manifest.StatementSHA256 = ""
	manifest.SignatureHex = ""
	manifest.ManifestSHA256 = ""
	return DigestDomain("receipt-manifest-statement", manifest)
}

func receiptManifestDigest(manifest ReceiptManifest) (string, error) {
	manifest.ManifestSHA256 = ""
	return DigestDomain("receipt-manifest", manifest)
}

func BuildSignedReceiptManifest(d Dataset, schedule []ScheduleEntry, registry ReceiptRegistry, session *AuthorizedSession, receipts []Receipt, ledger *ExecutionLedger, collectorID string, privateKey ed25519.PrivateKey) ([]byte, error) {
	if collectorID == "" || len(privateKey) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("independent collector identity and signing key required")
	}
	if err := ValidateReceiptRegistry(registry, d, schedule); err != nil {
		return nil, err
	}
	if err := session.validate(d, schedule, time.Now().UTC()); err != nil {
		return nil, err
	}
	authorization := session.authorization
	if err := validateReceiptBijection(d, schedule, receipts, authorization, ledger); err != nil {
		return nil, err
	}
	publicKey := privateKey.Public().(ed25519.PublicKey)
	if digestBytes(publicKey) != authorization.CollectorPublicKeySHA256 {
		return nil, fmt.Errorf("collector signing key is not authorization-bound")
	}
	manifest := ReceiptManifest{
		SchemaVersion: SchemaVersion + "/receipt-manifest", AuthorizationSHA256: authorization.AuthorizationSHA256,
		ReceiptRegistrySHA256: registry.RegistrySHA256, CollectorID: collectorID,
		CollectorPublicKeyHex: hex.EncodeToString(publicKey), Receipts: append([]Receipt(nil), receipts...),
	}
	statement, err := receiptManifestStatement(manifest)
	if err != nil {
		return nil, err
	}
	manifest.StatementSHA256 = statement
	manifest.SignatureHex = hex.EncodeToString(ed25519.Sign(privateKey, []byte(statement)))
	manifest.ManifestSHA256, _ = receiptManifestDigest(manifest)
	return canonicalJSON(manifest)
}

func ValidateReceiptManifest(data []byte, d Dataset, schedule []ScheduleEntry, registry ReceiptRegistry, session *AuthorizedSession, ledger *ExecutionLedger, trustedCollector ed25519.PublicKey) (ReceiptManifest, error) {
	if err := session.validate(d, schedule, time.Now().UTC()); err != nil {
		return ReceiptManifest{}, err
	}
	authorization := session.authorization
	var manifest ReceiptManifest
	if err := strictDecode(bytes.NewReader(data), &manifest); err != nil {
		return ReceiptManifest{}, err
	}
	canonical, err := canonicalJSON(manifest)
	if err != nil || !bytes.Equal(canonical, data) {
		return ReceiptManifest{}, fmt.Errorf("receipt manifest must use canonical encoding")
	}
	if manifest.SchemaVersion != SchemaVersion+"/receipt-manifest" ||
		manifest.AuthorizationSHA256 != authorization.AuthorizationSHA256 ||
		manifest.ReceiptRegistrySHA256 != registry.RegistrySHA256 || manifest.CollectorID == "" ||
		manifest.CollectorPublicKeyHex != hex.EncodeToString(trustedCollector) {
		return ReceiptManifest{}, fmt.Errorf("receipt manifest identity mismatch")
	}
	if digestBytes(trustedCollector) != authorization.CollectorPublicKeySHA256 {
		return ReceiptManifest{}, fmt.Errorf("collector key is not authorization-bound")
	}
	if err := ValidateReceiptRegistry(registry, d, schedule); err != nil {
		return ReceiptManifest{}, err
	}
	if err := validateReceiptBijection(d, schedule, manifest.Receipts, authorization, ledger); err != nil {
		return ReceiptManifest{}, err
	}
	statement, err := receiptManifestStatement(manifest)
	if err != nil || statement != manifest.StatementSHA256 {
		return ReceiptManifest{}, fmt.Errorf("receipt manifest statement mismatch")
	}
	signature, err := hex.DecodeString(manifest.SignatureHex)
	if err != nil || !ed25519.Verify(trustedCollector, []byte(statement), signature) {
		return ReceiptManifest{}, fmt.Errorf("receipt manifest signature invalid")
	}
	digest, err := receiptManifestDigest(manifest)
	if err != nil || digest != manifest.ManifestSHA256 {
		return ReceiptManifest{}, fmt.Errorf("receipt manifest digest mismatch")
	}
	return manifest, nil
}
