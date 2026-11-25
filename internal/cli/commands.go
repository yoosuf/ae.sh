package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/yoosuf/ae.sh/internal/adapters/jira"
	sqliteadapter "github.com/yoosuf/ae.sh/internal/adapters/sqlite"
	"github.com/yoosuf/ae.sh/internal/catalog"
	"github.com/yoosuf/ae.sh/internal/config"
	"github.com/yoosuf/ae.sh/internal/evidence"
	"github.com/yoosuf/ae.sh/internal/execution"
	"github.com/yoosuf/ae.sh/internal/framework"
	"github.com/yoosuf/ae.sh/internal/integration"
	"github.com/yoosuf/ae.sh/internal/schema"
	"github.com/yoosuf/ae.sh/internal/util"
	"github.com/yoosuf/ae.sh/internal/version"
	"github.com/yoosuf/ae.sh/internal/workflow"
)

func (a *App) cmdInit(args []string) error {
	fs := newFlagSet("init")
	profileFlag := fs.String("profile", "", "comma-separated profiles ("+strings.Join(config.KnownProfiles(), ",")+")")
	force := fs.Bool("force", false, "overwrite an existing aef.yaml")
	if err := fs.Parse(args); err != nil {
		return usage("init: %v", err)
	}
	root := a.Root
	if root == "" {
		var err error
		root, err = os.Getwd()
		if err != nil {
			return err
		}
	}
	cfg := config.Default()
	if *profileFlag != "" {
		seen := map[string]bool{}
		for _, p := range strings.Split(*profileFlag, ",") {
			p = strings.TrimSpace(p)
			if p == "" || seen[p] {
				continue
			}
			if !config.IsKnownProfile(p) {
				return usage("unknown profile %q (known: %s)", p, strings.Join(config.KnownProfiles(), ", "))
			}
			seen[p] = true
			cfg.Profiles = append(cfg.Profiles, p)
		}
	}
	if info, err := os.Stat(framework.Dir(root, framework.Skills)); err != nil || !info.IsDir() {
		return invalid("canonical framework assets not found under %s; run init from the AEF checkout or pass --root to that checkout", root)
	}
	path := filepath.Join(root, config.FileName)
	if err := config.Write(path, cfg, *force); err != nil {
		return invalid("%v", err)
	}
	for _, dir := range []string{filepath.Join(root, ".aef"), cfg.EvidenceDir(), cfg.ObjectsDir()} {
		if err := util.EnsureDir(dir); err != nil {
			return err
		}
	}
	a.emit(map[string]any{"created": filepath.ToSlash(path), "profiles": cfg.Profiles, "provider": cfg.Provider.Default})
	return nil
}

func (a *App) cmdValidate(args []string) error {
	if len(args) > 0 {
		return usage("validate takes no positional arguments")
	}
	ctx, err := a.loadRepo()
	if err != nil {
		return err
	}
	workflowIDs := make([]string, 0, len(ctx.Workflows))
	for _, wf := range ctx.Workflows {
		workflowIDs = append(workflowIDs, wf.ID)
	}
	for _, name := range config.KnownProfiles() {
		if _, err := config.LoadProfile(ctx.Config.Root, name); err != nil {
			return invalid("profile %s: %v", name, err)
		}
	}
	a.emit(map[string]any{
		"status":        "valid",
		"schemaVersion": version.SchemaVersion,
		"skills":        ctx.Catalogue.Slugs(),
		"workflows":     workflowIDs,
		"policies":      len(ctx.Policies.Policies),
		"agents":        len(ctx.Agents),
		"profiles":      config.KnownProfiles(),
		"evaluations":   "authored; no behavioral/model evaluation executed",
	})
	return nil
}

