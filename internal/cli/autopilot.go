package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aef-framework/aef/internal/adapters/github"
	sqliteadapter "github.com/aef-framework/aef/internal/adapters/sqlite"
	"github.com/aef-framework/aef/internal/execution"
	"github.com/aef-framework/aef/internal/integration"
	"github.com/aef-framework/aef/internal/util"
	"github.com/aef-framework/aef/internal/workflow"
)

const (
	maxAutopilotSteps         = 16
	maxAutopilotDiffBytes     = 2 << 20
	maxAutopilotUntrackedFile = 1 << 20
)

type autopilotStep struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

type autopilotResult struct {
	RunID       string          `json:"runId,omitempty"`
	Workflow    string          `json:"workflow"`
	Provider    string          `json:"provider"`
	IssueKey    string          `json:"issueKey,omitempty"`
	Status      string          `json:"status"`
	Steps       []autopilotStep `json:"steps"`
	Run         *execution.Run  `json:"run,omitempty"`
	Review      *ReviewResult   `json:"review,omitempty"`
	PullRequest map[string]any  `json:"pullRequest,omitempty"`
	Note        string          `json:"note"`
}

// cmdAutopilot is an opt-in orchestration loop. It never invents approvals,
// check results, or identity, and it never pushes, creates PRs, merges, or
// deploys. Live providers, writes, and GitHub mutations each need a separate
// explicit flag.
func (a *App) cmdAutopilot(ctx context.Context, args []string) error {
	fs := newFlagSet("autopilot")
	issueFlag := fs.String("issue", "", "issue fixture, Jira key, or Jira browse URL")
	workflowFlag := fs.String("workflow", "feature-development", "workflow id")
	providerFlag := fs.String("provider", "", "fake, claude, or codex (defaults to config)")
	actorFlag := fs.String("actor", "local-user", "run actor identity")
	resumeFlag := fs.String("resume", "", "continue an existing run")
	approverFlag := fs.String("approver", "", "explicit local approver for delegated gates")
	approverRoleFlag := fs.String("approver-role", "local-delegate", "role recorded with delegated approvals")
	profileFlag := fs.String("profile", "", "verification profiles (defaults to config/run)")
	githubPRFlag := fs.String("github-pr", "", "existing PR ref for GitHub state evidence")
	dryRun := fs.Bool("dry-run", false, "preview the loop without state or service calls")
	confirmLive := fs.Bool("confirm-live", false, "authorize Jira, provider, and GitHub reads")
	allowWrites := fs.Bool("allow-writes", false, "allow provider file edits in the managed worktree")
	confirmGates := fs.Bool("confirm-gates", false, "authorize delegated local approvals")
	comment := fs.Bool("comment", false, "post a run summary comment to the PR")
	confirmMutate := fs.Bool("confirm-mutate", false, "authorize the single mutating GitHub comment action")
	if err := fs.Parse(args); err != nil {
		return usage("autopilot: %v", err)
	}
	if fs.NArg() != 0 {
		return usage("autopilot takes no positional arguments")
	}
	if *resumeFlag != "" && *dryRun {
		return usage("autopilot --resume cannot be combined with --dry-run")
	}
	if *resumeFlag == "" && *issueFlag == "" {
		return usage("autopilot requires --issue or --resume")
	}
	if *approverFlag != "" && !*confirmGates {
		return usage("autopilot --approver requires --confirm-gates")
	}
	if *confirmMutate && !*comment {
		return usage("autopilot --confirm-mutate requires --comment")
	}
	if *comment && (*githubPRFlag == "" || !*confirmMutate) {
		return usage("autopilot --comment requires --github-pr and --confirm-mutate")
	}
	if *githubPRFlag != "" && !*confirmLive {
		return &PolicyError{Message: "autopilot GitHub reads require --confirm-live"}
	}

	repo, err := a.loadRepo()
	if err != nil {
		return err
	}
	providerName := *providerFlag
	var resumeRun execution.Run
	if *resumeFlag != "" {
		_, store, err := a.openStore()
		if err != nil {
			return err
		}
		resumeRun, err = store.GetRun(ctx, *resumeFlag)
		store.Close()
		if err != nil {
			return a.notFound(err)
		}
		if providerName == "" {
			providerName = resumeRun.Provider
		}
	}
	if providerName == "" {
		providerName = repo.Config.Provider.Default
	}
	switch providerName {
	case "fake", "claude", "codex":
	default:
		return usage("unsupported provider %q", providerName)
	}
	if providerName != "fake" && !*dryRun && !*confirmLive {
		return &PolicyError{Message: "autopilot live providers require --confirm-live; use --dry-run to preview"}
	}
	if *allowWrites && (providerName == "fake" || !*confirmLive) {
		return &PolicyError{Message: "autopilot --allow-writes requires a live provider and --confirm-live"}
	}
	if *githubPRFlag != "" && repo.Config.Integrations.GitHub.Mode != "local-cli" {
		return &PolicyError{Message: "autopilot GitHub reads require integrations.github.mode: local-cli"}
	}

	result := autopilotResult{
		Workflow: *workflowFlag, Provider: providerName, Steps: []autopilotStep{},
		Note: "Autonomous only within recorded policy: explicit local approvals, real local checks, and fail-closed gates. No push, PR creation, merge, or deployment is performed.",
	}
	addStep := func(name, status, detail string) {
		if len(result.Steps) < maxAutopilotSteps {
			result.Steps = append(result.Steps, autopilotStep{Name: name, Status: status, Detail: detail})
		}
	}

	var issueSource integration.Issue
	wf, err := a.findWorkflow(repo, *workflowFlag)
	if err != nil {
		return err
	}
	if *resumeFlag == "" {
		issue, err := a.resolveIssueWithLive(ctx, repo, *issueFlag, !*dryRun && *confirmLive)
		if err != nil {
			return err
		}
		issueSource = issue
		result.IssueKey = issue.Key
	} else {
		result.IssueKey = resumeRun.IssueKey
		result.Workflow = resumeRun.WorkflowID
		wf, err = a.findWorkflow(repo, resumeRun.WorkflowID)
		if err != nil {
			return err
		}
	}

	if *dryRun {
		engine := &execution.Engine{
			Policies: repo.Policies, KnownSkills: repo.Catalogue.BySlugSet(), KnownAgents: repo.Agents,
		}
		_, preview, err := engine.Start(ctx, execution.StartOptions{
			Workflow: wf, Issue: issueSource, Provider: providerName, Actor: *actorFlag, DryRun: true,
		})
		if err != nil {
			return invalid("%v", err)
		}
		for _, name := range []string{"issue-intake", "managed-worktree", "provider-tasks", "local-verification", "diff-review", "approval-gates", "github-state"} {
			addStep(name, "planned", "")
		}
		result.Status = "dry-run"
		result.Note = preview.Note + " " + result.Note
		a.emit(result)
		return nil
	}

	store, err := sqliteadapter.Open(repo.Config.StatePath())
	if err != nil {
		return err
	}
	defer store.Close()
	engine, err := a.buildEngineWithStore(repo, providerName, store)
	if err != nil {
		return err
	}
	engine.AllowWrites = *allowWrites

	blocked := func(runID, reason string) error {
		return &PolicyError{Message: fmt.Sprintf("autopilot %s blocked: %s", runID, util.Sanitize(reason))}
	}
	advance := func(runID string) (execution.Run, error) {
		run, err := engine.Continue(ctx, runID)
		if err != nil {
			latest, getErr := store.GetRun(ctx, runID)
			if getErr == nil {
				return latest, blocked(runID, latest.TerminalReason)
			}
			return run, blocked(runID, err.Error())
		}
		return run, nil
	}
	handleGate := func() (bool, execution.Run, error) {
		run, err := store.GetRun(ctx, result.RunID)
		if err != nil {
			return false, run, err
		}
		if run.Status != execution.StatusAwaitingApproval {
			return false, run, nil
		}
		gate, err := currentApprovalGate(wf, run)
		if err != nil {
			return false, run, err
		}
		if gate == "" {
			return false, run, blocked(run.ID, "run awaits an unknown approval gate")
		}
		if !*confirmGates || *approverFlag == "" {
			result.Status = "awaiting_approval"
			addStep("gate/"+gate, "stopped", "human approval required; rerun with --resume after approval")
			return true, run, nil
		}
		if *approverFlag == run.Actor {
			return false, run, blocked(run.ID, "self-approval is not permitted")
		}
		if err := engine.Approve(ctx, run.ID, gate, *approverFlag, *approverRoleFlag); err != nil {
			return false, run, blocked(run.ID, err.Error())
		}
		addStep("gate/"+gate, "ok", "recorded explicitly supplied local approver "+util.Sanitize(*approverFlag))
		return false, run, nil
	}

	runID := *resumeFlag
	if runID == "" {
		run, _, err := engine.Start(ctx, execution.StartOptions{
			Workflow: wf, Issue: issueSource, Provider: providerName, Actor: *actorFlag,
			Profiles: repo.Config.Profiles, DeferExecution: true,
		})
		if err != nil {
			return err
		}
		runID = run.ID
		addStep("issue-intake", "ok", "run created from "+result.IssueKey)
	} else {
		addStep("resume", "ok", "continuing existing run")
	}
	result.RunID = runID

	// Bind and validate the managed worktree before any provider task. The
	// pre-approval tasks also invoke the provider and may write when writes
	// are explicitly allowed, so they must never run in the CLI directory.
	run, err := store.GetRun(ctx, runID)
	if err != nil {
		return err
	}
	if run.Workspace == "" {
		path, baseSHA, err := a.prepareManagedWorktree(ctx, repo, runID)
		if err != nil {
			return err
		}
		if err := store.BindWorkspace(ctx, runID, path); err != nil {
			return err
		}
		if _, err := validateRunWorktree(ctx, repo.Config.Root, runID, path); err != nil {
			return err
		}
		addStep("managed-worktree", "ok", "detached worktree at "+filepath.ToSlash(path)+" from "+baseSHA)
	} else {
		if _, err := validateRunWorktree(ctx, repo.Config.Root, runID, run.Workspace); err != nil {
			return err
		}
		addStep("managed-worktree", "ok", "reusing persisted workspace")
	}
	run, err = advance(runID)
	if err != nil {
		return err
	}
	addStep("provider-tasks", "ok", "state "+run.State)
	if run.Status == execution.StatusBlocked {
		return blocked(runID, run.TerminalReason)
	}

	stopped, run, err := handleGate()
	if err != nil {
		return err
	}
	if stopped {
		result.Run = &run
		a.emit(result)
		return nil
	}
	if run.Status == execution.StatusAwaitingApproval {
		run, err = advance(runID)
		if err != nil {
			return err
		}
	}
	if run.Status == execution.StatusBlocked {
		return blocked(runID, run.TerminalReason)
	}

	run, err = store.GetRun(ctx, runID)
	if err != nil {
		return err
	}
	verifyWorkspace := ""
	if run.Workspace != "" {
		verifyWorkspace, err = validateRunWorktree(ctx, repo.Config.Root, runID, run.Workspace)
		if err != nil {
			return err
		}
	}

	blockOnFindings := func(label string, review ReviewResult) error {
		result.Review = &review
		ids := make([]string, 0, len(review.Findings))
		for _, finding := range review.Findings {
			ids = append(ids, finding.ID)
		}
		return blocked(runID, label+" has unresolved review findings requiring `aef review resolve`: "+strings.Join(ids, ", "))
	}

	var prSource *github.Source
	var prState github.PR
	var prChecks []github.Check
	var prEvidenceID string
	if *githubPRFlag != "" {
		source, err := github.New(repo.Config.Integrations.GitHub)
		if err != nil {
			return invalid("%v", err)
		}
		prSource = source
		source.AllowMutations = *comment && *confirmMutate
		prState, err = source.ViewPR(ctx, *githubPRFlag)
		if err != nil {
			return err
		}
		prChecks, err = source.Checks(ctx, *githubPRFlag)
		if err != nil {
			return err
		}
		prDiff, err := source.Diff(ctx, *githubPRFlag)
		if err != nil {
			return err
		}
		failing := []string{}
		for _, check := range prChecks {
			switch strings.ToUpper(check.Conclusion) {
			case "FAILURE", "TIMED_OUT", "CANCELLED", "ACTION_REQUIRED":
				failing = append(failing, check.Name)
			}
		}
		data := map[string]any{
			"number": prState.Number, "state": prState.State, "url": prState.URL, "headSHA": prState.HeadSHA,
			"baseRef": prState.BaseRef, "reviewDecision": prState.ReviewDecision,
			"reviews": len(prState.Reviews), "comments": len(prState.Comments), "checks": prChecks,
			"diffDigest": util.Digest(prDiff), "simulated": false,
			"note": "Read through the local gh CLI; no push, PR creation, merge, or deploy was performed.",
		}
		record, err := appendRunEvidence(ctx, repo.Config, store, runID, "github", "GitHub PR state", "aef-autopilot",
			util.SanitizeValue(data).(map[string]any), nil, nil)
		if err != nil {
			return err
		}
		prEvidenceID = record.RecordID
		result.PullRequest = map[string]any{
			"number": prState.Number, "state": prState.State, "url": prState.URL,
			"reviewDecision": prState.ReviewDecision, "checks": len(prChecks),
			"evidenceRecordID": prEvidenceID,
		}
		addStep("github-state", "ok", "PR "+util.Sanitize(*githubPRFlag)+" read with evidence "+prEvidenceID)
		prDiffPath := filepath.Join(repo.Config.EvidenceDir(), "autopilot", runID+".pr.diff")
		if err := util.EnsureDir(filepath.Dir(prDiffPath)); err != nil {
			return err
		}
		if err := os.WriteFile(prDiffPath, []byte(util.Sanitize(string(prDiff))), 0o600); err != nil {
			return err
		}
		prReview, err := a.reviewDiff(ctx, prDiffPath, runID, 0, run.CandidateSHA, run.BaseSHA)
		if err != nil {
			return err
		}
		if len(prReview.Findings) > 0 {
			return blockOnFindings("PR diff", prReview)
		}
		addStep("pr-diff-review", "ok", "pattern scan found no hypotheses; not an independent review")
		if len(failing) > 0 {
			return blocked(runID, "PR checks are failing: "+strings.Join(failing, ", "))
		}
	}

	_, _, err = a.verifyRun(ctx, repo, runID, *profileFlag, verifyWorkspace)
	if err != nil {
		return err
	}
	addStep("local-verification", "ok", "real local checks recorded as evidence")

	if run.Workspace != "" {
		diff, err := collectWorkspaceDiff(ctx, run.Workspace)
		if err != nil {
			return err
		}
		if len(diff) > maxAutopilotDiffBytes {
			return invalid("workspace diff exceeds %d bytes", maxAutopilotDiffBytes)
		}
		if strings.TrimSpace(diff) == "" {
			addStep("diff-review", "ok", "no local source changes to scan")
		} else {
			diffPath := filepath.Join(repo.Config.EvidenceDir(), "autopilot", runID+".diff")
			if err := util.EnsureDir(filepath.Dir(diffPath)); err != nil {
				return err
			}
			if err := os.WriteFile(diffPath, []byte(util.Sanitize(diff)), 0o600); err != nil {
				return err
			}
			review, err := a.reviewDiff(ctx, diffPath, runID, 0, run.CandidateSHA, run.BaseSHA)
			if err != nil {
				return err
			}
			if len(review.Findings) > 0 {
				return blockOnFindings("workspace diff", review)
			}
			addStep("diff-review", "ok", "pattern scan found no hypotheses; not an independent review")
		}
	}

	stopped, run, err = handleGate()
	if err != nil {
		return err
	}
	if stopped {
		result.Run = &run
		a.emit(result)
		return nil
	}
	if run.Status == execution.StatusAwaitingApproval {
		run, err = advance(runID)
		if err != nil {
			return err
		}
	}
	if run.Status == execution.StatusBlocked {
		return blocked(runID, run.TerminalReason)
	}

	if *comment {
		if prSource == nil || prEvidenceID == "" {
			return invalid("autopilot --comment requires a GitHub PR state record")
		}
		body := fmt.Sprintf("AEF run %s: state=%s, checks=%d, review=%s. Evidence record %s. No merge or deployment was performed by AEF.",
			runID, run.State, len(prChecks), prState.ReviewDecision, prEvidenceID)
		if err := prSource.Comment(ctx, *githubPRFlag, body); err != nil {
			return err
		}
		addStep("github-comment", "ok", "comment recorded; no merge or deployment")
	}

	run, err = store.GetRun(ctx, runID)
	if err != nil {
		return err
	}
	result.Run = &run
	switch run.Status {
	case execution.StatusCompleted:
		result.Status = "completed"
	case execution.StatusAwaitingApproval:
		result.Status = "awaiting_approval"
	default:
		result.Status = run.Status
	}
	a.emit(result)
	return nil
}

