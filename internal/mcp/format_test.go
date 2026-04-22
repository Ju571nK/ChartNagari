package mcp

import (
	"strings"
	"testing"
)

func TestMarkdownTable_Basic(t *testing.T) {
	out := MarkdownTable(
		[]string{"Sym", "Dir", "Score"},
		[][]string{
			{"BTCUSDT", "LONG", "14.5"},
			{"ETHUSDT", "SHORT", "11.0"},
		},
	)
	if !strings.Contains(out, "| Sym | Dir | Score |") {
		t.Errorf("header missing: %q", out)
	}
	if !strings.Contains(out, "| BTCUSDT | LONG | 14.5 |") {
		t.Errorf("row missing: %q", out)
	}
	if !strings.Contains(out, "|-----|-----|-----|") && !strings.Contains(out, "|-") {
		t.Errorf("separator missing: %q", out)
	}
}

func TestMarkdownTable_EmptyRows(t *testing.T) {
	out := MarkdownTable([]string{"A", "B"}, nil)
	if !strings.Contains(out, "| A | B |") {
		t.Errorf("header missing on empty table: %q", out)
	}
}

func TestDashIfEmpty(t *testing.T) {
	if DashIfEmpty("") != "—" {
		t.Error("empty should be em-dash")
	}
	if DashIfEmpty("x") != "x" {
		t.Error("non-empty should pass through")
	}
}
