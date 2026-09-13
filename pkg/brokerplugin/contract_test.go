package brokerplugin

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// Keep the language-neutral artifact navigable and versioned with the SDK.
// This checks local references and operations, not full OpenAPI conformance.
func TestOpenAPIReferencesAndOperations(t *testing.T) {
	raw, err := os.ReadFile("../../docs/api/order-plugin-v1.openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var spec map[string]any
	if err = yaml.Unmarshal(raw, &spec); err != nil {
		t.Fatal(err)
	}
	if spec["openapi"] != "3.1.0" || spec["info"].(map[string]any)["version"] != ProtocolVersion {
		t.Fatal("protocol version drift")
	}
	paths := spec["paths"].(map[string]any)
	for path, methods := range map[string][]string{"/v1/manifest": {"get"}, "/v1/accounts": {"get"}, "/v1/orders": {"get", "post"}, "/v1/orders/{client_order_id}": {"get"}, "/v1/orders/{client_order_id}/cancel": {"post"}} {
		p, ok := paths[path].(map[string]any)
		if !ok {
			t.Fatal("missing path", path)
		}
		for _, method := range methods {
			if p[method] == nil {
				t.Fatal("missing operation", path, method)
			}
		}
	}
	var walk func(any)
	walk = func(value any) {
		switch v := value.(type) {
		case map[string]any:
			if ref, ok := v["$ref"].(string); ok {
				var target any = spec
				if !strings.HasPrefix(ref, "#/") {
					t.Fatal("nonlocal reference", ref)
				}
				for _, part := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
					m, ok := target.(map[string]any)
					if !ok || m[part] == nil {
						t.Fatal("broken reference", ref)
					}
					target = m[part]
				}
			}
			for _, child := range v {
				walk(child)
			}
		case []any:
			for _, child := range v {
				walk(child)
			}
		}
	}
	walk(spec)
}

func TestManifestRefusesUnimplementedCapabilities(t *testing.T) {
	base := Manifest{ProtocolVersion: ProtocolVersion, ID: "example", Name: "Example", Version: "1.0", Modes: []string{"paper"}}
	for name, change := range map[string]func(*Manifest){
		"live":                    func(m *Manifest) { m.Modes = []string{"live"} },
		"future":                  func(m *Manifest) { m.ProtocolVersion = "2.0" },
		"replace":                 func(m *Manifest) { m.Capabilities.Replace = true },
		"events":                  func(m *Manifest) { m.Capabilities.Events = true },
		"limit":                   func(m *Manifest) { m.Capabilities.OrderTypes = []string{"limit"} },
		"empty submit dimensions": func(m *Manifest) { m.Capabilities.Submit = true },
		"secret options": func(m *Manifest) {
			m.Settings = []SettingField{{Key: "token", Label: "Token", Type: "secret", Options: []string{"must-not-be-published"}}}
		},
		"select without options": func(m *Manifest) { m.Settings = []SettingField{{Key: "mode", Label: "Mode", Type: "select"}} },
	} {
		t.Run(name, func(t *testing.T) {
			m := base
			change(&m)
			if m.Validate() == nil {
				t.Fatal("invalid manifest accepted")
			}
		})
	}
}

func TestOrderResponseValidation(t *testing.T) {
	for _, o := range []Order{
		{ClientOrderID: "id", AccountID: "a", Status: "filled", Quantity: "2", FilledQuantity: "1"},
		{ClientOrderID: "id", AccountID: "a", Status: "partial_fill", Quantity: "2", FilledQuantity: "2"},
		{ClientOrderID: "id", AccountID: "a", Status: "partial_fill", Quantity: "2", FilledQuantity: "0"},
		{ClientOrderID: "id", AccountID: "a", Status: "filled"},
		{ClientOrderID: "id", AccountID: "a", Status: "future-status"},
		{ClientOrderID: "id", AccountID: "a", Status: "cancelled", Quantity: "2", FilledQuantity: "3"},
		{ClientOrderID: "id", AccountID: "a", Status: "unknown", Quantity: "1e3"},
	} {
		if o.Validate() == nil {
			t.Fatalf("invalid broker response accepted: %+v", o)
		}
	}
}
