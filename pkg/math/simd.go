package math

import (
	"math"
)

// CosineDistance computes cosine distance between two float32 vectors.
// Returns a value in [0, 2] where 0 = identical, 2 = opposite.
// Optimized for float32 to minimize memory usage.
func CosineDistance(a, b []float32) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 2.0 // Maximum distance for empty input
	}
	if len(a) != len(b) {
		// Use shorter length
		if len(a) > len(b) {
			a = a[:len(b)]
		} else {
			b = b[:len(a)]
		}
	}

	// Compute dot product and magnitudes in a single pass
	var dot, magA, magB float64
	n := len(a)

	// Process 4 elements at a time for better CPU pipelining
	i := 0
	for ; i <= n-4; i += 4 {
		dot += float64(a[i])*float64(b[i]) +
			float64(a[i+1])*float64(b[i+1]) +
			float64(a[i+2])*float64(b[i+2]) +
			float64(a[i+3])*float64(b[i+3])

		magA += float64(a[i])*float64(a[i]) +
			float64(a[i+1])*float64(a[i+1]) +
			float64(a[i+2])*float64(a[i+2]) +
			float64(a[i+3])*float64(a[i+3])

		magB += float64(b[i])*float64(b[i]) +
			float64(b[i+1])*float64(b[i+1]) +
			float64(b[i+2])*float64(b[i+2]) +
			float64(b[i+3])*float64(b[i+3])
	}

	// Handle remaining elements
	for ; i < n; i++ {
		dot += float64(a[i]) * float64(b[i])
		magA += float64(a[i]) * float64(a[i])
		magB += float64(b[i]) * float64(b[i])
	}

	// Compute cosine similarity
	denom := math.Sqrt(magA * magB)
	if denom == 0 {
		return 2.0
	}

	similarity := dot / denom
	// Clamp to [-1, 1] to handle floating point errors
	if similarity > 1.0 {
		similarity = 1.0
	} else if similarity < -1.0 {
		similarity = -1.0
	}

	// Return distance (1 - similarity)
	return 1.0 - similarity
}

// CosineSimilarity computes cosine similarity (1 - distance).
// Returns a value in [-1, 1] where 1 = identical, -1 = opposite.
func CosineSimilarity(a, b []float32) float64 {
	return 1.0 - CosineDistance(a, b)
}

// EuclideanDistance computes L2 squared distance between two float32 vectors.
func EuclideanDistance(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return math.MaxFloat64
	}

	var sum float64
	n := len(a)

	// Process 4 elements at a time
	i := 0
	for ; i <= n-4; i += 4 {
		d0 := float64(a[i]) - float64(b[i])
		d1 := float64(a[i+1]) - float64(b[i+1])
		d2 := float64(a[i+2]) - float64(b[i+2])
		d3 := float64(a[i+3]) - float64(b[i+3])
		sum += d0*d0 + d1*d1 + d2*d2 + d3*d3
	}

	for ; i < n; i++ {
		d := float64(a[i]) - float64(b[i])
		sum += d * d
	}

	return sum
}

// DotProduct computes inner product between two float32 vectors.
func DotProduct(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var sum float64
	n := len(a)

	// Process 4 elements at a time
	i := 0
	for ; i <= n-4; i += 4 {
		sum += float64(a[i])*float64(b[i]) +
			float64(a[i+1])*float64(b[i+1]) +
			float64(a[i+2])*float64(b[i+2]) +
			float64(a[i+3])*float64(b[i+3])
	}

	for ; i < n; i++ {
		sum += float64(a[i]) * float64(b[i])
	}

	return sum
}

// NormalizeInPlace normalizes a vector to unit length in-place.
// Uses SIMD for dot product calculation.
func NormalizeInPlace(v []float32) {
	if len(v) == 0 {
		return
	}

	// Compute magnitude using SIMD dot product
	mag := math.Sqrt(DotProduct(v, v))
	if mag == 0 {
		return
	}

	invMag := float32(1.0 / mag)
	for i := range v {
		v[i] *= invMag
	}
}

// AddVectors adds two vectors element-wise, storing result in dst.
// dst must be pre-allocated with sufficient capacity.
func AddVectors(dst, a, b []float32) {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	if len(dst) < n {
		return
	}

	for i := 0; i < n; i++ {
		dst[i] = a[i] + b[i]
	}
}

// ScaleVector multiplies all elements by a scalar in-place.
func ScaleVector(v []float32, scalar float32) {
	for i := range v {
		v[i] *= scalar
	}
}

// ZeroVector fills a vector with zeros.
func ZeroVector(v []float32) {
	for i := range v {
		v[i] = 0
	}
}

// CopyVector copies src to dst.
func CopyVector(dst, src []float32) {
	copy(dst, src)
}

// MeanVector computes the element-wise mean of multiple vectors.
// Result is stored in dst which must be pre-allocated.
func MeanVector(dst []float32, vectors [][]float32) {
	if len(vectors) == 0 || len(dst) == 0 {
		return
	}

	ZeroVector(dst)

	for _, v := range vectors {
		for i := 0; i < len(dst) && i < len(v); i++ {
			dst[i] += v[i]
		}
	}

	invN := float32(1.0 / float64(len(vectors)))
	ScaleVector(dst, invN)
}
