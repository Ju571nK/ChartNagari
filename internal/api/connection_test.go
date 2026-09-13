package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Ju571nK/Chatter/internal/hub"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
)

type connectionTestHub struct{}

func (connectionTestHub) ServeWS(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }
func (connectionTestHub) ClientCount() int                               { return 0 }

func TestRemoteAccessReadsAndTickets(t *testing.T) {
	s := New(t.TempDir(), "")
	s.WithRemoteAccess(true)
	s.WithAPIToken("test-admin")
	s.WithHub(connectionTestHub{})
	s.WithAllowedOrigins([]string{"https://app.example"})
	call := func(method, path, token, origin, protocol string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, nil)
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		r.Header.Set("Origin", origin)
		r.Header.Set("Sec-WebSocket-Protocol", protocol)
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		return w
	}
	for _, token := range []string{"", "wrong"} {
		if w := call("GET", "/api/connection", token, "https://app.example", ""); w.Code != 401 {
			t.Fatalf("unprotected read: %d", w.Code)
		}
	}
	if w := call("GET", "/api/connection", "test-admin", "https://app.example", ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"remote_access":true`) {
		t.Fatal(w.Code, w.Body)
	}
	if w := call("OPTIONS", "/api/connection", "", "https://app.example", ""); w.Code != 204 || w.Header().Get("Access-Control-Allow-Origin") != "https://app.example" {
		t.Fatal("preflight failed")
	}
	if w := call("OPTIONS", "/api/connection", "", "https://evil.example", ""); w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("unexpected CORS access")
	}
	if w := call("POST", "/api/connection/ws-ticket", "", "https://app.example", ""); w.Code != 401 {
		t.Fatal("ticket must require authentication")
	}
	w := call("POST", "/api/connection/ws-ticket", "test-admin", "https://app.example", "")
	var out map[string]string
	json.Unmarshal(w.Body.Bytes(), &out)
	ticket := out["ticket"]
	if len(ticket) != 64 {
		t.Fatal("invalid ticket")
	}
	if w := call("GET", "/ws", "", "https://app.example", "ticket."+ticket); w.Code != 204 {
		t.Fatal("valid ticket failed", w.Code)
	}
	if w := call("GET", "/ws", "", "https://app.example", "ticket."+ticket); w.Code != 401 {
		t.Fatal("ticket replay allowed")
	}
	s.wsTickets["expired"] = connectionTicket{"https://app.example", time.Now().Add(-time.Second)}
	if w := call("GET", "/ws", "", "https://app.example", "ticket.expired"); w.Code != 401 {
		t.Fatal("expired ticket allowed")
	}
	s.wsTickets["bound"] = connectionTicket{"https://other.example", time.Now().Add(time.Second)}
	if w := call("GET", "/ws", "", "https://app.example", "ticket.bound"); w.Code != 401 {
		t.Fatal("wrong ticket origin allowed")
	}
	if w := call("GET", "/ws", "", "https://app.example", "plugin-id.fake"); w.Code != 401 {
		t.Fatal("partial execution protocol bypassed auth")
	}
	s.WithHub(hub.New(zerolog.Nop()))
	if w := call("GET", "/ws", "", "https://app.example", "chartnagari.v1,plugin-id.fake"); w.Code != 401 {
		t.Fatal("execution HMAC bypassed")
	}
}

func TestLocalReadCompatibility(t *testing.T) {
	s := New(t.TempDir(), "")
	s.WithAPIToken("admin")
	s.WithSettingsFile(t.TempDir() + "/settings.yaml")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/settings/config", nil))
	if w.Code != 200 {
		t.Fatal("local read compatibility changed")
	}
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/connection", nil))
	if w.Code != 401 {
		t.Fatal("connection verification must authenticate even in local mode")
	}
}

func TestBrowserWebSocketHandshake(t *testing.T) {
	s := New(t.TempDir(), "")
	s.WithRemoteAccess(true)
	s.WithAPIToken("admin-test")
	s.WithAllowedOrigins([]string{"https://app.example"})
	s.WithHub(hub.New(zerolog.Nop()))
	s.wsTickets = map[string]connectionTicket{"handshake": {"https://app.example", time.Now().Add(time.Minute)}}
	server := httptest.NewServer(s.Handler())
	defer server.Close()
	dialer := websocket.Dialer{Subprotocols: []string{"chartnagari.browser.v1", "ticket.handshake"}}
	conn, response, err := dialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/ws", http.Header{"Origin": []string{"https://app.example"}})
	if err != nil {
		t.Fatalf("handshake: %v (%v)", err, response)
	}
	defer conn.Close()
	if conn.Subprotocol() != "chartnagari.browser.v1" {
		t.Fatal("browser protocol was not negotiated")
	}
}
