package qdrant

import (
	"context"
	"crypto/tls"
	"fmt"
	"time"

	"github.com/Siddhant-K-code/distill/pkg/retriever"
	"github.com/Siddhant-K-code/distill/pkg/types"
	pb "github.com/qdrant/go-client/qdrant"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

// Client implements the Retriever interface for Qdrant.
type Client struct {
	cfg        Config
	conn       *grpc.ClientConn
	points     pb.PointsClient
	collection string
}

// Config holds Qdrant-specific configuration.
type Config struct {
	retriever.Config

	// Collection is the Qdrant collection to query
	Collection string

	// UseTLS enables TLS for the connection
	UseTLS bool

	// GRPCPort is the gRPC port (default: 6334)
	GRPCPort int
}

// NewClient creates a new Qdrant retriever client.
func NewClient(ctx context.Context, cfg Config) (*Client, error) {
	if cfg.Host == "" {
		return nil, fmt.Errorf("host is required")
	}
	if cfg.Collection == "" {
		return nil, fmt.Errorf("collection is required")
	}

	// Apply defaults
	if cfg.TimeoutSeconds <= 0 {
		cfg.TimeoutSeconds = 30
	}
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = 3
	}
	if cfg.GRPCPort <= 0 {
		cfg.GRPCPort = 6334
	}

	// Build connection options
	var opts []grpc.DialOption

	if cfg.UseTLS {
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{})))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	// Connect to Qdrant
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.GRPCPort)
	conn, err := grpc.DialContext(ctx, addr, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Qdrant at %s: %w", addr, err)
	}

	return &Client{
		cfg:        cfg,
		conn:       conn,
		points:     pb.NewPointsClient(conn),
		collection: cfg.Collection,
	}, nil
}

// Query retrieves chunks similar to the given embedding.
func (c *Client) Query(ctx context.Context, req *types.RetrievalRequest) (*types.RetrievalResult, error) {
	if len(req.QueryEmbedding) == 0 {
		return nil, retriever.ErrInvalidQuery
	}

	start := time.Now()

	topK := req.TopK
	if topK <= 0 {
		topK = 10
	}

	// Add API key to context if provided
	if c.cfg.APIKey != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, "api-key", c.cfg.APIKey)
	}

	// Convert float32 to float64 for Qdrant
	vector := make([]float32, len(req.QueryEmbedding))
	copy(vector, req.QueryEmbedding)

	// Build search request
	searchReq := &pb.SearchPoints{
		CollectionName: c.collection,
		Vector:         vector,
		Limit:          uint64(topK),
		WithPayload: &pb.WithPayloadSelector{
			SelectorOptions: &pb.WithPayloadSelector_Enable{Enable: req.IncludeMetadata},
		},
		WithVectors: &pb.WithVectorsSelector{
			SelectorOptions: &pb.WithVectorsSelector_Enable{Enable: req.IncludeEmbeddings},
		},
	}

	// Add filter if provided
	if len(req.Filter) > 0 {
		searchReq.Filter = buildFilter(req.Filter)
	}

	// Execute search
	resp, err := c.points.Search(ctx, searchReq)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	// Convert response to chunks
	chunks := make([]types.Chunk, 0, len(resp.Result))
	for _, point := range resp.Result {
		chunk := types.Chunk{
			Score:     point.Score,
			ClusterID: -1,
		}

		// Extract ID
		if point.Id != nil {
			switch id := point.Id.PointIdOptions.(type) {
			case *pb.PointId_Num:
				chunk.ID = fmt.Sprintf("%d", id.Num)
			case *pb.PointId_Uuid:
				chunk.ID = id.Uuid
			}
		}

		// Extract embedding if included
		if point.Vectors != nil {
			if vec := point.Vectors.GetVector(); vec != nil {
				chunk.Embedding = vec.Data
			}
		}

		// Extract payload/metadata
		if point.Payload != nil {
			chunk.Metadata = convertPayloadToMap(point.Payload)

			// Try to extract text from common fields
			if text, ok := chunk.Metadata["text"].(string); ok {
				chunk.Text = text
			} else if text, ok := chunk.Metadata["content"].(string); ok {
				chunk.Text = text
			} else if text, ok := chunk.Metadata["chunk_text"].(string); ok {
				chunk.Text = text
			}
		}

		chunks = append(chunks, chunk)
	}

	return &types.RetrievalResult{
		Chunks:         chunks,
		QueryEmbedding: req.QueryEmbedding,
		TotalMatches:   len(chunks),
		Latency:        time.Since(start),
	}, nil
}

