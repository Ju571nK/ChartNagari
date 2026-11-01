package api

import (
	"os"
	"path/filepath"
	"strings"
)

// productVersion is the trimmed contents of the repo VERSION file.
// go test uses the package directory as cwd, and the server uses the repo
// or image root, so the search walks parents and the executable directory.
func productVersion() string {
	starts := make([]string, 0, 2)
	if wd, err := os.Getwd(); err == nil {
		starts = append(starts, wd)
	}
	if exe, err := os.Executable(); err == nil {
		starts = append(starts, filepath.Dir(exe))
	}
	seen := map[string]struct{}{}
	for _, start := range starts {
		dir := start
		for i := 0; i < 8; i++ {
			if _, ok := seen[dir]; ok {
				break
			}
			seen[dir] = struct{}{}
			if v := readVersionFile(filepath.Join(dir, "VERSION")); v != "" {
				return v
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	return ""
}

func readVersionFile(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}
