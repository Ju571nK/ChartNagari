package ollama

import (
	"context"
	"fmt"
	"os/exec"
)

// Starter spawns `ollama serve` as a background process. Callers expect the
// method to return quickly — only the spawn error (not the process exit) is
// surfaced. The process is NOT owned by the parent context; it continues
// running after this function returns.
type Starter interface {
	Start(ctx context.Context) (pid int, err error)
}

type osStarter struct{}

// DefaultStarter returns a Starter that launches the real ollama binary.
func DefaultStarter() Starter { return osStarter{} }

// Start spawns `ollama serve`. Does NOT use exec.CommandContext so the process
// outlives the request that triggered it. Returns the spawned PID.
func (osStarter) Start(_ context.Context) (int, error) {
	cmd := exec.Command("ollama", "serve")
	// Redirect stdout/stderr to avoid tying us to the parent's stdio lifetime.
	// The subprocess will write to its own fds; we don't read them.
	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("ollama serve: start: %w", err)
	}
	// Don't Wait() — the process lifecycle is decoupled from this request.
	// Releasing the process lets the Go runtime stop tracking it.
	if cmd.Process != nil {
		_ = cmd.Process.Release()
		return cmd.Process.Pid, nil
	}
	return 0, fmt.Errorf("ollama serve: no process handle")
}
