package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	sqliteadapter "github.com/aef-framework/aef/internal/adapters/sqlite"
	"github.com/aef-framework/aef/internal/config"
	"github.com/aef-framework/aef/internal/evidence"
	"github.com/aef-framework/aef/internal/execution"
	"github.com/aef-framework/aef/internal/util"
)

type verificationResult struct {
	Gate       string `json:"gate"`
	Command    string `json:"command"`
	Workspace  string `json:"workspace,omitempty"`
	Passed     bool   `json:"passed"`
	Skipped    bool   `json:"skipped,omitempty"`
	Reason     string `json:"reason,omitempty"`
	ExitStatus int    `json:"exitStatus"`
	Output     string `json:"output,omitempty"`
}

func (a *App) cmdVerify(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return usage("verify requires a run id")
	}
	fs := newFlagSet("verify")
	profileFlag := fs.String("profile", "", "override the run's profiles (comma-separated)")
	if err := fs.Parse(args[1:]); err != nil {
		return usage("verify: %v", err)
	}
	if fs.NArg() != 0 {
		return usage("verify accepts exactly one run id")
	}
	runID := args[0]
	repo, err := a.loadRepo()
	if err != nil {
		return err
	}
	profiles, results, err := a.verifyRun(ctx, repo, runID, *profileFlag, "")
	if err != nil {
		return err
	}
	a.emit(map[string]any{
		"runID":     runID,
		"profiles":  profiles,
		"checks":    results,
		"allPassed": true,
		"note":      "Checks executed locally; output is sanitized and recorded as evidence. No check was marked passed without a real exit status.",
	})
	return nil
}

// verifyRun executes real local checks and writes their evidence. When
// workspace is set, checks run against that validated AEF worktree instead of
// the repository root. It returns an error on any failed gate so callers such
// as autopilot cannot treat a partial result as a pass.
func (a *App) verifyRun(ctx context.Context, repo repoContext, runID, profileOverride, workspace string) ([]string, []verificationResult, error) {
	checkRoot := repo.Config.Root
	if workspace != "" {
		checkRoot = workspace
	}
	store, err := sqliteadapter.Open(repo.Config.StatePath())
	if err != nil {
		return nil, nil, err
	}
	defer store.Close()
	run, err := store.GetRun(ctx, runID)
	if err != nil {
		return nil, nil, err
	}
	profiles := run.Profiles
	if len(profiles) == 0 {
		profiles = repo.Config.Profiles
	}
	if profileOverride != "" {
		profiles = strings.Split(profileOverride, ",")
	}
	if len(profiles) == 0 {
		return nil, nil, usage("verify requires at least one profile in config or --profile")
	}
	manager := evidence.New(repo.Config.EvidenceDir(), repo.Config.ObjectsDir())
	results := []verificationResult{}
	for _, name := range profiles {
		profile, err := config.LoadProfile(repo.Config.Root, strings.TrimSpace(name))
		if err != nil {
			return nil, nil, invalid("%v", err)
		}
		for _, check := range profile.Checks {
			result, err := a.runProfileCheck(ctx, checkRoot, manager, store, run.ID, profile.ID, check)
			if err != nil {
				return nil, nil, err
			}
			results = append(results, result)
		}
	}
	secretResult, err := a.runSecretScan(ctx, checkRoot, manager, store, run.ID)
	if err != nil {
		return nil, nil, err
	}
	results = append(results, secretResult)
	failed := []string{}
	for _, result := range results {
		if !result.Passed {
			failed = append(failed, result.Gate)
		}
	}
	if len(failed) > 0 {
		detail := strings.Join(failed, ", ")
		for _, result := range results {
			if !result.Passed && result.Reason != "" {
				detail += " (" + result.Gate + ": " + result.Reason + ")"
			}
		}
		// Do not emit a success-shaped payload before an error; JSON callers
		// must see exactly one error envelope on failure.
		return profiles, results, &PolicyError{Message: "verification failed for gates: " + detail}
	}
	return profiles, results, nil
}

