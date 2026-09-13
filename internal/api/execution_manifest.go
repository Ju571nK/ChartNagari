package api

import (
	"encoding/json"
	"net/http"
	"net/url"

	plugin "github.com/Ju571nK/Chatter/pkg/brokerplugin"
)

// getExecutionPluginManifest only inspects an already registered plugin. It
// cannot install code, change settings or activate order dispatch. Credentials
// stay server-side. A configured admin token is mandatory for this new proxy.
func (s *Server) getExecutionPluginManifest(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if s.apiToken == "" {
		http.Error(w, "admin token required for plugin inspection", http.StatusForbidden)
		return
	}
	if !s.requireBearer(w, r) {
		return
	}
	if s.execHolder == nil {
		http.NotFound(w, r)
		return
	}
	for _, p := range s.execHolder.Get().Plugins {
		if p.ID != r.PathValue("id") {
			continue
		}
		u, err := url.Parse(p.URL)
		if err != nil || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			http.Error(w, "invalid plugin URL", 422)
			return
		}
		// v1 is rooted at the registered webhook's origin. Never accept a
		// caller-supplied URL, path or signature target.
		c, err := plugin.NewClient(u.Scheme+"://"+u.Host, p.ID, p.Secret)
		if err != nil {
			http.Error(w, "plugin requires HTTPS or loopback HTTP and valid credentials", 422)
			return
		}
		var m plugin.Manifest
		if _, err = c.Do(r.Context(), http.MethodGet, "/v1/manifest", nil, &m); err != nil {
			http.Error(w, "plugin v1 metadata unavailable; legacy webhook may still be supported", 502)
			return
		}
		if m.Validate() != nil || m.ID != p.ID {
			http.Error(w, "incompatible plugin manifest", 502)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(m)
		return
	}
	http.NotFound(w, r)
}
