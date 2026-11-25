package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	sqliteadapter "github.com/yoosuf/ae.sh/internal/adapters/sqlite"
	"github.com/yoosuf/ae.sh/internal/config"
	"github.com/yoosuf/ae.sh/internal/integration"
	"github.com/yoosuf/ae.sh/internal/routing"
	"github.com/yoosuf/ae.sh/internal/util"
)

const (
	maxChatTurns       = 20
	defaultChatTimeout = 5 * time.Minute
	maxChatTimeout     = 30 * time.Minute
)

type chatScript struct {
	Messages []string `json:"messages"`
}

type chatTurn struct {
	Index            int               `json:"index"`
	Prompt           string            `json:"prompt"`
	Response         string            `json:"response"`
	SessionID        string            `json:"sessionId"`
	RoutingProfile   string            `json:"routingProfile,omitempty"`
	RequestedModel   string            `json:"requestedModel,omitempty"`
	Model            string            `json:"model,omitempty"`
	Simulated        bool              `json:"simulated"`
	Usage            integration.Usage `json:"usage"`
	EvidenceRecordID string            `json:"evidenceRecordId,omitempty"`
}

type chatResult struct {
	Provider       string     `json:"provider"`
	SessionID      string     `json:"sessionId,omitempty"`
	Workspace      string     `json:"workspace,omitempty"`
	RoutingProfile string     `json:"routingProfile,omitempty"`
	Turns          []chatTurn `json:"turns"`
	DryRun         bool       `json:"dryRun"`
	Note           string     `json:"note"`
}