func (a *App) cmdSkills(args []string) error {
	if len(args) == 0 {
		return usage("skills requires a subcommand: list, install, uninstall, export")
	}
	ctx, err := a.loadRepo()
	if err != nil {
		return err
	}
	switch args[0] {
	case "list":
		if len(args) != 1 {
			return usage("skills list takes no arguments")
		}
		var rows []map[string]any
		for _, skill := range ctx.Catalogue.Skills {
			rows = append(rows, map[string]any{
				"slug":    skill.Slug,
				"domain":  skill.Metadata["aef.domain"],
				"version": skill.Metadata["aef.version"],
				"fixture": skill.Metadata["aef.eval-fixture"],
			})
		}
		a.emit(rows)
		return nil
	case "install":
		fs := newFlagSet("skills install")
		provider := fs.String("provider", "all", "claude, codex, or all")
		scope := fs.String("scope", "user", "user or project")
		catVersion := fs.String("version", version.SupportedSkillsVersion, "catalogue version")
		dryRun := fs.Bool("dry-run", false, "preview without writing")
		force := fs.Bool("force", false, "overwrite divergent skill files")
		if err := fs.Parse(args[1:]); err != nil {
			return usage("skills install: %v", err)
		}
		opts := catalog.InstallOptions{
			Provider: *provider, Scope: *scope, ProjectDir: ctx.Config.Root,
			Version: *catVersion, DryRun: *dryRun, Force: *force,
		}
		plans, err := catalog.PlanInstall(ctx.Catalogue, opts)
		if err != nil {
			return usage("%v", err)
		}
		if *dryRun {
			a.emit(plans)
			return nil
		}
		for _, plan := range plans {
			if len(plan.Blocked) > 0 {
				return &PolicyError{Message: "install blocked: " + strings.Join(plan.Blocked, "; ")}
			}
			if err := catalog.Apply(ctx.Catalogue, plan, opts); err != nil {
				return err
			}
		}
		a.emit(map[string]any{"installed": true, "providers": strings.Split(*provider, ","), "scope": *scope,
			"manifest": plans[0].ManifestPath})
		return nil
	case "uninstall":
		fs := newFlagSet("skills uninstall")
		provider := fs.String("provider", "claude", "claude or codex")
		scope := fs.String("scope", "user", "user or project")
		force := fs.Bool("force", false, "remove despite unmanaged files")
		if err := fs.Parse(args[1:]); err != nil {
			return usage("skills uninstall: %v", err)
		}
		base := ""
		home, _ := os.UserHomeDir()
		if *provider == "claude" && *scope == "user" {
			base = filepath.Join(home, ".claude", "skills")
		} else if *provider == "claude" {
			base = filepath.Join(ctx.Config.Root, ".claude", "skills")
		} else if *provider == "codex" && *scope == "user" {
			base = filepath.Join(home, ".agents", "skills")
		} else if *provider == "codex" {
			base = filepath.Join(ctx.Config.Root, ".agents", "skills")
		} else {
			return usage("unsupported provider %q", *provider)
		}
		removed, err := catalog.Uninstall(filepath.Join(base, ".aef-manifest.json"), *force)
		if err != nil {
			return &PolicyError{Message: err.Error()}
		}
		a.emit(map[string]any{"removed": len(removed), "paths": removed})
		return nil
	case "export":
		fs := newFlagSet("skills export")
		out := fs.String("out", "dist/plugins", "output directory")
		provider := fs.String("provider", "all", "claude, codex, or all")
		if err := fs.Parse(args[1:]); err != nil {
			return usage("skills export: %v", err)
		}
		providers := []string{*provider}
		if *provider == "all" {
			providers = []string{"claude", "codex"}
		}
		var dirs []string
		for _, p := range providers {
			dir, err := catalog.ExportPlugin(ctx.Catalogue, filepath.Join(ctx.Config.Root, *out), p)
			if err != nil {
				return err
			}
			dirs = append(dirs, dir)
		}
		a.emit(map[string]any{"bundles": dirs, "note": "Generated bundles are not published; validate with the provider CLI before enabling."})
		return nil
	default:
		return usage("unknown skills subcommand %q", args[0])
	}
}

