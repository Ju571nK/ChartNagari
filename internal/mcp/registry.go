// Package mcp implements the ChartNagari MCP (Model Context Protocol) server
// tool layer. It is transport-agnostic — consumed both by the HTTP streamable
// endpoint in internal/api/mcp_handler.go and (indirectly, via HTTP) by the
// stdio bridge binary in cmd/chartnagari-mcp/.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
)

// JSON-RPC 2.0 standard error codes (MCP spec).
const (
	ErrCodeParseError     = -32700
	ErrCodeInvalidRequest = -32600
	ErrCodeMethodNotFound = -32601
	ErrCodeInvalidParams  = -32602
	ErrCodeInternalError  = -32603
)

// Error is the JSON-RPC 2.0 error envelope surfaced by Dispatch. Tool handlers
// may return this type to set a specific code; other errors are wrapped as
// ErrCodeInternalError by the caller (HTTP handler).
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func (e *Error) Error() string { return fmt.Sprintf("mcp error %d: %s", e.Code, e.Message) }

// NewInvalidParams returns an ErrCodeInvalidParams error with an optional hint
// to help the LLM self-recover.
func NewInvalidParams(msg, hint string) *Error {
	e := &Error{Code: ErrCodeInvalidParams, Message: msg}
	if hint != "" {
		e.Data = map[string]string{"hint": hint}
	}
	return e
}

// ToolContent is one content item in a MCP tool result. Per spec, type is
// "text" or "resource"; we only use "text".
type ToolContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// ToolResult is the envelope returned from a successful tool invocation.
type ToolResult struct {
	Content []ToolContent `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

// TextResult wraps a single text payload in a ToolResult.
func TextResult(text string) ToolResult {
	return ToolResult{Content: []ToolContent{{Type: "text", Text: text}}}
}

// Tool is the interface each registered tool implements.
type Tool interface {
	Name() string        // e.g. "get_analysis"
	Description() string // LLM-facing description
	InputSchema() string // JSON schema (string, served verbatim)
	Call(ctx context.Context, params json.RawMessage) (ToolResult, error)
}

// ToolDescriptor is the shape returned by Registry.List (used by tools/list).
type ToolDescriptor struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// Registry holds the installed tools.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

// Register installs a tool. If a tool with the same name already exists, the
// older one is replaced (tests replace fakes freely).
func (r *Registry) Register(t Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[t.Name()] = t
}

// List returns descriptors for all registered tools, sorted by name (stable
// for tests and for client-side UIs).
func (r *Registry) List() []ToolDescriptor {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.tools))
	for n := range r.tools {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]ToolDescriptor, 0, len(names))
	for _, n := range names {
		t := r.tools[n]
		out = append(out, ToolDescriptor{
			Name:        n,
			Description: t.Description(),
			InputSchema: json.RawMessage(t.InputSchema()),
		})
	}
	return out
}

// Dispatch invokes the named tool with the given raw params.
func (r *Registry) Dispatch(ctx context.Context, name string, params json.RawMessage) (ToolResult, error) {
	r.mu.RLock()
	t, ok := r.tools[name]
	r.mu.RUnlock()
	if !ok {
		return ToolResult{}, &Error{
			Code:    ErrCodeMethodNotFound,
			Message: fmt.Sprintf("unknown tool: %s", name),
		}
	}
	return t.Call(ctx, params)
}

// ── JSON-RPC 2.0 envelope types ───────────────────────────────────────────

// RPCRequest is a JSON-RPC 2.0 request envelope.
// `ID` is kept as RawMessage to preserve the caller's type (number or string)
// on roundtrip. A missing `id` means the request is a notification (no response).
type RPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// RPCResponse is a JSON-RPC 2.0 response envelope. Exactly one of Result or
// Error must be set; the other is encoded as omitempty.
type RPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}

// NewErrorResponse constructs an error response envelope.
func NewErrorResponse(id json.RawMessage, err *Error) RPCResponse {
	return RPCResponse{JSONRPC: "2.0", ID: id, Error: err}
}

// NewSuccessResponse constructs a success response envelope.
func NewSuccessResponse(id json.RawMessage, result any) RPCResponse {
	return RPCResponse{JSONRPC: "2.0", ID: id, Result: result}
}

// ── MCP spec payload types ────────────────────────────────────────────────

// InitializeResult is the body returned from the `initialize` request.
type InitializeResult struct {
	ProtocolVersion string             `json:"protocolVersion"`
	ServerInfo      ServerInfo         `json:"serverInfo"`
	Capabilities    ServerCapabilities `json:"capabilities"`
}

// ServerInfo identifies the MCP server implementation.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ServerCapabilities advertises optional MCP capabilities.
type ServerCapabilities struct {
	Tools *ToolsCapability `json:"tools,omitempty"`
}

// ToolsCapability signals that the server supports tools/list and tools/call.
type ToolsCapability struct{}

// ToolsListResult is the body of `tools/list`.
type ToolsListResult struct {
	Tools []ToolDescriptor `json:"tools"`
}
