// Package hub manages WebSocket client connections and broadcasts messages
// to all connected browsers in real time.
package hub

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
)

// Client type tags. BroadcastTo uses these to route messages.
const (
	ClientBrowser   = "browser"
	ClientExecution = "execution"
)

// Message is the envelope sent over WebSocket to all clients.
type Message struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload,omitempty"`
}

// OriginCheck returns true if the Origin header is allowed. If allowedOrigins
// is empty the check is permissive (local/self-hosted default). Codex #5
// replaces the previous unconditional "return true" with an allowlist.
func OriginCheck(allowedOrigins []string) func(r *http.Request) bool {
	allowed := make([]string, 0, len(allowedOrigins))
	for _, o := range allowedOrigins {
		o = strings.TrimSpace(o)
		if o != "" {
			allowed = append(allowed, o)
		}
	}
	return func(r *http.Request) bool {
		if len(allowed) == 0 {
			return true
		}
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}
		for _, a := range allowed {
			if a == "*" || a == origin {
				return true
			}
		}
		return false
	}
}

// ExecutionAuth resolves a plugin ID → HMAC secret. Returns ok=false for
// unknown or disabled plugins. Injected at hub creation so the hub package
// does not depend on internal/config.
type ExecutionAuth func(pluginID string) (secret string, ok bool)

// client represents a single WebSocket connection.
type client struct {
	hub        *Hub
	conn       *websocket.Conn
	send       chan []byte
	clientType string // ClientBrowser | ClientExecution
	pluginID   string // set when clientType == ClientExecution
}

// Hub maintains the set of active clients and broadcasts messages to them.
type Hub struct {
	clients    map[*client]struct{}
	mu         sync.RWMutex
	broadcast  chan []byte
	register   chan *client
	unregister chan *client
	log        zerolog.Logger
	upgrader   websocket.Upgrader
	auth       ExecutionAuth
	skewSec    int
}

// Options configures a Hub.
type Options struct {
	AllowedOrigins   []string
	ExecutionAuth    ExecutionAuth
	TimestampSkewSec int
}

// New creates a Hub with permissive defaults. Call Run() in a goroutine.
func New(log zerolog.Logger) *Hub {
	return NewWithOptions(log, Options{})
}

// NewWithOptions creates a Hub with explicit origin allowlist and exec auth.
func NewWithOptions(log zerolog.Logger, opts Options) *Hub {
	skew := opts.TimestampSkewSec
	if skew <= 0 {
		skew = 300
	}
	return &Hub{
		clients:    make(map[*client]struct{}),
		broadcast:  make(chan []byte, 256),
		register:   make(chan *client, 16),
		unregister: make(chan *client, 16),
		log:        log,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin:     OriginCheck(opts.AllowedOrigins),
			Subprotocols:    []string{"chartnagari.v1"},
		},
		auth:    opts.ExecutionAuth,
		skewSec: skew,
	}
}

// Run processes registrations, unregistrations, and broadcasts.
func (h *Hub) Run() {
	for {
		select {
		case c := <-h.register:
			h.mu.Lock()
			h.clients[c] = struct{}{}
			h.mu.Unlock()
			h.log.Debug().Str("client_type", c.clientType).Msg("ws: client connected")

		case c := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[c]; ok {
				delete(h.clients, c)
				close(c.send)
			}
			h.mu.Unlock()
			h.log.Debug().Str("client_type", c.clientType).Msg("ws: client disconnected")

		case msg := <-h.broadcast:
			h.mu.RLock()
			for c := range h.clients {
				// Legacy Broadcast path routes to browsers only; execution
				// clients opt-in via BroadcastTo.
				if c.clientType != ClientBrowser {
					continue
				}
				select {
				case c.send <- msg:
				default:
				}
			}
			h.mu.RUnlock()
		}
	}
}

// Broadcast sends a typed message to all connected browser clients.
// Execution clients are excluded — use BroadcastTo("execution", ...).
func (h *Hub) Broadcast(msgType string, payload interface{}) {
	data, err := json.Marshal(Message{Type: msgType, Payload: payload})
	if err != nil {
		h.log.Error().Err(err).Msg("ws: marshal error")
		return
	}
	select {
	case h.broadcast <- data:
	default:
		h.log.Warn().Msg("ws: broadcast channel full — dropping message")
	}
}

