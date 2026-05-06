package mcp

import (
	"strings"
)

// MarkdownTable renders a GitHub-flavored markdown table. Each row must have
// the same length as headers; callers are responsible for that. Cells are
// rendered verbatim — no escaping of `|` characters (callers must not pass them).
func MarkdownTable(headers []string, rows [][]string) string {
	var b strings.Builder
	// Header
	b.WriteString("| ")
	b.WriteString(strings.Join(headers, " | "))
	b.WriteString(" |\n")
	// Separator
	b.WriteString("|")
	for range headers {
		b.WriteString("-----|")
	}
	b.WriteString("\n")
	// Rows
	for _, row := range rows {
		b.WriteString("| ")
		b.WriteString(strings.Join(row, " | "))
		b.WriteString(" |\n")
	}
	return b.String()
}

// DashIfEmpty returns "—" (em-dash) when s is empty — conventional filler for
// missing values in LLM-facing markdown tables.
func DashIfEmpty(s string) string {
	if s == "" {
		return "—"
	}
	return s
}
