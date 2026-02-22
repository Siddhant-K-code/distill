package cmd

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Siddhant-K-code/distill/pkg/memory"
	"github.com/mark3labs/mcp-go/mcp"
)

func (m *MCPServer) handleStoreMemory(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	text, _ := args["text"].(string)
	if text == "" {
		return mcp.NewToolResultError("text is required"), nil
	}

	source, _ := args["source"].(string)
	sessionID, _ := args["session_id"].(string)

	var tags []string
	if tagsRaw, ok := args["tags"].([]interface{}); ok {
		for _, t := range tagsRaw {
			if s, ok := t.(string); ok {
				tags = append(tags, s)
			}
		}
	}

	entry := memory.StoreEntry{
		Text:   text,
		Source: source,
		Tags:   tags,
	}

	if m.embedder != nil {
		emb, err := m.embedder.Embed(ctx, text)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("embedding error: %v", err)), nil
		}
		entry.Embedding = emb
	}

	result, err := m.memStore.Store(ctx, memory.StoreRequest{
		SessionID: sessionID,
		Entries:   []memory.StoreEntry{entry},
	})
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("store error: %v", err)), nil
	}

	out, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(out)), nil
}

func (m *MCPServer) handleRecallMemory(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	query, _ := args["query"].(string)
	if query == "" {
		return mcp.NewToolResultError("query is required"), nil
	}

	var tags []string
	if tagsRaw, ok := args["tags"].([]interface{}); ok {
		for _, t := range tagsRaw {
			if s, ok := t.(string); ok {
				tags = append(tags, s)
			}
		}
	}

	maxResults := 10
	if v, ok := args["max_results"].(float64); ok && v > 0 {
		maxResults = int(v)
	}

	maxTokens := 0
	if v, ok := args["max_tokens"].(float64); ok && v > 0 {
		maxTokens = int(v)
	}

	req := memory.RecallRequest{
		Query:         query,
		Tags:          tags,
		MaxResults:    maxResults,
		MaxTokens:     maxTokens,
		RecencyWeight: 0.3,
	}

	if m.embedder != nil {
		emb, err := m.embedder.Embed(ctx, query)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("embedding error: %v", err)), nil
		}
		req.QueryEmbedding = emb
	}

	result, err := m.memStore.Recall(ctx, req)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("recall error: %v", err)), nil
	}

	out, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(out)), nil
}

func (m *MCPServer) handleForgetMemory(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	var ids []string
	if idsRaw, ok := args["ids"].([]interface{}); ok {
		for _, id := range idsRaw {
			if s, ok := id.(string); ok {
				ids = append(ids, s)
			}
		}
	}

	var tags []string
	if tagsRaw, ok := args["tags"].([]interface{}); ok {
		for _, t := range tagsRaw {
			if s, ok := t.(string); ok {
				tags = append(tags, s)
			}
		}
	}

	if len(ids) == 0 && len(tags) == 0 {
		return mcp.NewToolResultError("at least one of ids or tags is required"), nil
	}

	result, err := m.memStore.Forget(ctx, memory.ForgetRequest{
		IDs:  ids,
		Tags: tags,
	})
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("forget error: %v", err)), nil
	}

	out, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(out)), nil
}

func (m *MCPServer) handleMemoryStats(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	stats, err := m.memStore.Stats(ctx)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("stats error: %v", err)), nil
	}

	out, _ := json.MarshalIndent(stats, "", "  ")
	return mcp.NewToolResultText(string(out)), nil
}
