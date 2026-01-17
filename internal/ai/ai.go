// Package ai provides LLM integration for the chat CLI.
package ai

import (
	"context"
)

// Provider is an LLM backend.
type Provider interface {
	// Chat sends a message and returns the response.
	Chat(ctx context.Context, messages []Message, tools []Tool) (*Response, error)
}

// Message is a chat message.
type Message struct {
	Role    string `json:"role"` // "user", "assistant", "system"
	Content string `json:"content"`
}

// Tool is a function the LLM can call.
type Tool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"` // JSON Schema
}

// ToolCall is an LLM request to call a tool.
type ToolCall struct {
	Name       string         `json:"name"`
	Parameters map[string]any `json:"parameters"`
}

// Response is an LLM response.
type Response struct {
	Content   string     `json:"content,omitempty"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

// OllamaProvider uses Ollama for local inference.
type OllamaProvider struct {
	baseURL string
	model   string
}

// NewOllamaProvider creates a new Ollama provider.
func NewOllamaProvider(baseURL, model string) *OllamaProvider {
	return &OllamaProvider{
		baseURL: baseURL,
		model:   model,
	}
}

// Chat sends a message to Ollama.
func (o *OllamaProvider) Chat(ctx context.Context, messages []Message, tools []Tool) (*Response, error) {
	// TODO: implement Ollama API call with tool support
	return &Response{Content: "Ollama chat not yet implemented"}, nil
}

// AnthropicProvider uses Claude API.
type AnthropicProvider struct {
	apiKey string
	model  string
}

// NewAnthropicProvider creates a new Anthropic provider.
func NewAnthropicProvider(apiKey, model string) *AnthropicProvider {
	return &AnthropicProvider{
		apiKey: apiKey,
		model:  model,
	}
}

// Chat sends a message to Claude.
func (a *AnthropicProvider) Chat(ctx context.Context, messages []Message, tools []Tool) (*Response, error) {
	// TODO: implement Anthropic API call with tool use
	return &Response{Content: "Anthropic chat not yet implemented"}, nil
}

// Tools available to the AI assistant.
var Tools = []Tool{
	{
		Name:        "get_status",
		Description: "Get system status including download queue, disk space, and service health",
		Parameters:  map[string]any{"type": "object", "properties": map[string]any{}},
	},
	{
		Name:        "search_content",
		Description: "Search for a movie or TV show across indexers",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string", "description": "Search query"},
				"type":  map[string]any{"type": "string", "enum": []string{"movie", "series"}},
			},
			"required": []string{"query"},
		},
	},
	{
		Name:        "list_downloads",
		Description: "List active and recent downloads",
		Parameters:  map[string]any{"type": "object", "properties": map[string]any{}},
	},
	{
		Name:        "diagnose_issue",
		Description: "Investigate why a download is stuck, failed, or not importing",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"download_id": map[string]any{"type": "integer"},
				"problem":     map[string]any{"type": "string"},
			},
		},
	},
	{
		Name:        "grab_release",
		Description: "Download a specific release",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"content_id": map[string]any{"type": "integer"},
				"release_id": map[string]any{"type": "string"},
			},
			"required": []string{"content_id", "release_id"},
		},
	},
}
