package compress

import (
	"context"
	"strings"
	"testing"

	"github.com/Siddhant-K-code/distill/pkg/types"
)

func BenchmarkCompress_ShortText(b *testing.B) {
	c := NewExtractiveCompressor()
	ctx := context.Background()
	chunk := types.Chunk{ID: "bench", Text: "This is a short text for compression benchmarking."}
	opts := DefaultOptions()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = c.Compress(ctx, []types.Chunk{chunk}, opts)
	}
}

func BenchmarkCompress_LongText(b *testing.B) {
	c := NewExtractiveCompressor()
	ctx := context.Background()
	long := strings.Repeat("This is a longer text with more content for compression benchmarking. ", 50)
	chunk := types.Chunk{ID: "bench", Text: long}
	opts := DefaultOptions()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = c.Compress(ctx, []types.Chunk{chunk}, opts)
	}
}
