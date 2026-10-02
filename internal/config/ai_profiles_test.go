package config

import (
	"path/filepath"
	"testing"
)

func TestAIProfileActivateSnapshotRejectsConcurrentSave(t *testing.T) {
	store, err := NewAIProfileStore(filepath.Join(t.TempDir(), "profiles.json"))
	if err != nil {
		t.Fatal(err)
	}
	initial, err := store.Save(AIProfile{Name: "Local", Kind: "ollama", BaseURL: "http://localhost:11434", Model: "qwen3:4b"}, false)
	if err != nil {
		t.Fatal(err)
	}
	expected, ok := store.Get(initial.ID)
	if !ok {
		t.Fatal("saved profile not found")
	}
	if !store.MarkTested(expected) {
		t.Fatal("could not mark initial snapshot tested")
	}
	if _, err := store.Save(AIProfile{ID: expected.ID, Name: "Local", Kind: "ollama", BaseURL: expected.BaseURL, Model: "qwen3:8b"}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ActivateSnapshot(expected); err == nil {
		t.Fatal("activation accepted a snapshot superseded by a concurrent save")
	}
	if _, ok := store.Active(); ok {
		t.Fatal("stale activation persisted an active profile")
	}

	current, ok := store.Get(initial.ID)
	if !ok {
		t.Fatal("edited profile not found")
	}
	if store.MarkTested(current) == false {
		t.Fatal("could not mark current snapshot tested")
	}
	activated, err := store.ActivateSnapshot(current)
	if err != nil {
		t.Fatal(err)
	}
	active, ok := store.Active()
	if !ok || active != current || activated != current {
		t.Fatalf("activated snapshot mismatch: returned=%+v stored=%+v ok=%v", activated, active, ok)
	}
}
