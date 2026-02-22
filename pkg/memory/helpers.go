package memory

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"math"
	"time"
)

// generateID creates a random 16-char hex ID with a time prefix for ordering.
func generateID() string {
	b := make([]byte, 12)
	// First 4 bytes: unix timestamp for natural ordering
	ts := uint32(time.Now().Unix())
	b[0] = byte(ts >> 24)
	b[1] = byte(ts >> 16)
	b[2] = byte(ts >> 8)
	b[3] = byte(ts)
	// Remaining 8 bytes: random
	_, _ = rand.Read(b[4:])
	return hex.EncodeToString(b)
}

// encodeEmbedding converts a float32 slice to a byte slice for SQLite BLOB storage.
func encodeEmbedding(emb []float32) []byte {
	if len(emb) == 0 {
		return nil
	}
	buf := make([]byte, len(emb)*4)
	for i, v := range emb {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(v))
	}
	return buf
}

// decodeEmbedding converts a byte slice back to a float32 slice.
func decodeEmbedding(buf []byte) []float32 {
	if len(buf) == 0 || len(buf)%4 != 0 {
		return nil
	}
	emb := make([]float32, len(buf)/4)
	for i := range emb {
		emb[i] = math.Float32frombits(binary.LittleEndian.Uint32(buf[i*4:]))
	}
	return emb
}

// estimateTokens returns a rough token count for a text string.
// Uses the same heuristic as pkg/compress: ~4 chars per token.
func estimateTokens(text string) int {
	return (len(text) + 3) / 4
}
