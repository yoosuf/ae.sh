package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	sqliteadapter "github.com/yoosuf/ae.sh/internal/adapters/sqlite"
	"github.com/yoosuf/ae.sh/internal/evidence"
	"github.com/yoosuf/ae.sh/internal/execution"
	"github.com/yoosuf/ae.sh/internal/util"
)

var worktreeRunIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

// worktreePath returns the only path AE may use for a run's worktree. The
// run id is constrained and the result must remain under <root>/.ae/worktrees.
func worktreePath(root, runID string) (string, error) {
	if !worktreeRunIDPattern.MatchString(runID) {
		return "", invalid("invalid run id %q for worktree use", runID)
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	base := filepath.Join(absRoot, ".ae", "worktrees")
	path := filepath.Join(base, runID)
	rel, err := filepath.Rel(base, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", &PolicyError{Message: "worktree path escapes the AE worktree directory"}
	}
	return path, nil
}

// validateWorkspace accepts only an existing AE-managed worktree directory.
// Arbitrary paths are rejected so a run cannot be pointed at user-owned work.
func validateWorkspace(root, value string) (string, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	base, err := filepath.EvalSymlinks(filepath.Join(absRoot, ".ae", "worktrees"))
	if err != nil {
		return "", invalid("AE worktree directory does not exist: %v", err)
	}
	absValue, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(absValue)
	if err != nil {
		return "", invalid("workspace does not exist: %s", value)
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return "", invalid("workspace is not a directory: %s", value)
	}
	rel, err := filepath.Rel(base, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", &PolicyError{Message: "workspace must be an AE-managed worktree under .ae/worktrees"}
	}
	return resolved, nil
}

// validateManagedWorktree adds git-level checks to validateWorkspace: the path
// must be a registered git worktree whose real path matches the managed path.
// A plain directory placed under .ae/worktrees is not sufficient. Ownership by
// a specific run is checked separately by autopilot, which always allocates a
// dedicated worktree per run.
func validateManagedWorktree(ctx context.Context, root, value string) (string, error) {
	resolved, err := validateWorkspace(root, value)
	if err != nil {
		return "", err
	}
	top, err := runGit(ctx, resolved, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", invalid("workspace is not a git worktree: %v", err)
	}
	topReal, err := filepath.EvalSymlinks(strings.TrimSpace(top))
	if err != nil {
		return "", err
	}
	if topReal != resolved {
		return "", &PolicyError{Message: "workspace git root does not match the managed worktree"}
	}
	list, err := runGit(ctx, root, "worktree", "list", "--porcelain")
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(list, "\n") {
		if !strings.HasPrefix(line, "worktree ") {
			continue
		}
		registered, err := filepath.EvalSymlinks(strings.TrimSpace(strings.TrimPrefix(line, "worktree ")))
		if err == nil && registered == resolved {
			return resolved, nil
		}
	}
	return "", &PolicyError{Message: "workspace is not a git worktree registered with this repository"}
}

// validateRunWorktree requires the workspace to be the dedicated AE worktree
// for this run. Autopilot binds one worktree per run and never reuses another
// run's workspace.
func validateRunWorktree(ctx context.Context, root, runID, value string) (string, error) {
	resolved, err := validateManagedWorktree(ctx, root, value)
	if err != nil {
		return "", err
	}
	expected, err := worktreePath(root, runID)
	if err != nil {
		return "", err
	}
	expectedReal, err := filepath.EvalSymlinks(expected)
	if err != nil {
		return "", invalid("run worktree does not exist: %v", err)
	}
	if resolved != expectedReal {
		return "", &PolicyError{Message: "workspace does not belong to this run"}
	}
	return resolved, nil
}

func (a *App) cmdWorktree(ctx context.Context, args []string) error {
	if len(args) != 2 || (args[0] != "prepare" && args[0] != "clean") {
		return usage("worktree requires: worktree prepare|clean <run-id>")
	}
	repo, err := a.loadRepo()
	if err != nil {
		return err
	}
	path, err := worktreePath(repo.Config.Root, args[1])
	if err != nil {
		return err
	}
	if _, err := exec.LookPath("git"); err != nil {
		return &PolicyError{Message: "git is not installed; no worktree was created"}
	}
	rootReal, topReal, err := gitWorktreeRoot(ctx, repo.Config.Root)
	if err != nil {
		return err
	}
	store, err := sqliteadapter.Open(repo.Config.StatePath())
	if err != nil {
		return err
	}
	defer store.Close()
	run, err := store.GetRun(ctx, args[1])
	if err != nil {
		return a.notFound(err)
	}
	manager := evidence.New(repo.Config.EvidenceDir(), repo.Config.ObjectsDir())
	if args[0] == "prepare" {
		baseSHA, err := a.createDetachedWorktree(ctx, manager, store, run, path, rootReal)
		if err != nil {
			return err
		}
		a.emit(map[string]any{"runID": run.ID, "path": filepath.ToSlash(path), "baseSHA": baseSHA, "prepared": true})
		return nil
	}
	return a.cleanWorktree(ctx, manager, store, run, path, topReal)
}

// prepareManagedWorktree creates the only worktree path AE may use for a run.
// It is shared by `worktree prepare` and `autopilot`.
func (a *App) prepareManagedWorktree(ctx context.Context, repo repoContext, runID string) (string, string, error) {
	path, err := worktreePath(repo.Config.Root, runID)
	if err != nil {
		return "", "", err
	}
	if _, err := exec.LookPath("git"); err != nil {
		return "", "", &PolicyError{Message: "git is not installed; no worktree was created"}
	}
	rootReal, _, err := gitWorktreeRoot(ctx, repo.Config.Root)
	if err != nil {
		return "", "", err
	}
	store, err := sqliteadapter.Open(repo.Config.StatePath())
	if err != nil {
		return "", "", err
	}
	defer store.Close()
	run, err := store.GetRun(ctx, runID)
	if err != nil {
		return "", "", a.notFound(err)
	}
	manager := evidence.New(repo.Config.EvidenceDir(), repo.Config.ObjectsDir())
	baseSHA, err := a.createDetachedWorktree(ctx, manager, store, run, path, rootReal)
	if err != nil {
		return "", "", err
	}
	return path, baseSHA, nil
}

func gitWorktreeRoot(ctx context.Context, root string) (string, string, error) {
	top, err := runGit(ctx, root, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", "", invalid("repository root is not a git worktree: %v", err)
	}
	rootReal, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", "", err
	}
	topReal, err := filepath.EvalSymlinks(strings.TrimSpace(top))
	if err != nil {
		return "", "", err
	}
	if rootReal != topReal {
		return "", "", &PolicyError{Message: "ae.yaml must be at the git worktree root; refusing to create a worktree from a nested directory"}
	}
	return rootReal, topReal, nil
}

