package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProductVersionMatchesVERSIONFile(t *testing.T) {
	root := findRepoRoot(t)
	wantBytes, err := os.ReadFile(filepath.Join(root, "VERSION"))
	if err != nil {
		t.Fatalf("read VERSION: %v", err)
	}
	want := strings.TrimSpace(string(wantBytes))
	if want == "" {
		t.Fatal("VERSION file is empty")
	}
	got := ProductVersion()
	if got != want {
		t.Fatalf("ProductVersion() = %q, want %q from VERSION", got, want)
	}
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getcwd: %v", err)
	}
	dir := wd
	for i := 0; i < 8; i++ {
		if _, err := os.Stat(filepath.Join(dir, "VERSION")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatal("VERSION not found walking from package dir")
	return ""
}
