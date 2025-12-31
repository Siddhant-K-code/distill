package types

import "time"

// Chunk represents a retrieved document chunk with its embedding and relevance score.
type Chunk struct {
	// ID is the unique identifier in the vector database
	ID string

	// Text is the original text content of the chunk
	Text string

	// Embedding is the vector representation (float32 for memory efficiency)
	Embedding []float32

	// Score is the relevance score from the vector DB query (higher = more relevant)
	Score float32

	// Metadata contains additional key-value pairs
	Metadata map[string]interface{}

	// ClusterID is assigned during deduplication (-1 if not clustered)
	ClusterID int
}

// NewChunk creates a new Chunk with initialized fields.
func NewChunk(id, text string, embedding []float32, score float32) *Chunk {
	return &Chunk{
		ID:        id,
		Text:      text,
		Embedding: embedding,
		Score:     score,
		Metadata:  make(map[string]interface{}),
		ClusterID: -1,
	}
}

// Dimension returns the embedding dimensionality.
func (c *Chunk) Dimension() int {
	return len(c.Embedding)
}

// Clone creates a deep copy of the chunk.
func (c *Chunk) Clone() *Chunk {
	embedding := make([]float32, len(c.Embedding))
	copy(embedding, c.Embedding)

	metadata := make(map[string]interface{}, len(c.Metadata))
	for k, v := range c.Metadata {
		metadata[k] = v
	}

	return &Chunk{
		ID:        c.ID,
		Text:      c.Text,
		Embedding: embedding,
		Score:     c.Score,
		Metadata:  metadata,
		ClusterID: c.ClusterID,
	}
}

// RetrievalRequest represents a query to the vector database.
type RetrievalRequest struct {
	// Query is the text query (will be embedded if EmbeddingProvider is set)
	Query string

	// QueryEmbedding is the pre-computed query vector (optional if Query is set)
	QueryEmbedding []float32

	// TopK is the number of results to retrieve
	TopK int

	// Namespace is the vector DB namespace/collection
	Namespace string

	// Filter is metadata filter criteria
	Filter map[string]interface{}

	// IncludeEmbeddings requests embeddings in the response
	IncludeEmbeddings bool

	// IncludeMetadata requests metadata in the response
	IncludeMetadata bool
}

// RetrievalResult holds the output of a vector database query.
type RetrievalResult struct {
	// Chunks are the retrieved document chunks
	Chunks []Chunk

	// QueryEmbedding is the embedding used for the query
	QueryEmbedding []float32

	// TotalMatches is the total number of matches (may exceed len(Chunks))
	TotalMatches int

	// Latency is the query execution time
	Latency time.Duration
}

// Cluster represents a group of semantically similar chunks.
type Cluster struct {
	// ID is the cluster identifier
	ID int

	// Members are the chunks belonging to this cluster
	Members []Chunk

	// Centroid is the geometric center of the cluster
	Centroid []float32

	// Representative is the selected chunk to represent this cluster
	Representative *Chunk
}

// Size returns the number of members in the cluster.
func (c *Cluster) Size() int {
	return len(c.Members)
}

// ClusterResult holds the output of the clustering process.
type ClusterResult struct {
	// Clusters are the identified groups
	Clusters []Cluster

	// Representatives are the selected chunks (one per cluster)
	Representatives []Chunk

	// InputCount is the number of chunks before clustering
	InputCount int

	// ClusterCount is the number of clusters formed
	ClusterCount int

	// Latency is the clustering execution time
	Latency time.Duration
}

// ReductionPercent calculates the percentage of chunks removed.
func (r *ClusterResult) ReductionPercent() float64 {
	if r.InputCount == 0 {
		return 0
	}
	return float64(r.InputCount-len(r.Representatives)) / float64(r.InputCount) * 100
}

// BrokerResult holds the final output of the ContextLab broker.
type BrokerResult struct {
	// Chunks are the deduplicated, diverse chunks
	Chunks []Chunk

	// Stats contains processing statistics
	Stats BrokerStats
}

// BrokerStats tracks broker operation metrics.
type BrokerStats struct {
	// Retrieved is the number of chunks fetched from vector DB
	Retrieved int

	// Clustered is the number of clusters formed
	Clustered int

	// Returned is the number of chunks in final output
	Returned int

	// RetrievalLatency is time spent querying vector DB
	RetrievalLatency time.Duration

	// ClusteringLatency is time spent clustering
	ClusteringLatency time.Duration

	// TotalLatency is end-to-end processing time
	TotalLatency time.Duration
}