// QueryByID retrieves chunks similar to an existing vector by its ID.
func (c *Client) QueryByID(ctx context.Context, id string, topK int, namespace string) (*types.RetrievalResult, error) {
	start := time.Now()

	if topK <= 0 {
		topK = 10
	}

	// Add API key to context if provided
	if c.cfg.APIKey != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, "api-key", c.cfg.APIKey)
	}

	// First, fetch the vector by ID
	getReq := &pb.GetPoints{
		CollectionName: c.collection,
		Ids: []*pb.PointId{
			{PointIdOptions: &pb.PointId_Uuid{Uuid: id}},
		},
		WithPayload: &pb.WithPayloadSelector{
			SelectorOptions: &pb.WithPayloadSelector_Enable{Enable: true},
		},
		WithVectors: &pb.WithVectorsSelector{
			SelectorOptions: &pb.WithVectorsSelector_Enable{Enable: true},
		},
	}

	getResp, err := c.points.Get(ctx, getReq)
	if err != nil {
		return nil, fmt.Errorf("get point failed: %w", err)
	}

	if len(getResp.Result) == 0 {
		return nil, retriever.ErrNotFound
	}

	// Extract the vector
	point := getResp.Result[0]
	var vector []float32
	if point.Vectors != nil {
		if vec := point.Vectors.GetVector(); vec != nil {
			vector = vec.Data
		}
	}

	if len(vector) == 0 {
		return nil, fmt.Errorf("point %s has no vector", id)
	}

	// Now search using this vector
	req := &types.RetrievalRequest{
		QueryEmbedding:    vector,
		TopK:              topK,
		Namespace:         namespace,
		IncludeEmbeddings: true,
		IncludeMetadata:   true,
	}

	result, err := c.Query(ctx, req)
	if err != nil {
		return nil, err
	}

	result.Latency = time.Since(start)
	return result, nil
}

// Close releases resources.
func (c *Client) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// buildFilter converts a map to Qdrant filter.
func buildFilter(filter map[string]interface{}) *pb.Filter {
	if len(filter) == 0 {
		return nil
	}

	conditions := make([]*pb.Condition, 0, len(filter))

	for key, value := range filter {
		var condition *pb.Condition

		switch v := value.(type) {
		case string:
			condition = &pb.Condition{
				ConditionOneOf: &pb.Condition_Field{
					Field: &pb.FieldCondition{
						Key: key,
						Match: &pb.Match{
							MatchValue: &pb.Match_Keyword{Keyword: v},
						},
					},
				},
			}
		case int, int64:
			var intVal int64
			switch iv := v.(type) {
			case int:
				intVal = int64(iv)
			case int64:
				intVal = iv
			}
			condition = &pb.Condition{
				ConditionOneOf: &pb.Condition_Field{
					Field: &pb.FieldCondition{
						Key: key,
						Match: &pb.Match{
							MatchValue: &pb.Match_Integer{Integer: intVal},
						},
					},
				},
			}
		case bool:
			condition = &pb.Condition{
				ConditionOneOf: &pb.Condition_Field{
					Field: &pb.FieldCondition{
						Key: key,
						Match: &pb.Match{
							MatchValue: &pb.Match_Boolean{Boolean: v},
						},
					},
				},
			}
		}

		if condition != nil {
			conditions = append(conditions, condition)
		}
	}

	if len(conditions) == 0 {
		return nil
	}

	return &pb.Filter{
		Must: conditions,
	}
}

// convertPayloadToMap converts Qdrant payload to a Go map.
func convertPayloadToMap(payload map[string]*pb.Value) map[string]interface{} {
	if payload == nil {
		return nil
	}

	result := make(map[string]interface{}, len(payload))
	for k, v := range payload {
		result[k] = convertQdrantValue(v)
	}
	return result
}

// convertQdrantValue converts a Qdrant Value to a Go interface{}.
func convertQdrantValue(v *pb.Value) interface{} {
	if v == nil {
		return nil
	}

	switch val := v.Kind.(type) {
	case *pb.Value_NullValue:
		return nil
	case *pb.Value_DoubleValue:
		return val.DoubleValue
	case *pb.Value_IntegerValue:
		return val.IntegerValue
	case *pb.Value_StringValue:
		return val.StringValue
	case *pb.Value_BoolValue:
		return val.BoolValue
	case *pb.Value_ListValue:
		if val.ListValue == nil {
			return nil
		}
		list := make([]interface{}, len(val.ListValue.Values))
		for i, item := range val.ListValue.Values {
			list[i] = convertQdrantValue(item)
		}
		return list
	case *pb.Value_StructValue:
		if val.StructValue == nil {
			return nil
		}
		return convertPayloadToMap(val.StructValue.Fields)
	default:
		return nil
	}
}
