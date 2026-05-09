package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Ju571nK/Chatter/internal/marks"
)

type fakeRollupSource struct {
	rows  []marks.RollupRow
	err   error
	gotBy marks.GroupBy
}

func (f *fakeRollupSource) Rollup(by marks.GroupBy, since time.Time) ([]marks.RollupRow, error) {
	f.gotBy = by
	if f.err != nil {
		return nil, f.err
	}
	return f.rows, nil
}

func TestGetMyPerformance_RendersTable(t *testing.T) {
	src := &fakeRollupSource{rows: []marks.RollupRow{
		{Key: "ict_liquidity_sweep", Took: 12, Skipped: 6, Wins: 8, Losses: 3, BreakEvens: 1, HitRate: 0.667, SkipRate: 0.333},
	}}
	tool := NewGetMyPerformance(src)
	res, err := tool.Call(context.Background(), json.RawMessage(`{"by":"rule","since_days":30}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	text := res.Content[0].Text
	if !strings.Contains(text, "ict_liquidity_sweep") {
		t.Errorf("missing rule name: %q", text)
	}
	if !strings.Contains(text, "66.7%") {
		t.Errorf("missing hit rate %%: %q", text)
	}
	if src.gotBy != marks.GroupByRule {
		t.Errorf("got by = %q, want rule", src.gotBy)
	}
}

func TestGetMyPerformance_EmptyResult(t *testing.T) {
	src := &fakeRollupSource{rows: []marks.RollupRow{}}
	tool := NewGetMyPerformance(src)
	res, _ := tool.Call(context.Background(), json.RawMessage(`{"by":"rule"}`))
	text := res.Content[0].Text
	if !strings.Contains(text, "No marked trades") {
		t.Errorf("expected empty message, got %q", text)
	}
}

func TestGetMyPerformance_FilterByMethodology(t *testing.T) {
	src := &fakeRollupSource{rows: []marks.RollupRow{
		{Key: "ict_a", Took: 1, Wins: 1},
		{Key: "wyckoff_b", Took: 1, Losses: 1},
	}}
	tool := NewGetMyPerformance(src)
	res, _ := tool.Call(context.Background(), json.RawMessage(`{"by":"rule","filter":{"methodology":"ict"}}`))
	text := res.Content[0].Text
	if !strings.Contains(text, "ict_a") {
		t.Errorf("ict_a should be present: %q", text)
	}
	if strings.Contains(text, "wyckoff_b") {
		t.Errorf("wyckoff_b should be filtered out: %q", text)
	}
}

func TestGetMyPerformance_InvalidBy(t *testing.T) {
	src := &fakeRollupSource{}
	tool := NewGetMyPerformance(src)
	_, err := tool.Call(context.Background(), json.RawMessage(`{"by":"explode"}`))
	if err == nil {
		t.Fatal("expected error for invalid by")
	}
	var mcpErr *Error
	if !errors.As(err, &mcpErr) || mcpErr.Code != ErrCodeInvalidParams {
		t.Errorf("want InvalidParams, got %v", err)
	}
}
