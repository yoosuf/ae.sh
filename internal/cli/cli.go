// Package cli implements the aef command line interface. It is discoverable,
// local-first, and uses consistent exit codes and opt-in JSON output.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yoosuf/ae.sh/internal/adapters/claude"
	"github.com/yoosuf/ae.sh/internal/adapters/codex"
	"github.com/yoosuf/ae.sh/internal/adapters/fake"
	sqliteadapter "github.com/yoosuf/ae.sh/internal/adapters/sqlite"
	"github.com/yoosuf/ae.sh/internal/catalog"
	"github.com/yoosuf/ae.sh/internal/config"
	"github.com/yoosuf/ae.sh/internal/evidence"
	"github.com/yoosuf/ae.sh/internal/execution"
	"github.com/yoosuf/ae.sh/internal/framework"
	"github.com/yoosuf/ae.sh/internal/integration"
	"github.com/yoosuf/ae.sh/internal/policy"
	"github.com/yoosuf/ae.sh/internal/version"
	"github.com/yoosuf/ae.sh/internal/workflow"
)

// Exit codes.
const (
	ExitOK         = 0
	ExitError      = 1
	ExitUsage      = 2
	ExitValidation = 3
	ExitPolicy     = 4
	ExitConflict   = 5
)

// App is the CLI runtime.
type App struct {
	Stdout io.Writer
	Stderr io.Writer
	Args   []string
	JSON   bool
	Root   string
}

// Run dispatches a command and returns a process exit code.
func (a *App) Run(ctx context.Context) int {
	args := a.splitGlobals(a.Args)
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		a.printHelp()
		return ExitOK
	}
	command := args[0]
	args = args[1:]
	var err error
	switch command {
	case "help":
		a.printHelp()
	case "init":
		err = a.cmdInit(args)
	case "doctor":
		err = a.cmdDoctor(ctx, args)
	case "validate":
		err = a.cmdValidate(args)
	case "skills":
		err = a.cmdSkills(args)
	case "plan":
		err = a.cmdPlan(ctx, args)
	case "run":
		err = a.cmdRun(ctx, args)
	case "status":
		err = a.cmdStatus(ctx, args)
	case "resume":
		err = a.cmdResume(ctx, args)
	case "cancel":
		err = a.cmdCancel(ctx, args)
	case "approve":
		err = a.cmdApprove(ctx, args)
	case "review":
		err = a.cmdReview(ctx, args)
	case "chat":
		err = a.cmdChat(ctx, args)
	case "autopilot":
		err = a.cmdAutopilot(ctx, args)
	case "verify":
		err = a.cmdVerify(ctx, args)
	case "worktree":
		err = a.cmdWorktree(ctx, args)
	case "trace":
		err = a.cmdTrace(ctx, args)
	case "evidence":
		err = a.cmdEvidence(ctx, args)
	case "eval":
		err = a.cmdEval(args)
	case "version":
		a.emit(map[string]any{"version": version.Version, "schemaVersion": version.SchemaVersion})
	default:
		err = fmt.Errorf("unknown command %q (try `aef help`)", command)
	}
	if err == nil {
		return ExitOK
	}
	return a.handleError(err)
}

func (a *App) handleError(err error) int {
	var usage *UsageError
	var validation *ValidationError
	var denied *PolicyError
	var conflict *ConflictError
	switch {
	case errors.As(err, &usage):
		if a.JSON {
			a.emit(map[string]any{"error": err.Error(), "exitCode": ExitUsage})
		} else {
			fmt.Fprintln(a.Stderr, err)
		}
		return ExitUsage
	case errors.As(err, &denied):
		if a.JSON {
			a.emit(map[string]any{"error": err.Error(), "exitCode": ExitPolicy})
		} else {
			fmt.Fprintln(a.Stderr, err)
		}
		return ExitPolicy
	case errors.As(err, &conflict):
		if a.JSON {
			a.emit(map[string]any{"error": err.Error(), "exitCode": ExitConflict})
		} else {
			fmt.Fprintln(a.Stderr, err)
		}
		return ExitConflict
	case errors.As(err, &validation):
		if a.JSON {
			a.emit(map[string]any{"error": err.Error(), "exitCode": ExitValidation})
		} else {
			fmt.Fprintln(a.Stderr, err)
		}
		return ExitValidation
	default:
		if a.JSON {
			a.emit(map[string]any{"error": err.Error(), "exitCode": ExitError})
		} else {
			fmt.Fprintln(a.Stderr, err)
		}
		return ExitError
	}
}

