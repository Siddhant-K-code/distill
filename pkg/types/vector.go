package types

// Vector represents a vector embedding optimized for Pinecone gRPC operations.
// Uses float32 exclusively to minimize memory footprint (50% savings vs float64).
type Vector struct {
	ID       string
	Values   []float32
	Metadata map[string]interface{}
}

// VectorBatch represents a batch of vectors for bulk operations.
// Pinecone optimal batch size is 100 vectors.
type VectorBatch struct {
	Vectors []Vector
}

// NewVector creates a new Vector with pre-allocated metadata map.
func NewVector(id string, values []float32) *Vector {
	return &Vector{
		ID:       id,
		Values:   values,
		Metadata: make(map[string]interface{}),
	}
}

// NewVectorWithMetadata creates a Vector with existing metadata.
func NewVectorWithMetadata(id string, values []float32, metadata map[string]interface{}) *Vector {
	return &Vector{
		ID:       id,
		Values:   values,
		Metadata: metadata,
	}
}

// Clone creates a deep copy of the vector.
func (v *Vector) Clone() *Vector {
	values := make([]float32, len(v.Values))
	copy(values, v.Values)

	metadata := make(map[string]interface{}, len(v.Metadata))
	for k, val := range v.Metadata {
		metadata[k] = val
	}

	return &Vector{
		ID:       v.ID,
		Values:   values,
		Metadata: metadata,
	}
}

// Dimension returns the dimensionality of the vector.
func (v *Vector) Dimension() int {
	return len(v.Values)
}

// DeduplicationResult holds the output of the deduplication process.
type DeduplicationResult struct {
	UniqueVectors    []Vector
	DuplicateCount   int
	TotalProcessed   int
	ClusterCount     int
	ProcessingTimeMs int64
}

// SavingsPercent calculates the percentage of duplicates found.
func (r *DeduplicationResult) SavingsPercent() float64 {
	if r.TotalProcessed == 0 {
		return 0
	}
	return float64(r.DuplicateCount) / float64(r.TotalProcessed) * 100
}

// IngestionStats tracks ingestion pipeline metrics.
type IngestionStats struct {
	TotalVectors     int64
	UploadedVectors  int64
	FailedVectors    int64
	BatchesProcessed int64
	RetryCount       int64
	DurationMs       int64
}

// SuccessRate returns the percentage of successfully uploaded vectors.
func (s *IngestionStats) SuccessRate() float64 {
	if s.TotalVectors == 0 {
		return 0
	}
	return float64(s.UploadedVectors) / float64(s.TotalVectors) * 100
}
