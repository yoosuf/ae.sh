// Package claude is an optional adapter for Claude Code's noninteractive CLI.
// It is not exercised by offline tests. Credentials come from the environment,
// never from CLI flags, and commands are spawned from argument arrays.
package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yoosuf/ae.sh/internal/integration"
	"github.com/yoosuf/ae.sh/internal/util"
)

// Claude Code added the settings keys that make file tools refuse paths
// outside the working directories in v2.1.314. AE requires that version
// before enabling writes; older binaries fail closed.
var minConfinementVersion = [3]int{2, 1, 314}

var versionPattern = regexp.MustCompile(`(\d+)\.(\d+)\.(\d+)`)

// Provider invokes `claude -p --output-format json`. File tools are read-only
// unless a caller explicitly sets RunRequest.Write for a managed workspace.
// Write mode passes a temporary settings file that blocks file-tool reads and
// writes outside the working directory, and never enables a shell tool.
// Verified against https://code.claude.com/docs/en/permissions.md on 2026-11-25.
type Provider struct {
	Binary string
	Model  string

	mu      sync.Mutex
	cancels map[string]context.CancelFunc
}

func New() *Provider {
	return &Provider{Binary: "claude", cancels: make(map[string]context.CancelFunc)}
}

func (p *Provider) Name() string { return "claude" }

func (p *Provider) Capabilities() integration.Capabilities {
	return integration.Capabilities{StructuredOutput: true, Cancellation: true, Usage: true, SessionMetadata: true, Live: true, ModelSelection: true}
}

func (p *Provider) Run(ctx context.Context, request integration.RunRequest) (integration.RunResult, error) {
	if p.Binary == "" {
		return integration.RunResult{}, fmt.Errorf("claude binary not configured")
	}
	if request.Workspace == "" {
		return integration.RunResult{}, fmt.Errorf("claude provider requires an isolated workspace")
	}
	if request.Timeout <= 0 {
		request.Timeout = 5 * time.Minute
	}
	runCtx, cancel := context.WithTimeout(ctx, request.Timeout)
	defer cancel()
	p.register(request.TaskID, cancel)
	defer p.unregister(request.TaskID)

	model := request.RequestedModel
	if model == "" {
		model = p.Model
	}
	args := []string{"--bare"}
	if request.SessionID != "" {
		args = append(args, "--resume", request.SessionID)
	}
	tools := "Read"
	if request.Write {
		if request.Workspace == "" {
			return integration.RunResult{}, fmt.Errorf("claude writes require an isolated workspace")
		}
		supported, version, err := p.supportsWriteConfinement(ctx)
		if err != nil {
			return integration.RunResult{}, err
		}
		if !supported {
			return integration.RunResult{}, fmt.Errorf("claude %s cannot confine file writes; writes require v%d.%d.%d or later", version, minConfinementVersion[0], minConfinementVersion[1], minConfinementVersion[2])
		}
		settingsPath, err := writeConfinementSettings()
		if err != nil {
			return integration.RunResult{}, err
		}
		defer os.Remove(settingsPath)
		// No shell tool. Settings make Write/Edit refuse paths outside the
		// managed workspace even if the model asks for an absolute path.
		tools = "Read,Write,Edit"
		args = append(args, "--settings", settingsPath)
	}
	args = append(args,
		"-p",
		request.Prompt,
		"--output-format", "json",
		"--allowedTools", tools,
		"--permission-prompts", "none",
	)
	if model != "" {
		args = append(args, "--model", model)
	}
	cmd := exec.CommandContext(runCtx, p.Binary, args...)
	cmd.Dir = request.Workspace
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return integration.RunResult{}, fmt.Errorf("claude invocation failed: %w: %s", err, sanitizeShort(stderr.String()))
	}
	var response struct {
		Result       string                     `json:"result"`
		SessionID    string                     `json:"session_id"`
		Structured   map[string]any             `json:"structured_output"`
		TotalCostUSD *float64                   `json:"total_cost_usd"`
		ModelUsage   map[string]json.RawMessage `json:"modelUsage"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		return integration.RunResult{}, fmt.Errorf("parse claude JSON output: %w", err)
	}
	if response.Structured == nil {
		response.Structured = map[string]any{"result": response.Result}
	}
	sessionID := response.SessionID
	if sessionID == "" {
		sessionID = request.SessionID
	}
	exit := 0
	actualModel := ""
	if len(response.ModelUsage) == 1 {
		for name := range response.ModelUsage {
			actualModel = name
		}
	}
	return integration.RunResult{
		Output:         response.Structured,
		ExitStatus:     &exit,
		Usage:          integration.Usage{Known: response.TotalCostUSD != nil, CostUSD: response.TotalCostUSD},
		SessionID:      sessionID,
		RequestedModel: model,
		Model:          actualModel,
		Simulated:      false,
	}, nil
}

// supportsWriteConfinement checks the installed CLI version before enabling
// file writes. A missing or unparseable version is treated as unsupported.
func (p *Provider) supportsWriteConfinement(ctx context.Context) (bool, string, error) {
	out, err := exec.CommandContext(ctx, p.Binary, "--version").Output()
	if err != nil {
		return false, "", fmt.Errorf("cannot verify claude version for confined writes: %w", err)
	}
	version := strings.TrimSpace(string(out))
	match := versionPattern.FindStringSubmatch(version)
	if match == nil {
		return false, version, nil
	}
	var parsed [3]int
	for i := range parsed {
		value, err := strconv.Atoi(match[i+1])
		if err != nil {
			return false, version, nil
		}
		parsed[i] = value
	}
	for i := range parsed {
		if parsed[i] > minConfinementVersion[i] {
			return true, version, nil
		}
		if parsed[i] < minConfinementVersion[i] {
			return false, version, nil
		}
	}
	return true, version, nil
}

func writeConfinementSettings() (string, error) {
	settings := map[string]any{
		"permissions": map[string]bool{
			"blockReadsOutsideWorkingDirectories":  true,
			"blockWritesOutsideWorkingDirectories": true,
		},
	}
	raw, err := json.Marshal(settings)
	if err != nil {
		return "", err
	}
	file, err := os.CreateTemp("", "ae-claude-settings-*.json")
	if err != nil {
		return "", err
	}
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		os.Remove(file.Name())
		return "", err
	}
	if _, err := file.Write(raw); err != nil {
		file.Close()
		os.Remove(file.Name())
		return "", err
	}
	if err := file.Close(); err != nil {
		os.Remove(file.Name())
		return "", err
	}
	return file.Name(), nil
}

func (p *Provider) Cancel(_ context.Context, taskID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	cancel, ok := p.cancels[taskID]
	if !ok {
		return fmt.Errorf("no active claude invocation for %q", taskID)
	}
	cancel()
	return nil
}

func (p *Provider) register(key string, cancel context.CancelFunc) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cancels[key] = cancel
}

func (p *Provider) unregister(key string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.cancels, key)
}

func sanitizeShort(s string) string {
	s = strings.TrimSpace(util.Sanitize(s))
	if len(s) > 500 {
		s = s[:500] + "…"
	}
	return s
}