// resolveIssueWithLive keeps dry runs free of network calls while allowing
// configured Jira Cloud intake on explicit live paths.
func (a *App) resolveIssueWithLive(callCtx context.Context, repo repoContext, value string, allowLive bool) (integration.Issue, error) {
	if value == "" {
		return integration.Issue{}, usage("--issue is required")
	}
	path := value
	if _, err := os.Stat(path); err != nil {
		candidate := filepath.Join(repo.Config.Root, "examples", "offline-workflow", "issue-"+value+".json")
		if _, statErr := os.Stat(candidate); statErr == nil {
			path = candidate
		} else {
			if repo.Config.Integrations.Jira.Mode == "jira-cloud" {
				if !allowLive {
					return integration.Issue{}, &PolicyError{Message: "live Jira issue intake requires --confirm-live on an authorized live command; dry runs use fixtures"}
				}
				key, err := jira.ParseIssue(value, repo.Config.Integrations.Jira)
				if err != nil {
					return integration.Issue{}, invalid("%v", err)
				}
				source, err := jira.New(repo.Config.Integrations.Jira)
				if err != nil {
					return integration.Issue{}, invalid("%v", err)
				}
				issue, err := source.Load(callCtx, key)
				if err != nil {
					return integration.Issue{}, invalid("%v", err)
				}
				return issue, nil
			}
			if strings.Contains(value, "://") {
				return integration.Issue{}, &PolicyError{Message: "Jira links require integrations.jira.mode: jira-cloud; fixtures remain the offline default"}
			}
			return integration.Issue{}, invalid("issue fixture not found for %q", value)
		}
	}
	issue, err := integration.LoadIssue(path)
	if err != nil {
		return integration.Issue{}, invalid("%v", err)
	}
	return issue, nil
}

func (a *App) findWorkflow(ctx repoContext, id string) (workflow.Workflow, error) {
	for _, wf := range ctx.Workflows {
		if wf.ID == id {
			return wf, nil
		}
	}
	return workflow.Workflow{}, invalid("workflow %q not found", id)
}

func (a *App) cmdPlan(ctx context.Context, args []string) error {
	fs := newFlagSet("plan")
	issueFlag := fs.String("issue", "", "issue key or fixture path")
	if err := fs.Parse(args); err != nil {
		return usage("plan: %v", err)
	}
	repo, err := a.loadRepo()
	if err != nil {
		return err
	}
	issue, err := a.resolveIssueWithLive(ctx, repo, *issueFlag, false)
	if err != nil {
		return err
	}
	wf, err := a.findWorkflow(repo, "feature-development")
	if err != nil {
		return err
	}
	engine := &execution.Engine{
		Policies:    repo.Policies,
		Root:        repo.Config.Root,
		KnownSkills: repo.Catalogue.BySlugSet(),
		KnownAgents: repo.Agents,
	}
	_, preview, err := engine.Start(ctx, execution.StartOptions{
		Workflow: wf, Issue: issue, Provider: "fake", Actor: "plan", DryRun: true,
	})
	if err != nil {
		return invalid("%v", err)
	}
	a.emit(preview)
	return nil
}

