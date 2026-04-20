// Package ollama provides platform and Ollama process/network state detection.
// It is intentionally decoupled from internal/llm — it handles OS-level
// inspection (process existence, Docker environment, file presence) rather than
// inference.
package ollama

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	stdruntime "runtime"
	"strings"
	"time"
)

// Deployment describes how Ollama is expected to run relative to ChartNagari.
type Deployment string

const (
	DeploymentDocker Deployment = "docker"
	DeploymentNative Deployment = "native"
)

// State is the result of the five-branch detection state machine.
type State string

const (
	// StateReady — /api/tags is reachable and the configured model is present.
	StateReady State = "READY"

	// StateReadyNoModel — /api/tags is reachable but the configured model is missing.
	StateReadyNoModel State = "READY_NO_MODEL"

	// StateInstalledNotRunning — Ollama binary is present but the server is not running.
	StateInstalledNotRunning State = "INSTALLED_NOT_RUNNING"

	// StateNotInstalled — Ollama is not reachable and the binary cannot be found.
	StateNotInstalled State = "NOT_INSTALLED"

	// StateDockerSidecarAvailable — running inside Docker, sidecar template exists,
	// override file absent; the sidecar compose service can be activated.
	StateDockerSidecarAvailable State = "DOCKER_SIDECAR_AVAILABLE"
)

// Status is the full result returned by Detector.Detect.
type Status struct {
	State           State      `json:"state"`
	Host            string     `json:"host"`
	Model           string     `json:"model"`
	ModelsAvailable []string   `json:"models_available,omitempty"`
	Deployment      Deployment `json:"deployment"`
	Version         string     `json:"version,omitempty"`
	Suggest         Suggest    `json:"suggest"`
}

// Suggest contains a machine-readable action and an optional shell command.
type Suggest struct {
	Action    string `json:"action"`
	Command   string `json:"command,omitempty"`
	SizeBytes int64  `json:"size_bytes,omitempty"`
}

// RuntimeInspector abstracts OS / process / file seams so tests can inject fakes.
type RuntimeInspector interface {
	// InDocker reports whether the process is running inside a Docker container.
	InDocker() bool
	// OllamaVersion runs `ollama --version` and returns the trimmed output.
	// Returns an error when the binary is absent or the command fails.
	OllamaVersion() (string, error)
	// OverrideFileExists reports whether docker-compose.override.yml exists.
	OverrideFileExists() bool
	// SidecarTemplateExists reports whether docker-compose.ollama.yml.template exists.
	SidecarTemplateExists() bool
}

// modelSizes holds approximate download sizes for well-known models.
// Used to populate Suggest.SizeBytes when the model is not yet pulled.
var modelSizes = map[string]int64{
	"gemma4:4b":   2_600_000_000,
	"gemma4:12b":  7_200_000_000,
	"gemma4:27b":  15_000_000_000,
	"llama3.1:8b": 4_700_000_000,
}

// Detector performs the five-state Ollama detection sequence.
type Detector struct {
	host    string
	model   string
	client  *http.Client
	runtime RuntimeInspector
}

// NewDetector constructs a Detector with a 2-second HTTP timeout so that
// /api/tags probes never stall the caller.
func NewDetector(host, model string, runtime RuntimeInspector) *Detector {
	return &Detector{
		host:  host,
		model: model,
		client: &http.Client{
			Timeout: 2 * time.Second,
		},
		runtime: runtime,
	}
}

// installCommandForGOOS returns the recommended install shell command for the
// given GOOS string. Extracted so tests can verify the table independently.
func installCommandForGOOS(goos string) string {
	switch goos {
	case "linux":
		return "curl -fsSL https://ollama.com/install.sh | sh"
	case "darwin":
		return "brew install ollama"
	case "windows":
		return "winget install Ollama.Ollama"
	default:
		return "https://ollama.com/download"
	}
}

// tagsAPIResponse mirrors the Ollama GET /api/tags response.
// Only the `name` field inside each model entry is needed.
type tagsAPIResponse struct {
	Models []struct {
		Name string `json:"name"`
	} `json:"models"`
}

// deployment derives DeploymentDocker or DeploymentNative based on the inspector.
func (d *Detector) deployment() Deployment {
	if d.runtime.InDocker() {
		return DeploymentDocker
	}
	return DeploymentNative
}

// Detect runs the five-branch state machine and returns a fully populated Status.
// It never blocks longer than the 2-second HTTP client timeout.
func (d *Detector) Detect(ctx context.Context) Status {
	base := Status{
		Host:       d.host,
		Model:      d.model,
		Deployment: d.deployment(),
	}

	// --- Probe /api/tags ---
	models, reachable := d.fetchTags(ctx)
	if reachable {
		base.ModelsAvailable = models

		// Branch 1: model present → READY
		for _, m := range models {
			if m == d.model {
				base.State = StateReady
				base.Suggest = Suggest{Action: "ready"}
				return base
			}
		}

		// Branch 2: reachable but model absent → READY_NO_MODEL
		base.State = StateReadyNoModel
		base.Suggest = Suggest{
			Action:    "pull_model",
			Command:   "ollama pull " + d.model,
			SizeBytes: modelSizes[d.model],
		}
		return base
	}

	// /api/tags is unreachable from here on.

	// Branch 3: binary present but server not running → INSTALLED_NOT_RUNNING
	ver, err := d.runtime.OllamaVersion()
	if err == nil {
		base.State = StateInstalledNotRunning
		base.Version = ver
		base.Suggest = Suggest{
			Action:  "start_ollama",
			Command: "ollama serve",
		}
		return base
	}

	// Branch 4: Docker sidecar available
	if d.runtime.InDocker() && d.runtime.SidecarTemplateExists() && !d.runtime.OverrideFileExists() {
		base.State = StateDockerSidecarAvailable
		base.Deployment = DeploymentDocker
		base.Suggest = Suggest{Action: "enable_sidecar"}
		return base
	}

	// Branch 5: not installed at all
	base.State = StateNotInstalled
	base.Suggest = Suggest{
		Action:  "install_ollama",
		Command: installCommandForGOOS(stdruntime.GOOS),
	}
	return base
}

// fetchTags issues GET <host>/api/tags and parses the model name list.
// Returns (names, true) on HTTP 200; (nil, false) on any error or non-200.
func (d *Detector) fetchTags(ctx context.Context) ([]string, bool) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.host+"/api/tags", nil)
	if err != nil {
		return nil, false
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, false
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1 MiB cap
	if err != nil {
		return nil, false
	}

	var parsed tagsAPIResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, false
	}

	names := make([]string, 0, len(parsed.Models))
	for _, m := range parsed.Models {
		names = append(names, m.Name)
	}
	return names, true
}

// ---------------------------------------------------------------------------
// Real RuntimeInspector — production implementation backed by real OS calls.
// Tests must inject a fake instead of using this.
// ---------------------------------------------------------------------------

type osRuntime struct{}

func (osRuntime) InDocker() bool {
	_, err := os.Stat("/.dockerenv")
	return err == nil
}

func (osRuntime) OllamaVersion() (string, error) {
	out, err := exec.Command("ollama", "--version").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func (osRuntime) OverrideFileExists() bool {
	_, err := os.Stat("docker-compose.override.yml")
	return err == nil
}

func (osRuntime) SidecarTemplateExists() bool {
	_, err := os.Stat("docker-compose.ollama.yml.template")
	return err == nil
}

// DefaultRuntime returns the production OS-backed RuntimeInspector.
// Inject a fake via NewDetector for all test scenarios.
func DefaultRuntime() RuntimeInspector { return osRuntime{} }