func currentApprovalGate(wf workflow.Workflow, run execution.Run) (string, error) {
	order, err := wf.TopologicalOrder()
	if err != nil {
		return "", err
	}
	if run.CurrentTask < 0 || run.CurrentTask >= len(order) {
		return "", fmt.Errorf("run task index %d is outside workflow order", run.CurrentTask)
	}
	taskID := order[run.CurrentTask]
	for _, task := range wf.Tasks {
		if task.ID == taskID && task.Type == "approval" {
			return task.Gate, nil
		}
	}
	return "", nil
}

// collectWorkspaceDiff combines staged, unstaged, and untracked text changes.
// A plain `git diff` misses provider-created untracked files, which must not
// silently bypass the local pattern scan. Binary, oversized, symlinked, or
// path-escaping untracked files fail closed because they cannot be scanned.
func collectWorkspaceDiff(ctx context.Context, workspace string) (string, error) {
	staged, err := runGit(ctx, workspace, "diff", "--cached", "--no-ext-diff", "--no-color")
	if err != nil {
		return "", err
	}
	unstaged, err := runGit(ctx, workspace, "diff", "--no-ext-diff", "--no-color")
	if err != nil {
		return "", err
	}
	untracked, err := runGit(ctx, workspace, "ls-files", "--others", "--exclude-standard")
	if err != nil {
		return "", err
	}
	var out strings.Builder
	if staged != "" {
		out.WriteString(staged)
		if !strings.HasSuffix(staged, "\n") {
			out.WriteByte('\n')
		}
	}
	if unstaged != "" {
		out.WriteString(unstaged)
		if !strings.HasSuffix(unstaged, "\n") {
			out.WriteByte('\n')
		}
	}
	workspaceAbs, err := filepath.Abs(workspace)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(untracked, "\n") {
		rel := strings.TrimRight(line, "\r")
		if rel == "" {
			continue
		}
		if filepath.IsAbs(rel) || strings.Contains(rel, `\`) || strings.HasPrefix(rel, "..") {
			return "", invalid("untracked workspace path %q cannot be scanned", util.Sanitize(rel))
		}
		relPath := filepath.Clean(filepath.FromSlash(rel))
		if relPath == "." || relPath == ".." || strings.HasPrefix(relPath, ".."+string(filepath.Separator)) {
			return "", invalid("untracked workspace path %q cannot be scanned", util.Sanitize(rel))
		}
		path := filepath.Join(workspaceAbs, relPath)
		withinWorkspace, err := filepath.Rel(workspaceAbs, path)
		if err != nil || withinWorkspace == ".." || strings.HasPrefix(withinWorkspace, ".."+string(filepath.Separator)) {
			return "", &PolicyError{Message: "untracked workspace path escapes the managed worktree"}
		}
		info, err := os.Lstat(path)
		if err != nil {
			return "", invalid("untracked workspace file %q cannot be read: %v", util.Sanitize(rel), err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return "", invalid("untracked workspace path %q is not a regular file and cannot be scanned", util.Sanitize(rel))
		}
		if info.Size() > maxAutopilotUntrackedFile {
			return "", invalid("untracked workspace file %q exceeds %d bytes", util.Sanitize(rel), maxAutopilotUntrackedFile)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		if bytes.IndexByte(raw, 0) >= 0 {
			return "", invalid("untracked workspace file %q is binary and cannot be scanned", util.Sanitize(rel))
		}
		lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
		out.WriteString("diff --git a/" + rel + " b/" + rel + "\n")
		out.WriteString("new file mode 100644\n--- /dev/null\n+++ b/" + rel + "\n")
		fmt.Fprintf(&out, "@@ -0,0 +1,%d @@\n", len(lines))
		for _, line := range lines {
			out.WriteString("+" + line + "\n")
		}
	}
	if out.Len() > maxAutopilotDiffBytes {
		return "", invalid("workspace diff exceeds %d bytes", maxAutopilotDiffBytes)
	}
	return out.String(), nil
}