func (a *App) cmdRun(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return usage("run requires a workflow id")
	}
	id := args[0]
	fs := newFlagSet("run")
	issueFlag := fs.String("issue", "", "issue key or fixture path")
	providerFlag := fs.String("provider", "", "fake, claude, or codex")
	dryRun := fs.Bool("dry-run", false, "preview without writes")
	actor := fs.String("actor", "local-user", "approver identity for this run")
	workspaceFlag := fs.String("workspace", "", "prepared AEF worktree to use (aef worktree prepare <run-id>)")
	confirmLive := fs.Bool("confirm-live", false, "authorize live provider calls")
	allowWrites := fs.Bool("allow-writes", false, "allow provider file edits inside a managed worktree")
	if err := fs.Parse(args[1:]); err != nil {
		return usage("run: %v", err)
	}
	repo, err := a.loadRepo()
	if err != nil {
		return err
	}
	issue, err := a.resolveIssueWithLive(ctx, repo, *issueFlag, *confirmLive && !*dryRun)
	if err != nil {
		return err
	}
	wf, err := a.findWorkflow(repo, id)
	if err != nil {
		return err
	}
	providerName := *providerFlag
	if providerName == "" {
		providerName = repo.Config.Provider.Default
	}
	if providerName != "fake" && !*dryRun && !*confirmLive {
		return &PolicyError{Message: "live provider runs require --confirm-live and a managed AEF worktree; use --dry-run to preview"}
	}
	workspace := ""
	if *workspaceFlag != "" {
		workspace, err = validateManagedWorktree(ctx, repo.Config.Root, *workspaceFlag)
		if err != nil {
			return err
		}
	}
	if providerName != "fake" && !*dryRun && workspace == "" {
		return &PolicyError{Message: "live provider runs require --workspace pointing to a managed AEF worktree"}
	}
	if *allowWrites && (providerName == "fake" || workspace == "") {
		return &PolicyError{Message: "--allow-writes requires a live provider and a managed AEF worktree"}
	}
	engine, store, err := a.buildEngine(repo, providerName)
	if err != nil {
		return err
	}
	defer store.Close()
	engine.AllowWrites = *allowWrites && !*dryRun
	run, preview, err := engine.Start(ctx, execution.StartOptions{
		Workflow: wf, Issue: issue, Provider: providerName, Actor: *actor,
		DryRun: *dryRun, Profiles: repo.Config.Profiles, Workspace: workspace,
	})
	if *dryRun {
		if err != nil {
			return invalid("%v", err)
		}
		a.emit(preview)
		return nil
	}
	if err != nil {
		if strings.Contains(err.Error(), "blocked") {
			return &PolicyError{Message: err.Error()}
		}
		return err
	}
	a.emit(run)
	return nil
}

func (a *App) openStore() (config.Config, *sqliteadapter.Store, error) {
	cfg, err := a.loadConfig()
	if err != nil {
		return config.Config{}, nil, err
	}
	store, err := sqliteadapter.Open(cfg.StatePath())
	if err != nil {
		return config.Config{}, nil, err
	}
	return cfg, store, nil
}

func (a *App) cmdStatus(ctx context.Context, args []string) error {
	fs := newFlagSet("status")
	list := fs.Bool("list", false, "list recent runs")
	if err := fs.Parse(args); err != nil {
		return usage("status: %v", err)
	}
	if *list && len(fs.Args()) > 0 {
		return usage("status --list takes no run id")
	}
	if !*list && len(fs.Args()) != 1 {
		return usage("status requires a run id or --list")
	}
	_, store, err := a.openStore()
	if err != nil {
		return err
	}
	defer store.Close()
	if *list {
		runs, err := store.ListRuns(ctx, 50)
		if err != nil {
			return err
		}
		a.emit(runs)
		return nil
	}
	run, err := store.GetRun(ctx, fs.Arg(0))
	if err != nil {
		return a.notFound(err)
	}
	transitions, err := store.Transitions(ctx, run.ID)
	if err != nil {
		return err
	}
	a.emit(map[string]any{"run": run, "transitions": transitions})
	return nil
}

func (a *App) notFound(err error) error {
	if _, ok := err.(execution.ErrNotFound); ok {
		return invalid("%v", err)
	}
	return err
}

