// Package main is the ChartNagari MCP stdio bridge. It reads JSON-RPC requests
// from stdin (one per line), forwards them via HTTP POST to a running
// ChartNagari server's /api/mcp endpoint, and writes responses to stdout.
//
// Configured via environment:
//
//	CHARTNAGARI_URL    (default http://localhost:8080)
//	CHARTNAGARI_TOKEN  (required if server's API_TOKEN is set)
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	defaultURL     = "http://localhost:8080"
	defaultTimeout = 60 * time.Second
)

type bridgeConfig struct {
	url     string
	token   string
	timeout time.Duration
}

func main() {
	cfg := bridgeConfig{
		url:     envOr("CHARTNAGARI_URL", defaultURL),
		token:   os.Getenv("CHARTNAGARI_TOKEN"),
		timeout: defaultTimeout,
	}
	cfg.url = strings.TrimRight(cfg.url, "/") + "/api/mcp"

	if err := runBridge(cfg, os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "chartnagari-mcp: %v\n", err)
		os.Exit(1)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func runBridge(cfg bridgeConfig, in io.Reader, out, stderr io.Writer) error {
	if cfg.url == "" || cfg.url == "/api/mcp" {
		return errors.New("CHARTNAGARI_URL must be set (e.g. http://localhost:8080)")
	}
	client := &http.Client{Timeout: cfg.timeout}
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 64*1024), 1<<20)

	var sessionID string
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		respBytes, newSID := forwardOne(client, cfg, line, sessionID)
		if newSID != "" {
			sessionID = newSID
		}
		_, _ = out.Write(respBytes)
		_, _ = out.Write([]byte("\n"))
	}
	return scanner.Err()
}

// forwardOne sends a single JSON-RPC request over HTTP. Returns the response
// body bytes and any Mcp-Session-Id captured from the response headers.
func forwardOne(client *http.Client, cfg bridgeConfig, body []byte, sessionID string) ([]byte, string) {
	req, err := http.NewRequest(http.MethodPost, cfg.url, bytes.NewReader(body))
	if err != nil {
		return rpcErrorBytes(body, -32603, "invalid request: "+err.Error()), ""
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.token != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.token)
	}
	if sessionID != "" {
		req.Header.Set("Mcp-Session-Id", sessionID)
	}

	resp, err := client.Do(req)
	if err != nil {
		return rpcErrorBytes(body, -32603, "ChartNagari server not reachable at "+cfg.url+": "+err.Error()), ""
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode == http.StatusUnauthorized {
		return rpcErrorBytes(body, -32603, "unauthorized — check CHARTNAGARI_TOKEN"), ""
	}
	if resp.StatusCode != http.StatusOK {
		buf, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return rpcErrorBytes(body, -32603, fmt.Sprintf("ChartNagari responded %d: %s", resp.StatusCode, string(buf))), ""
	}

	sid := resp.Header.Get("Mcp-Session-Id")
	buf, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	return buf, sid
}

// rpcErrorBytes synthesizes a JSON-RPC error response that preserves the
// request ID for correlation.
func rpcErrorBytes(origReq []byte, code int, msg string) []byte {
	var peek struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
	}
	_ = json.Unmarshal(origReq, &peek)
	resp := map[string]any{
		"jsonrpc": "2.0",
		"id":      peek.ID,
		"error":   map[string]any{"code": code, "message": msg},
	}
	b, _ := json.Marshal(resp)
	return b
}