// cmdChat runs a bounded, opt-in conversation through a provider adapter. Each
// turn is sanitized; provider output remains untrusted text until independently
// verified. No live call happens without an explicit provider and --confirm-live.
func (a *App) cmdChat(ctx context.Context, args []string) error {
	fs := newFlagSet("chat")
	providerName := fs.String("provider", "", "fake, claude, or codex (required)")
	message := fs.String("message", "", "single chat message")
	script := fs.String("script", "", "JSON file with a messages array")
	sessionID := fs.String("session", "", "resume an existing provider session")
	runID := fs.String("run", "", "optional run id for evidence persistence")
	workspaceFlag := fs.String("workspace", "", "managed AE worktree for live providers")
	route := fs.String("route", "", "routing profile: fast, balanced, deep-reasoning, independent-review, recovery")
	dryRun := fs.Bool("dry-run", false, "preview the conversation without calling a provider")
	confirmLive := fs.Bool("confirm-live", false, "authorize a live provider call")
	timeoutRaw := fs.String("timeout", defaultChatTimeout.String(), "per-turn timeout, e.g. 5m")
	if err := fs.Parse(args); err != nil {
		return usage("chat: %v", err)
	}
	if *providerName == "" {
		return usage("chat requires an explicit --provider")
	}
	if (*message == "") == (*script == "") {
		return usage("chat requires exactly one of --message or --script")
	}
	timeout, err := time.ParseDuration(*timeoutRaw)
	if err != nil || timeout < 10*time.Second || timeout > maxChatTimeout {
		return usage("chat --timeout must be between 10s and %s", maxChatTimeout)
	}
	prompts, err := chatPrompts(*message, *script)
	if err != nil {
		return invalid("%v", err)
	}
	provider, err := newProvider(*providerName)
	if err != nil {
		return err
	}
	cfg, err := a.loadConfig()
	if err != nil {
		return err
	}
	profile, err := routing.Normalize(*route)
	if err != nil {
		return invalid("chat --route: %v", err)
	}
	requestedModel := ""
	if provider.Name() != "fake" {
		requestedModel, err = routing.ModelFor(cfg.Provider.Routes, provider.Name(), profile)
		if err != nil {
			return invalid("chat route: %v", err)
		}
	}
	workspace := cfg.Root
	if *workspaceFlag != "" {
		workspace, err = validateManagedWorktree(ctx, cfg.Root, *workspaceFlag)
		if err != nil {
			return err
		}
	}
	if provider.Capabilities().Live && !*dryRun {
		if !*confirmLive {
			return &PolicyError{Message: "live provider chat requires explicit --confirm-live; use --dry-run to preview"}
		}
		if *workspaceFlag == "" {
			return &PolicyError{Message: "live provider chat requires --workspace pointing to a managed AE worktree"}
		}
	}
	result := chatResult{
		Provider:       provider.Name(),
		SessionID:      *sessionID,
		Workspace:      workspace,
		RoutingProfile: profile,
		Turns:          []chatTurn{},
		DryRun:         *dryRun,
		Note:           "Provider output is untrusted text; deterministic checks and independent review are still required.",
	}
	if *dryRun {
		for i, prompt := range prompts {
			result.Turns = append(result.Turns, chatTurn{Index: i + 1, Prompt: prompt})
		}
		a.emit(result)
		return nil
	}
	var chatCfg config.Config
	var chatStore *sqliteadapter.Store
	if *runID != "" {
		chatCfg, chatStore, err = a.openStore()
		if err != nil {
			return err
		}
		defer chatStore.Close()
		if _, err := chatStore.GetRun(ctx, *runID); err != nil {
			return invalid("chat run: %v", err)
		}
	}
	engineRunID := *runID
	if engineRunID == "" {
		engineRunID = "chat-local"
	}
	for i, prompt := range prompts {
		request := integration.RunRequest{
			RunID:          engineRunID,
			TaskID:         fmt.Sprintf("chat-%03d", i+1),
			Skill:          "provider-chat",
			Prompt:         util.Sanitize(prompt),
			Workspace:      workspace,
			Timeout:        timeout,
			SessionID:      result.SessionID,
			Persistent:     true,
			RoutingProfile: profile,
			RequestedModel: requestedModel,
		}
		response, err := provider.Run(ctx, request)
		if err != nil {
			return fmt.Errorf("chat turn %d: %w", i+1, err)
		}
		if response.ExitStatus != nil && *response.ExitStatus != 0 {
			return fmt.Errorf("chat turn %d: provider exit status %d", i+1, *response.ExitStatus)
		}
		if provider.Capabilities().SessionMetadata {
			if response.SessionID == "" {
				return fmt.Errorf("chat turn %d: provider did not return a session id", i+1)
			}
			result.SessionID = response.SessionID
		}
		turn := chatTurn{
			Index:          i + 1,
			Prompt:         request.Prompt,
			Response:       util.Sanitize(renderChatOutput(response.Output)),
			SessionID:      result.SessionID,
			RoutingProfile: profile,
			RequestedModel: response.RequestedModel,
			Model:          response.Model,
			Simulated:      response.Simulated,
			Usage:          response.Usage,
		}
		if *runID != "" {
			record, err := appendRunEvidence(ctx, chatCfg, chatStore, *runID, "provider", fmt.Sprintf("Chat turn %d", turn.Index), "ae-chat", map[string]any{
				"turn":           turn.Index,
				"provider":       provider.Name(),
				"sessionId":      turn.SessionID,
				"prompt":         turn.Prompt,
				"response":       turn.Response,
				"routingProfile": turn.RoutingProfile,
				"requestedModel": turn.RequestedModel,
				"model":          turn.Model,
				"simulated":      turn.Simulated,
				"usage":          turn.Usage,
				"note":           result.Note,
			}, response.ExitStatus, &response.Usage)
			if err != nil {
				return err
			}
			turn.EvidenceRecordID = record.RecordID
		}
		result.Turns = append(result.Turns, turn)
	}
	a.emit(result)
	return nil
}

func chatPrompts(message, script string) ([]string, error) {
	if script == "" {
		value := util.Sanitize(strings.TrimSpace(message))
		if value == "" {
			return nil, fmt.Errorf("chat message is empty")
		}
		return []string{value}, nil
	}
	raw, err := os.ReadFile(script)
	if err != nil {
		return nil, err
	}
	var doc chatScript
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse chat script: %w", err)
	}
	if len(doc.Messages) == 0 || len(doc.Messages) > maxChatTurns {
		return nil, fmt.Errorf("chat script must contain between 1 and %d messages", maxChatTurns)
	}
	prompts := make([]string, 0, len(doc.Messages))
	for _, value := range doc.Messages {
		value = util.Sanitize(strings.TrimSpace(value))
		if value == "" {
			return nil, fmt.Errorf("chat script contains an empty message")
		}
		prompts = append(prompts, value)
	}
	return prompts, nil
}

func renderChatOutput(output map[string]any) string {
	if value, ok := output["result"].(string); ok && value != "" {
		return value
	}
	raw, err := json.Marshal(output)
	if err != nil {
		return fmt.Sprintf("%v", output)
	}
	return string(raw)
}