func (a *App) cmdResume(ctx context.Context, args []string) error {
	if len(args) < 1 {
		return usage("resume requires a run id")
	}
	fs := newFlagSet("resume")
	providerFlag := fs.String("provider", "", "provider for this continuation (defaults to config)")
	confirmLive := fs.Bool("confirm-live", false, "authorize live provider calls")
	allowWrites := fs.Bool("allow-writes", false, "allow provider file edits inside a managed worktree")
	if err := fs.Parse(args[1:]); err != nil {
		return usage("resume: %v", err)
	}
	if fs.NArg() != 0 {
		return usage("resume accepts exactly one run id")
	}
	runID := args[0]
	repo, err := a.loadRepo()
	if err != nil {
		return err
	}
	providerName := *providerFlag
	if providerName == "" {
		providerName = repo.Config.Provider.Default
	}
	engine, store, err := a.buildEngine(repo, providerName)
	if err != nil {
		return err
	}
	defer store.Close()
	run, err := store.GetRun(ctx, runID)
	if err != nil {
		return a.notFound(err)
	}
	if providerName != "fake" && !*confirmLive {
		return &PolicyError{Message: "live provider runs require --confirm-live; use --dry-run on a new run to preview"}
	}
	if providerName != "fake" && run.Workspace == "" {
		return &PolicyError{Message: "live provider runs require a run with a managed AEF worktree"}
	}
	if *allowWrites && (providerName == "fake" || run.Workspace == "") {
		return &PolicyError{Message: "--allow-writes requires a live provider and a managed AEF worktree"}
	}
	if run.Workspace != "" {
		if _, err := validateManagedWorktree(ctx, repo.Config.Root, run.Workspace); err != nil {
			return err
		}
	}
	engine.AllowWrites = *allowWrites
	run, err = engine.Continue(ctx, runID)
	if err != nil {
		if strings.Contains(err.Error(), "blocked") {
			return &PolicyError{Message: err.Error()}
		}
		return err
	}
	a.emit(run)
	return nil
}

func (a *App) cmdCancel(ctx context.Context, args []string) error {
	if len(args) < 1 {
		return usage("cancel requires a run id")
	}
	fs := newFlagSet("cancel")
	reason := fs.String("reason", "cancelled by operator", "reason for cancellation")
	if err := fs.Parse(args[1:]); err != nil {
		return usage("cancel: %v", err)
	}
	repo, err := a.loadRepo()
	if err != nil {
		return err
	}
	engine, store, err := a.buildEngine(repo, "")
	if err != nil {
		return err
	}
	defer store.Close()
	if err := engine.Cancel(ctx, args[0], *reason); err != nil {
		return err
	}
	run, err := store.GetRun(ctx, args[0])
	if err != nil {
		return err
	}
	a.emit(run)
	return nil
}

func (a *App) cmdApprove(ctx context.Context, args []string) error {
	if len(args) < 1 {
		return usage("approve requires a run id")
	}
	fs := newFlagSet("approve")
	gate := fs.String("gate", "", "approval gate name")
	approver := fs.String("approver", "", "human approver identity")
	role := fs.String("role", "", "approver role for separation-of-duties checks")
	if err := fs.Parse(args[1:]); err != nil {
		return usage("approve: %v", err)
	}
	if *gate == "" || *approver == "" {
		return usage("approve requires --gate and --approver")
	}
	repo, err := a.loadRepo()
	if err != nil {
		return err
	}
	engine, store, err := a.buildEngine(repo, "")
	if err != nil {
		return err
	}
	defer store.Close()
	if err := engine.Approve(ctx, args[0], *gate, *approver, *role); err != nil {
		return &PolicyError{Message: err.Error()}
	}
	run, err := store.GetRun(ctx, args[0])
	if err != nil {
		return err
	}
	a.emit(run)
	return nil
}

func (a *App) cmdReview(ctx context.Context, args []string) error {
	if len(args) > 0 && args[0] == "resolve" {
		return a.cmdReviewResolve(ctx, args[1:])
	}
	fs := newFlagSet("review")
	pr := fs.Int("pr", 0, "pull request number (evidence only)")
	diff := fs.String("diff", "", "path to a unified diff")
	runID := fs.String("run", "", "optional run id for evidence persistence")
	candidateSHA := fs.String("candidate-sha", "", "candidate commit SHA (evidence only)")
	baseSHA := fs.String("base-sha", "", "base commit SHA (evidence only)")
	if err := fs.Parse(args); err != nil {
		return usage("review: %v", err)
	}
	if *diff == "" {
		return &PolicyError{Message: "live GitHub PR review is not implemented; supply --diff for a local pattern scan"}
	}
	result, err := a.reviewDiff(ctx, *diff, *runID, *pr, *candidateSHA, *baseSHA)
	if err != nil {
		return err
	}
	a.emit(result)
	return nil
}