// splitGlobals extracts flags that apply to every command. Keeping them out of
// subcommand FlagSets avoids surprising interactions with Go's flag package.
func (a *App) splitGlobals(raw []string) []string {
	out := make([]string, 0, len(raw))
	for i := 0; i < len(raw); i++ {
		switch {
		case raw[i] == "--json":
			a.JSON = true
		case raw[i] == "--root" && i+1 < len(raw):
			a.Root = raw[i+1]
			i++
		case strings.HasPrefix(raw[i], "--root="):
			a.Root = strings.TrimPrefix(raw[i], "--root=")
		default:
			out = append(out, raw[i])
		}
	}
	return out
}

// Typed errors for consistent exit codes.
type UsageError struct{ Message string }

func (e *UsageError) Error() string { return e.Message }

type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }

type PolicyError struct{ Message string }

func (e *PolicyError) Error() string { return e.Message }

type ConflictError struct{ Message string }

func (e *ConflictError) Error() string { return e.Message }

func usage(format string, args ...any) error {
	return &UsageError{Message: fmt.Sprintf(format, args...)}
}
func invalid(format string, args ...any) error {
	return &ValidationError{Message: fmt.Sprintf(format, args...)}
}

func (a *App) printHelp() {
	fmt.Fprint(a.Stdout, `aef - Agentic Engineering Framework

Usage: aef [--root DIR] [--json] <command> [options]

Commands:
  init --profile go,react [--force]
  doctor
  validate
  skills list
  skills install --provider claude|codex|all --scope user|project [--version v] [--dry-run] [--force]
  skills uninstall --provider claude|codex --scope user|project [--force]
  skills export --out dist/plugins --provider claude|codex|all
  plan --issue PROJ-101|--issue path/to/issue.json
  run <workflow> --issue PROJ-101 [--issue path] [--provider fake|claude|codex] [--dry-run] [--actor name] [--confirm-live] [--workspace DIR] [--allow-writes]
  status <run-id> | status --list
  resume <run-id> [--provider fake|claude|codex] [--confirm-live] [--allow-writes]
  cancel <run-id> --reason text
  approve <run-id> --gate architecture --approver name [--role role]
  review --diff path/to.diff [--pr 123] [--run run-id] [--candidate-sha sha] [--base-sha sha]
  review resolve --run run-id --finding rev-... --status fixed|accepted|false-positive --reason text [--actor name]
  chat --provider fake|claude|codex --message text|--script file.json [--session id] [--run run-id] [--workspace DIR] [--dry-run] [--confirm-live]
  autopilot --issue key|fixture | --resume run-id [--workflow feature-development] [--provider fake|claude|codex] [--approver name] [--confirm-gates] [--confirm-live] [--allow-writes] [--github-pr ref] [--comment --confirm-mutate] [--dry-run]
  verify <run-id> [--profile go,react]
  worktree prepare <run-id> | worktree clean <run-id>
  trace <run-id> [--format json|markdown] [--out FILE]
  evidence export <run-id> --format json|markdown|both [--out dir]
  eval --suite security|skills|all [--execute]
  version

Exit codes: 0 ok, 1 error, 2 usage, 3 validation, 4 policy denied, 5 conflict.
`)
}

func (a *App) emit(value any) {
	if a.JSON {
		raw, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			fmt.Fprintln(a.Stdout, `{"error":"unserializable output"}`)
			return
		}
		fmt.Fprintln(a.Stdout, string(raw))
		return
	}
	switch v := value.(type) {
	case string:
		fmt.Fprintln(a.Stdout, v)
	default:
		raw, err := json.MarshalIndent(v, "", "  ")
		if err == nil {
			fmt.Fprintln(a.Stdout, string(raw))
		}
	}
}

