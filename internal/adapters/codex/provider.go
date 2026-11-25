// Package codex is an optional adapter for Codex noninteractive CLI.
// It is not exercised by offline tests. Credentials are inherited from the
// environment; commands are spawned from argument arrays, never a shell.
package codex

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/yoosuf/ae.sh/internal/integration"
)

// Provider invokes `codex exec --json` with a read-only sandbox by default.
// workspace-write is used only when the caller explicitly sets Write for a
// managed workspace. Verified against
// https://learn.chatgpt.com/docs/non-interactive-mode.md on 2026-11-25.
type Provider struct {
	Binary string

	mu      sync.Mutex
	cancels map[string]context.CancelFunc
}

func New() *Provider {
	return &Provider{Binary: "codex", cancels: make(map[string]context.CancelFunc)}
}

func (p *Provider) Name() string { return "codex" }

func (p *Provider) Capabilities() integration.Capabilities {
	return integration.Capabilities{StructuredOutput: true, Cancellation: true, Usage: true, SessionMetadata: true, Live: true}
}

func (p *Provider) Run(ctx context.Context, request integration.RunRequest) (integration.RunResult, error) {
	if p.Binary == "" {
		return integration.RunResult{}, fmt.Errorf("codex binary not configured")
	}
	if request.Workspace == "" {
		return integration.RunResult{}, fmt.Errorf("codex provider requires an isolated workspace")
	}
	if request.Timeout <= 0 {
		request.Timeout = 5 * time.Minute
	}
	runCtx, cancel := context.WithTimeout(ctx, request.Timeout)
	defer cancel()
	p.register(request.TaskID, cancel)
	defer p.unregister(request.TaskID)

	if request.SessionID != "" && !request.Persistent {
		return integration.RunResult{}, fmt.Errorf("codex session resume requires a persistent conversation")
	}
	args := []string{"exec"}
	if request.SessionID != "" {
		args = append(args, "resume", request.SessionID)
	} else if !request.Persistent {
		args = append(args, "--ephemeral")
	}
	sandbox := "read-only"
	if request.Write {
		sandbox = "workspace-write"
	}
	args = append(args, "--json", "--sandbox", sandbox, request.Prompt)
	cmd := exec.CommandContext(runCtx, p.Binary, args...)
	cmd.Dir = request.Workspace
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return integration.RunResult{}, fmt.Errorf("codex invocation failed: %w: %s", err, sanitizeShort(stderr.String()))
	}

	var lastMessage string
	var sessionID string
	var usage integration.Usage
	scanner := bufio.NewScanner(bytes.NewReader(stdout.Bytes()))
	scanner.Buffer(make([]byte, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		var event struct {
			Type     string `json:"type"`
			ThreadID string `json:"thread_id"`
			Item     struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"item"`
			Usage struct {
				InputTokens  *int `json:"input_tokens"`
				OutputTokens *int `json:"output_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return integration.RunResult{}, fmt.Errorf("parse codex JSONL event: %w", err)
		}
		if event.ThreadID != "" {
			sessionID = event.ThreadID
		}
		if event.Type == "item.completed" && event.Item.Type == "agent_message" && event.Item.Text != "" {
			lastMessage = event.Item.Text
		}
		if event.Type == "turn.completed" && event.Usage.InputTokens != nil {
			usage = integration.Usage{
				Known:        true,
				InputTokens:  event.Usage.InputTokens,
				OutputTokens: event.Usage.OutputTokens,
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return integration.RunResult{}, err
	}
	if lastMessage == "" {
		return integration.RunResult{}, fmt.Errorf("codex returned no agent message")
	}
	output := map[string]any{"result": lastMessage}
	var structured map[string]any
	if json.Unmarshal([]byte(lastMessage), &structured) == nil && structured != nil {
		output = structured
	}
	if sessionID == "" {
		sessionID = request.SessionID
	}
	exit := 0
	return integration.RunResult{
		Output:     output,
		ExitStatus: &exit,
		Usage:      usage,
		SessionID:  sessionID,
		Simulated:  false,
	}, nil
}

func (p *Provider) Cancel(_ context.Context, taskID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	cancel, ok := p.cancels[taskID]
	if !ok {
		return fmt.Errorf("no active codex invocation for %q", taskID)
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
	s = strings.TrimSpace(s)
	if len(s) > 500 {
		s = s[:500] + "…"
	}
	return s
}
