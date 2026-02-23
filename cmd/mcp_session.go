package cmd

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Siddhant-K-code/distill/pkg/session"
	"github.com/mark3labs/mcp-go/mcp"
)

func (m *MCPServer) handleCreateSession(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	sessionID, _ := args["session_id"].(string)
	maxTokens := 128000
	if v, ok := args["max_tokens"].(float64); ok && v > 0 {
		maxTokens = int(v)
	}

	sess, err := m.sessStore.Create(ctx, session.CreateRequest{
		SessionID: sessionID,
		MaxTokens: maxTokens,
	})
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("create session: %v", err)), nil
	}

	data, _ := json.MarshalIndent(sess, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

func (m *MCPServer) handlePushSession(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	sessionID, _ := args["session_id"].(string)
	if sessionID == "" {
		return mcp.NewToolResultError("session_id is required"), nil
	}

	role, _ := args["role"].(string)
	if role == "" {
		role = "tool"
	}

	content, _ := args["content"].(string)
	if content == "" {
		return mcp.NewToolResultError("content is required"), nil
	}

	source, _ := args["source"].(string)

	importance := 0.5
	if v, ok := args["importance"].(float64); ok && v > 0 {
		importance = v
	}

	result, err := m.sessStore.Push(ctx, session.PushRequest{
		SessionID: sessionID,
		Entries: []session.PushEntry{
			{
				Role:       role,
				Content:    content,
				Source:     source,
				Importance: importance,
			},
		},
	})
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("push: %v", err)), nil
	}

	data, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

func (m *MCPServer) handleSessionContext(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	sessionID, _ := args["session_id"].(string)
	if sessionID == "" {
		return mcp.NewToolResultError("session_id is required"), nil
	}

	maxTokens := 0
	if v, ok := args["max_tokens"].(float64); ok && v > 0 {
		maxTokens = int(v)
	}

	role, _ := args["role"].(string)

	result, err := m.sessStore.Context(ctx, session.ContextRequest{
		SessionID: sessionID,
		MaxTokens: maxTokens,
		Role:      role,
	})
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("context: %v", err)), nil
	}

	data, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

func (m *MCPServer) handleDeleteSession(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	sessionID, _ := args["session_id"].(string)
	if sessionID == "" {
		return mcp.NewToolResultError("session_id is required"), nil
	}

	result, err := m.sessStore.Delete(ctx, sessionID)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("delete: %v", err)), nil
	}

	data, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}
