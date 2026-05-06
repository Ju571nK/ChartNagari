package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/Ju571nK/Chatter/internal/mcp"
)

// mcpSessionTTL is how long an idle session is kept in memory.
const mcpSessionTTL = 30 * time.Minute

// mcpMaxBodyBytes caps request body size (DoS protection).
const mcpMaxBodyBytes = 1 << 20 // 1 MiB

type mcpSession struct {
	id        string
	createdAt time.Time
	lastSeen  time.Time
}

// mcpSessionStore is an in-memory session registry.
type mcpSessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*mcpSession
}

func newMCPSessionStore() *mcpSessionStore {
	return &mcpSessionStore{sessions: make(map[string]*mcpSession)}
}

func (s *mcpSessionStore) create() *mcpSession {
	id := newSessionID()
	sess := &mcpSession{id: id, createdAt: time.Now(), lastSeen: time.Now()}
	s.mu.Lock()
	s.sessions[id] = sess
	s.mu.Unlock()
	return sess
}

func (s *mcpSessionStore) get(id string) (*mcpSession, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	if ok {
		sess.lastSeen = time.Now()
	}
	return sess, ok
}

func (s *mcpSessionStore) evictIdle(cutoff time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, sess := range s.sessions {
		if sess.lastSeen.Before(cutoff) {
			delete(s.sessions, id)
		}
	}
}

func newSessionID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func (s *Server) handleMCP(w http.ResponseWriter, r *http.Request) {
	if !s.requireBearer(w, r) {
		return
	}
	if s.mcpRegistry == nil {
		http.Error(w, "MCP registry not configured", http.StatusServiceUnavailable)
		return
	}
	if s.mcpSessions == nil {
		s.mcpSessions = newMCPSessionStore()
	}

	s.mcpSessions.evictIdle(time.Now().Add(-mcpSessionTTL))

	r.Body = http.MaxBytesReader(w, r.Body, mcpMaxBodyBytes)
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		_ = json.NewEncoder(w).Encode(mcp.NewErrorResponse(nil,
			&mcp.Error{Code: mcp.ErrCodeInternalError, Message: "request body too large"}))
		return
	}

	var req mcp.RPCRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(mcp.NewErrorResponse(nil,
			&mcp.Error{Code: mcp.ErrCodeParseError, Message: "invalid JSON"}))
		return
	}

	sid := r.Header.Get("Mcp-Session-Id")
	if req.Method == "initialize" {
		sess := s.mcpSessions.create()
		w.Header().Set("Mcp-Session-Id", sess.id)
	} else if sid != "" {
		if _, ok := s.mcpSessions.get(sid); !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(mcp.NewErrorResponse(req.ID,
				&mcp.Error{Code: mcp.ErrCodeInternalError, Message: "session expired — reinitialize"}))
			return
		}
	}

	resp := s.dispatchMCP(r.Context(), req)

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) dispatchMCP(ctx context.Context, req mcp.RPCRequest) mcp.RPCResponse {
	switch req.Method {
	case "initialize":
		return mcp.NewSuccessResponse(req.ID, mcp.InitializeResult{
			ProtocolVersion: "2024-11-05",
			ServerInfo:      mcp.ServerInfo{Name: "chartnagari", Version: "2.7.0.0"},
			Capabilities:    mcp.ServerCapabilities{Tools: &mcp.ToolsCapability{}},
		})
	case "tools/list":
		return mcp.NewSuccessResponse(req.ID, mcp.ToolsListResult{Tools: s.mcpRegistry.List()})
	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return mcp.NewErrorResponse(req.ID, &mcp.Error{Code: mcp.ErrCodeInvalidParams, Message: err.Error()})
		}
		start := time.Now()
		result, err := s.mcpRegistry.Dispatch(ctx, p.Name, p.Arguments)
		dur := time.Since(start)
		if err != nil {
			var mcpErr *mcp.Error
			if errors.As(err, &mcpErr) {
				log.Warn().Str("tool", p.Name).Dur("duration", dur).Err(err).Msg("api: mcp tool returned error")
				return mcp.NewErrorResponse(req.ID, mcpErr)
			}
			log.Error().Str("tool", p.Name).Dur("duration", dur).Err(err).Msg("api: mcp tool internal error")
			return mcp.NewErrorResponse(req.ID, &mcp.Error{Code: mcp.ErrCodeInternalError, Message: err.Error()})
		}
		log.Info().Str("tool", p.Name).Dur("duration", dur).Msg("api: mcp tool call")
		return mcp.NewSuccessResponse(req.ID, result)
	default:
		return mcp.NewErrorResponse(req.ID, &mcp.Error{
			Code:    mcp.ErrCodeMethodNotFound,
			Message: "unknown method: " + req.Method,
		})
	}
}