// reviewDiff scans a local diff and, when runID is set, persists the result as
// chained review evidence. It is shared by `review` and `autopilot`.
func (a *App) reviewDiff(ctx context.Context, diffPath, runID string, pr int, candidateSHA, baseSHA string) (ReviewResult, error) {
	result, err := ScanDiff(diffPath)
	if err != nil {
		return ReviewResult{}, invalid("%v", err)
	}
	result.PR = pr
	result.CandidateSHA = candidateSHA
	result.BaseSHA = baseSHA
	if runID != "" {
		cfg, store, err := a.openStore()
		if err != nil {
			return ReviewResult{}, err
		}
		defer store.Close()
		run, err := store.GetRun(ctx, runID)
		if err != nil {
			return ReviewResult{}, invalid("review run: %v", err)
		}
		if result.CandidateSHA == "" {
			result.CandidateSHA = run.CandidateSHA
		}
		if result.BaseSHA == "" {
			result.BaseSHA = run.BaseSHA
		}
		if err := validateReviewResult(result); err != nil {
			return ReviewResult{}, err
		}
		data, err := util.JSONValue(result)
		if err != nil {
			return ReviewResult{}, err
		}
		record, err := appendRunEvidence(ctx, cfg, store, runID, "review", "Local diff pattern scan", "aef-review", data.(map[string]any), nil, nil)
		if err != nil {
			return ReviewResult{}, err
		}
		result.EvidenceRecordID = record.RecordID
	}
	if err := validateReviewResult(result); err != nil {
		return ReviewResult{}, err
	}
	return result, nil
}

func (a *App) cmdReviewResolve(ctx context.Context, args []string) error {
	fs := newFlagSet("review resolve")
	runID := fs.String("run", "", "run id containing the review finding")
	findingID := fs.String("finding", "", "stable finding id from review output")
	status := fs.String("status", "", "fixed, accepted, or false-positive")
	reason := fs.String("reason", "", "human explanation for the decision")
	actor := fs.String("actor", "local-user", "decision maker")
	if err := fs.Parse(args); err != nil {
		return usage("review resolve: %v", err)
	}
	if *runID == "" || *findingID == "" || *status == "" || *reason == "" {
		return usage("review resolve requires --run, --finding, --status, and --reason")
	}
	switch *status {
	case "fixed", "accepted", "false-positive":
	default:
		return usage("review resolve --status must be fixed, accepted, or false-positive")
	}
	if len(util.Sanitize(*reason)) < 8 {
		return usage("review resolve --reason must be at least 8 characters")
	}
	cfg, store, err := a.openStore()
	if err != nil {
		return err
	}
	defer store.Close()
	run, err := store.GetRun(ctx, *runID)
	if err != nil {
		return invalid("review resolve run: %v", err)
	}
	state, err := execution.CollectReviewEvidence(ctx, store, run)
	if err != nil {
		return invalid("review resolve: %v", err)
	}
	var ref *execution.ReviewFindingRef
	for i := range state.Findings {
		if state.Findings[i].ID == *findingID {
			ref = &state.Findings[i]
			break
		}
	}
	if ref == nil {
		return invalid("review finding %q was not found in run %s", util.Sanitize(*findingID), *runID)
	}
	if _, ok := state.Resolved[*findingID]; ok {
		return &ConflictError{Message: "review finding " + *findingID + " is already resolved"}
	}
	data := map[string]any{
		"findingId":      ref.ID,
		"ruleId":         ref.RuleID,
		"severity":       ref.Severity,
		"reviewRecordID": ref.RecordID,
		"diffDigest":     ref.DiffDigest,
		"candidateSHA":   ref.CandidateSHA,
		"status":         *status,
		"reason":         util.Sanitize(*reason),
		"note":           "Human decision about a local pattern-scan hypothesis.",
	}
	record, err := appendRunEvidence(ctx, cfg, store, *runID, "remediation", "Resolve review finding "+ref.ID, util.Sanitize(*actor), data, nil, nil)
	if err != nil {
		return err
	}
	a.emit(map[string]any{
		"findingId":        ref.ID,
		"status":           *status,
		"evidenceRecordID": record.RecordID,
		"note":             "Resolution is a human decision; the underlying hypothesis is not independently verified by this command.",
	})
	return nil
}

