package lock

import "github.com/Siddhant-K-code/distill/internal/artifact"

func canonicalJSON(value any) ([]byte, error) {
	return artifact.CanonicalJSON(value)
}

func decodeCanonicalJSON(data []byte, value any) error {
	return artifact.DecodeCanonicalJSON(data, value)
}

func digestBytes(data []byte) string {
	return artifact.DigestBytes(data)
}

func validDigest(value string) bool {
	return artifact.ValidDigest(value)
}