func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

func (a *App) loadConfig() (config.Config, error) {
	root := a.Root
	if root == "" {
		var err error
		root, err = os.Getwd()
		if err != nil {
			return config.Config{}, err
		}
	}
	path, err := config.Find(root)
	if err != nil {
		return config.Config{}, usage("%v", err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		return config.Config{}, invalid("%v", err)
	}
	return cfg, nil
}

type repoContext struct {
	Config    config.Config
	Catalogue catalog.Catalogue
	Workflows []workflow.Workflow
	Policies  policy.Bundle
	Agents    map[string]bool
}

func (a *App) loadRepo() (repoContext, error) {
	cfg, err := a.loadConfig()
	if err != nil {
		return repoContext{}, err
	}
	cat, err := catalog.Load(cfg.Root)
	if err != nil {
		return repoContext{}, invalid("%v", err)
	}
	if broken := catBrokenReferences(cat); len(broken) > 0 {
		return repoContext{}, invalid("broken skill references: %s", strings.Join(broken, ", "))
	}
	agents := map[string]bool{}
	agentDir := framework.Dir(cfg.Root, framework.Agents)
	entries, err := os.ReadDir(agentDir)
	if err == nil {
		for _, entry := range entries {
			if !entry.IsDir() && (strings.HasSuffix(entry.Name(), ".yaml") || strings.HasSuffix(entry.Name(), ".yml")) {
				agents[strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))] = true
			}
		}
	}
	workflows, err := workflow.LoadDirectory(framework.Dir(cfg.Root, framework.Workflows), cat.BySlugSet(), agents)
	if err != nil {
		return repoContext{}, invalid("%v", err)
	}
	bundle, err := policy.LoadBundle(cfg.Root, []string{
		framework.Relative(framework.Policies, "default.yaml"),
		framework.Relative(framework.Policies, "high-risk.yaml"),
	})
	if err != nil {
		return repoContext{}, invalid("%v", err)
	}
	return repoContext{Config: cfg, Catalogue: cat, Workflows: workflows, Policies: bundle, Agents: agents}, nil
}

func catBrokenReferences(cat catalog.Catalogue) []string {
	var out []string
	for _, skill := range cat.Skills {
		for _, ref := range catalog.ValidateReferences(skill) {
			out = append(out, skill.Slug+": "+ref)
		}
	}
	sort.Strings(out)
	return out
}

func newProvider(name string) (integration.Provider, error) {
	switch name {
	case "fake":
		return fake.New(), nil
	case "claude":
		return claude.New(), nil
	case "codex":
		return codex.New(), nil
	default:
		return nil, usage("unsupported provider %q", name)
	}
}

func (a *App) buildEngine(ctx repoContext, providerName string) (*execution.Engine, *sqliteadapter.Store, error) {
	store, err := sqliteadapter.Open(ctx.Config.StatePath())
	if err != nil {
		return nil, nil, err
	}
	engine, err := a.buildEngineWithStore(ctx, providerName, store)
	if err != nil {
		store.Close()
		return nil, nil, err
	}
	return engine, store, nil
}

func (a *App) buildEngineWithStore(ctx repoContext, providerName string, store *sqliteadapter.Store) (*execution.Engine, error) {
	if providerName == "" {
		providerName = ctx.Config.Provider.Default
	}
	provider, err := newProvider(providerName)
	if err != nil {
		return nil, err
	}
	return &execution.Engine{
		Store:       store,
		Policies:    ctx.Policies,
		Provider:    provider,
		Evidence:    evidence.New(ctx.Config.EvidenceDir(), ctx.Config.ObjectsDir()),
		Root:        ctx.Config.Root,
		KnownSkills: ctx.Catalogue.BySlugSet(),
		KnownAgents: ctx.Agents,
	}, nil
}