func (a *App) createDetachedWorktree(ctx context.Context, manager *evidence.Manager, store *sqliteadapter.Store, run execution.Run, path, root string) (string, error) {
	if _, err := os.Lstat(path); err == nil {
		return "", &ConflictError{Message: "worktree already exists: " + filepath.ToSlash(path)}
	} else if !os.IsNotExist(err) {
		return "", err
	}
	baseSHA, err := runGit(ctx, root, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	baseSHA = strings.TrimSpace(baseSHA)
	if err := util.EnsureDir(filepath.Dir(path)); err != nil {
		return "", err
	}
	command := fmt.Sprintf("git worktree add --detach %s %s", filepath.ToSlash(path), baseSHA)
	if _, err := runGit(ctx, root, "worktree", "add", "--detach", path, baseSHA); err != nil {
		return "", &PolicyError{Message: "git refused to create the worktree: " + util.Sanitize(err.Error())}
	}
	// Reject a path that resolved outside the AE worktree directory.
	base := filepath.Join(root, ".ae", "worktrees")
	baseReal, err := filepath.EvalSymlinks(base)
	if err != nil {
		return "", &PolicyError{Message: "worktree path could not be verified; the worktree was left for manual inspection"}
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil || filepath.Base(resolved) != filepath.Base(path) || !within(baseReal, resolved) {
		if _, removeErr := runGit(ctx, root, "worktree", "remove", path); removeErr != nil {
			return "", &PolicyError{Message: "worktree path was redirected; automatic removal failed and the worktree was left for manual inspection at " + filepath.ToSlash(path)}
		}
		return "", &PolicyError{Message: "worktree path was redirected; the worktree was removed"}
	}
	if err := appendWorktreeEvidence(ctx, manager, store, run.ID, "prepare", map[string]any{
		"command": command, "path": filepath.ToSlash(path), "baseSHA": baseSHA,
		"simulated": false, "note": "Detached worktree; no branch, commit, push, or deployment was created.",
	}); err != nil {
		return "", err
	}
	return baseSHA, nil
}

func (a *App) cleanWorktree(ctx context.Context, manager *evidence.Manager, store *sqliteadapter.Store, run execution.Run, path, root string) error {
	if _, err := os.Lstat(path); err != nil {
		return invalid("worktree does not exist: %s", path)
	}
	if _, err := runGit(ctx, root, "worktree", "remove", path); err != nil {
		return &PolicyError{Message: "git refused to remove the worktree (it may contain changes): " + util.Sanitize(err.Error())}
	}
	if err := appendWorktreeEvidence(ctx, manager, store, run.ID, "clean", map[string]any{
		"command": "git worktree remove " + filepath.ToSlash(path), "path": filepath.ToSlash(path),
		"simulated": false, "note": "Worktree removed without --force; no commits or branches were changed.",
	}); err != nil {
		return err
	}
	a.emit(map[string]any{"runID": run.ID, "path": filepath.ToSlash(path), "removed": true})
	return nil
}

func within(base, candidate string) bool {
	rel, err := filepath.Rel(base, candidate)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func runGit(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %s: %w", strings.Join(args, " "), strings.TrimSpace(util.Sanitize(string(out))), err)
	}
	return string(out), nil
}

func appendWorktreeEvidence(ctx context.Context, manager *evidence.Manager, store *sqliteadapter.Store, runID, action string, data map[string]any) error {
	records, err := store.Evidence(ctx, runID)
	if err != nil {
		return err
	}
	record, err := manager.Write(ctx, runID, len(records), "artifact", "Worktree "+action, "ae worktree", data, nil, nil, "")
	if err != nil {
		return err
	}
	return store.AppendEvidence(ctx, execution.EvidenceRecord{RunID: runID, JSON: mustEvidenceJSON(record)})
}