func (a *App) runProfileCheck(ctx context.Context, root string, manager *evidence.Manager, store *sqliteadapter.Store, runID, profileID string, check config.Check) (verificationResult, error) {
	fields := strings.Fields(check.Command)
	if len(fields) == 0 {
		return verificationResult{}, invalid("profile %s has an empty verification command", profileID)
	}
	if _, err := exec.LookPath(fields[0]); err != nil {
		reason := "tool not installed: " + fields[0]
		missingTool := 127
		result := verificationResult{Gate: check.Gate, Command: check.Command, Workspace: filepath.ToSlash(root), Reason: reason, Skipped: true, ExitStatus: missingTool}
		if err := writeVerificationEvidence(ctx, manager, store, runID, root, check.Gate, check.Command, reason, false, &missingTool, profileID); err != nil {
			return verificationResult{}, err
		}
		return result, nil
	}
	cmd := exec.CommandContext(ctx, "sh", "-c", check.Command)
	cmd.Dir = root
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	exit := 0
	if ee, ok := err.(*exec.ExitError); ok {
		exit = ee.ExitCode()
	} else if err != nil {
		exit = 127
	}
	passed := exit == 0 && (!check.ExpectEmptyOutput || strings.TrimSpace(stdout.String()) == "")
	output := truncateVerificationOutput(util.Sanitize(strings.TrimSpace(stdout.String() + "\n" + stderr.String())))
	if err := writeVerificationEvidence(ctx, manager, store, runID, root, check.Gate, check.Command, output, passed, &exit, profileID); err != nil {
		return verificationResult{}, err
	}
	return verificationResult{
		Gate: check.Gate, Command: check.Command, Workspace: filepath.ToSlash(root), Passed: passed,
		ExitStatus: exit, Output: output,
	}, nil
}

func (a *App) runSecretScan(ctx context.Context, root string, manager *evidence.Manager, store *sqliteadapter.Store, runID string) (verificationResult, error) {
	const gate = "secret-scan"
	const command = "aef builtin secret-scan"
	filesScanned := 0
	findings := 0
	paths := []string{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", ".aef", "dist", "node_modules", "vendor":
				return filepath.SkipDir
			}
			return nil
		}
		info, err := entry.Info()
		if err != nil || info.Size() > 1<<20 {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil || bytes.IndexByte(raw, 0) >= 0 {
			return nil
		}
		filesScanned++
		if count := util.SecretShapeCount(string(raw)); count > 0 {
			findings += count
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				rel = path
			}
			paths = append(paths, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		return verificationResult{}, err
	}
	passed := findings == 0
	exit := 0
	if !passed {
		exit = 1
	}
	data := map[string]any{
		"gate": gate, "command": command, "workspace": filepath.ToSlash(root),
		"passed": passed, "simulated": false,
		"filesScanned": filesScanned, "findings": findings, "paths": paths,
	}
	if err := appendVerificationRecord(ctx, manager, store, runID, gate, "Verify "+gate, "aef verify", data, &exit); err != nil {
		return verificationResult{}, err
	}
	return verificationResult{
		Gate: gate, Command: command, Workspace: filepath.ToSlash(root), Passed: passed, ExitStatus: exit,
		Output: fmt.Sprintf("scanned %d files; %d credential-shaped matches", filesScanned, findings),
	}, nil
}

func writeVerificationEvidence(ctx context.Context, manager *evidence.Manager, store *sqliteadapter.Store, runID, root, gate, command, output string, passed bool, exit *int, profileID string) error {
	data := map[string]any{
		"gate": gate, "command": command, "workspace": filepath.ToSlash(root), "output": output,
		"passed": passed, "simulated": false, "profile": profileID,
	}
	return appendVerificationRecord(ctx, manager, store, runID, gate, "Verify "+gate, "aef verify", data, exit)
}

func appendVerificationRecord(ctx context.Context, manager *evidence.Manager, store *sqliteadapter.Store, runID, gate, subject, actor string, data map[string]any, exit *int) error {
	records, err := store.Evidence(ctx, runID)
	if err != nil {
		return err
	}
	record, err := manager.Write(ctx, runID, len(records), "test", subject, actor, data, exit, nil, "")
	if err != nil {
		return err
	}
	return store.AppendEvidence(ctx, execution.EvidenceRecord{RunID: runID, JSON: mustEvidenceJSON(record)})
}

func mustEvidenceJSON(record evidence.Record) string {
	raw, err := json.Marshal(record)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func truncateVerificationOutput(value string) string {
	const limit = 16 << 10
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "\n[truncated]"
}