func validateReviewResult(result ReviewResult) error {
	generic, err := util.JSONValue(result)
	if err != nil {
		return invalid("review result: %v", err)
	}
	if err := schema.Validate("review", generic); err != nil {
		return invalid("%v", err)
	}
	return nil
}

func (a *App) cmdEvidence(ctx context.Context, args []string) error {
	if len(args) == 0 || args[0] != "export" {
		return usage("evidence requires: export <run-id>")
	}
	if len(args) < 2 {
		return usage("evidence export requires a run id")
	}
	fs := newFlagSet("evidence export")
	format := fs.String("format", "both", "json, markdown, or both")
	out := fs.String("out", "", "output directory (default .aef/evidence/export)")
	if err := fs.Parse(args[2:]); err != nil {
		return usage("evidence export: %v", err)
	}
	cfg, store, err := a.openStore()
	if err != nil {
		return err
	}
	defer store.Close()
	records, err := store.Evidence(ctx, args[1])
	if err != nil {
		return err
	}
	var parsed []evidence.Record
	for _, item := range records {
		var record evidence.Record
		if err := json.Unmarshal([]byte(item.JSON), &record); err != nil {
			return invalid("stored evidence is malformed: %v", err)
		}
		parsed = append(parsed, record)
	}
	evidence.SortRecords(parsed)
	dir := *out
	if dir == "" {
		dir = filepath.Join(cfg.EvidenceDir(), "export", args[1])
	}
	var files []string
	switch *format {
	case "json", "both":
		path := filepath.Join(dir, "evidence.json")
		if err := evidence.ExportJSON(parsed, path); err != nil {
			return err
		}
		files = append(files, path)
	}
	switch *format {
	case "markdown", "both":
		path := filepath.Join(dir, "evidence.md")
		if err := evidence.ExportMarkdown(parsed, path); err != nil {
			return err
		}
		files = append(files, path)
	}
	a.emit(map[string]any{"records": len(parsed), "files": files})
	return nil
}

func (a *App) cmdDoctor(ctx context.Context, args []string) error {
	fs := newFlagSet("doctor")
	if err := fs.Parse(args); err != nil {
		return usage("doctor: %v", err)
	}
	checks := []map[string]any{}
	add := func(name, status, detail string) {
		checks = append(checks, map[string]any{"name": name, "status": status, "detail": detail})
	}
	add("framework", "ok", version.Version)
	add("go", runtime.Version(), runtime.GOOS+"/"+runtime.GOARCH)
	for _, name := range schema.Names {
		if _, err := schema.Asset(name); err != nil {
			add("schema/"+name, "fail", err.Error())
		}
	}
	add("schemas", "ok", strings.Join(schema.Names, ", "))
	repo, err := a.loadRepo()
	if err != nil {
		add("repository", "fail", err.Error())
	} else {
		add("repository", "ok", fmt.Sprintf("%d skills, %d workflows, %d policies", len(repo.Catalogue.Skills), len(repo.Workflows), len(repo.Policies.Policies)))
		for _, problem := range repo.Config.SelfCheck() {
			add("config", "fail", problem)
		}
		if _, err := sqliteadapter.Open(repo.Config.StatePath()); err != nil {
			add("state", "fail", err.Error())
		} else {
			add("state", "ok", repo.Config.StatePath())
		}
		plan := map[string]any{
			"provider":               "fake",
			"offline":                true,
			"liveProviderSmokeTests": "not executed",
		}
		add("offline-demo", "ok", fmt.Sprint(plan["offline"]))
	}
	for _, binary := range []string{"claude", "codex", "git"} {
		path, err := exec.LookPath(binary)
		if err != nil {
			add("provider/"+binary, "not-installed", "optional")
		} else {
			add("provider/"+binary, "ok", path)
		}
	}
	a.emit(map[string]any{"checks": checks})
	return nil
}