// BroadcastTo sends a typed message only to clients matching the given type.
// Decision A2: prevents browser chart signals from leaking to execution plugins.
func (h *Hub) BroadcastTo(clientType string, msgType string, payload interface{}) {
	data, err := json.Marshal(Message{Type: msgType, Payload: payload})
	if err != nil {
		h.log.Error().Err(err).Msg("ws: marshal error")
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.clients {
		if c.clientType != clientType {
			continue
		}
		select {
		case c.send <- data:
		default:
		}
	}
}

// ClientCount returns the number of currently connected clients.
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// ClientCountByType returns the number of connected clients of the given type.
func (h *Hub) ClientCountByType(clientType string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	n := 0
	for c := range h.clients {
		if c.clientType == clientType {
			n++
		}
	}
	return n
}

// parseExecutionSubprotocol extracts plugin-id/signature/ts from the
// Sec-WebSocket-Protocol header (Codex #5). wantsExec=false for browsers.
func parseExecutionSubprotocol(r *http.Request) (pluginID, signature string, ts int64, wantsExec bool) {
	raw := r.Header.Get("Sec-WebSocket-Protocol")
	if raw == "" {
		return "", "", 0, false
	}
	parts := strings.Split(raw, ",")
	var hasV1 bool
	for _, p := range parts {
		p = strings.TrimSpace(p)
		switch {
		case p == "chartnagari.v1":
			hasV1 = true
		case strings.HasPrefix(p, "plugin-id."):
			pluginID = strings.TrimPrefix(p, "plugin-id.")
		case strings.HasPrefix(p, "signature."):
			signature = strings.TrimPrefix(p, "signature.")
		case strings.HasPrefix(p, "ts."):
			if v, err := strconv.ParseInt(strings.TrimPrefix(p, "ts."), 10, 64); err == nil {
				ts = v
			}
		}
	}
	if !hasV1 {
		return "", "", 0, false
	}
	return pluginID, signature, ts, true
}

// verifyExecutionSubprotocol validates the subprotocol HMAC. Canonical string:
// plugin_id + "\n" + ts (as decimal unix seconds).
func (h *Hub) verifyExecutionSubprotocol(pluginID, signature string, ts int64) bool {
	if h.auth == nil {
		return false
	}
	if pluginID == "" || signature == "" || ts == 0 {
		return false
	}
	now := time.Now().Unix()
	diff := now - ts
	if diff < 0 {
		diff = -diff
	}
	if diff > int64(h.skewSec) {
		return false
	}
	secret, ok := h.auth(pluginID)
	if !ok || secret == "" {
		return false
	}
	canonical := pluginID + "\n" + strconv.FormatInt(ts, 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(canonical))
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}

// ServeWS upgrades an HTTP connection to WebSocket and registers the client.
//
// Client type resolution (Decision A2):
//   - Sec-WebSocket-Protocol contains chartnagari.v1 + plugin-id/signature/ts
//     → execution client (HMAC verified, 401 on failure).
//   - Otherwise → browser client.
func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request) {
	pluginID, sig, ts, wantsExec := parseExecutionSubprotocol(r)

	clientType := ClientBrowser
	if wantsExec {
		if !h.verifyExecutionSubprotocol(pluginID, sig, ts) {
			h.log.Warn().
				Str("plugin_id", pluginID).
				Int64("ts", ts).
				Msg("ws: execution subprotocol auth failed")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		clientType = ClientExecution
	}

	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.log.Error().Err(err).Msg("ws: upgrade failed")
		return
	}

	c := &client{
		hub:        h,
		conn:       conn,
		send:       make(chan []byte, 64),
		clientType: clientType,
		pluginID:   pluginID,
	}
	h.register <- c

	welcome, _ := json.Marshal(Message{Type: "connected", Payload: map[string]string{
		"message":     "ChartNagari WebSocket connected",
		"client_type": clientType,
	}})
	c.send <- welcome

	go c.writePump()
	go c.readPump()
}

const (
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = (pongWait * 9) / 10
)

// writePump pumps messages from the send channel to the WebSocket connection.
func (c *client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	for {
		select {
		case msg, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// readPump reads from the WebSocket (primarily to handle pong/close frames).
func (c *client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()
	c.conn.SetReadLimit(512)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
	for {
		if _, _, err := c.conn.ReadMessage(); err != nil {
			break
		}
	}
}
