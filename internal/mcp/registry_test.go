package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

type fakeTool struct {
	name   string
	called bool
}

func (f *fakeTool) Name() string        { return f.name }
func (f *fakeTool) Description() string { return "fake " + f.name }
func (f *fakeTool) InputSchema() string { return `{"type":"object"}` }
func (f *fakeTool) Call(_ context.Context, _ json.RawMessage) (ToolResult, error) {
	f.called = true
	return TextResult("ok"), nil
}

func TestRegistry_RegisterAndList(t *testing.T) {
	r := NewRegistry()
	a := &fakeTool{name: "a"}
	b := &fakeTool{name: "b"}
	r.Register(a)
	r.Register(b)
	list := r.List()
	if len(list) != 2 {
		t.Fatalf("want 2 tools, got %d", len(list))
	}
	if list[0].Name != "a" || list[1].Name != "b" {
		t.Fatalf("unexpected order: %+v", list)
	}
}

func TestRegistry_Dispatch(t *testing.T) {
	r := NewRegistry()
	a := &fakeTool{name: "a"}
	r.Register(a)

	result, err := r.Dispatch(context.Background(), "a", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if !a.called {
		t.Fatal("tool not invoked")
	}
	if len(result.Content) == 0 || result.Content[0].Text != "ok" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestRegistry_Dispatch_UnknownTool(t *testing.T) {
	r := NewRegistry()
	_, err := r.Dispatch(context.Background(), "missing", json.RawMessage(`{}`))
	var mcpErr *Error
	if !errors.As(err, &mcpErr) {
		t.Fatalf("want *Error, got %T", err)
	}
	if mcpErr.Code != ErrCodeMethodNotFound {
		t.Fatalf("want ErrCodeMethodNotFound, got %d", mcpErr.Code)
	}
}

func TestError_StandardCodes(t *testing.T) {
	if ErrCodeParseError != -32700 {
		t.Fatalf("parse error code: %d", ErrCodeParseError)
	}
	if ErrCodeInvalidParams != -32602 {
		t.Fatalf("invalid params code: %d", ErrCodeInvalidParams)
	}
}
