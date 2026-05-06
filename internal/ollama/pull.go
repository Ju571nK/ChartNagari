package ollama

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
)

// PullRunner is exported for callers (cmd/server) that need the concrete type.
type PullRunner interface {
	Pull(ctx context.Context, model string, onLine func(line []byte)) error
}

// pullRunner runs `ollama pull <model>` as a subprocess and streams stdout
// JSONL lines to onLine.
type pullRunner struct{}

// DefaultPullRunner returns a runner that uses os/exec to drive the real
// `ollama` binary.
func DefaultPullRunner() PullRunner { return pullRunner{} }

func (pullRunner) Pull(ctx context.Context, model string, onLine func(line []byte)) error {
	cmd := exec.CommandContext(ctx, "ollama", "pull", model)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("ollama pull: stdout pipe: %w", err)
	}
	// Interleave stderr into stdout so error messages surface in the SSE stream.
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("ollama pull: start: %w", err)
	}

	scanner := bufio.NewScanner(stdout)
	// Allow long lines — pull progress JSON can carry larger payloads.
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)
	for scanner.Scan() {
		onLine(scanner.Bytes())
	}
	if err := scanner.Err(); err != nil {
		// Best-effort: still wait on the process to avoid zombies.
		_ = cmd.Wait()
		return fmt.Errorf("ollama pull: scan: %w", err)
	}
	return cmd.Wait()
}
