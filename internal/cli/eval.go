package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/yoosuf/ae.sh/internal/adapters/fake"
	"github.com/yoosuf/ae.sh/internal/config"
	"github.com/yoosuf/ae.sh/internal/integration"
	"github.com/yoosuf/ae.sh/internal/routing"
	"github.com/yoosuf/ae.sh/internal/util"
	"gopkg.in/yaml.v3"
)

type evalIndex struct {
	SchemaVersion string      `yaml:"schema_version"`
	Catalogue     []evalEntry `yaml:"catalogue"`
	Notes         string      `yaml:"notes"`
}

type evalEntry struct {
	ID       string `yaml:"id"`
	Domain   string `yaml:"domain"`
	Fixture  string `yaml:"fixture"`
	Expected string `yaml:"expected"`
	Status   string `yaml:"status"`
}

type evalExpectation struct {
	Fixture              string   `yaml:"fixture"`
	ExpectedOutcome      string   `yaml:"expected_outcome"`
	RequiredOutputFields []string `yaml:"required_output_fields"`
	Rubric               []struct {
		ID          string `yaml:"id"`
		Weight      int    `yaml:"weight"`
		Description string `yaml:"description"`
	} `yaml:"rubric"`
}

var (
	evalCasePattern       = regexp.MustCompile(`(?m)^## Case [0-9]+`)
	evalIdentifierPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
)

func (a *App) cmdEval(ctx context.Context, args []string) error {
	fs := newFlagSet("eval")
	suite := fs.String("suite", "skills", "skills, security, or all")
	execute := fs.Bool("execute", false, "run deterministic structural checks on authored fixtures")
	behavioral := fs.Bool("behavioral", false, "run deterministic fake-provider behavioral scenarios (implies structural checks)")
	if err := fs.Parse(args); err != nil {
		return usage("eval: %v", err)
	}
	root := a.Root
	if root == "" {
		var err error
		root, err = os.Getwd()
		if err != nil {
			return err
		}
	}
	if configPath, err := config.Find(root); err == nil {
		root = filepath.Dir(configPath)
	}
	if *suite != "skills" && *suite != "security" && *suite != "all" {
		return usage("unknown eval suite %q", *suite)
	}
	if *behavioral && *suite == "security" {
		return usage("eval --behavioral requires --suite skills or all")
	}
	result := map[string]any{
		"suite": *suite, "executed": false, "modelBehaviorExecuted": false,
		"mode": "catalogue-only",
	}
	if !*execute && !*behavioral {
		if *suite == "skills" || *suite == "all" {
			index, err := loadEvalIndex(root)
			if err != nil {
				return err
			}
			result["skills"] = map[string]any{"fixtures": len(index.Catalogue), "status": "authored; not executed"}
		}
		if *suite == "security" || *suite == "all" {
			raw, err := os.ReadFile(filepath.Join(root, "evaluations", "adversarial", "prompt-injection.md"))
			if err != nil {
				return invalid("%v", err)
			}
			result["security"] = map[string]any{"cases": len(evalCasePattern.FindAll(raw, -1)), "status": "authored; not executed"}
		}
		a.emit(result)
		return nil
	}
	failures := []string{}
	result["executed"] = true
	result["mode"] = "structural-fixture-checks"
	if *behavioral {
		result["mode"] = "structural-fixture-checks+behavioral-fake-provider"
		result["provider"] = "fake"
	}
	if *suite == "skills" || *suite == "all" {
		passed, total, problems, err := checkSkillFixtures(root)
		if err != nil {
			return err
		}
		result["skills"] = map[string]any{"fixtures": total, "passed": passed, "problems": problems}
		failures = append(failures, problems...)
	}
	if *suite == "security" || *suite == "all" {
		passed, cases, problems, err := checkSecurityFixture(root)
		if err != nil {
			return err
		}
		result["security"] = map[string]any{"cases": cases, "passed": passed, "problems": problems}
		failures = append(failures, problems...)
	}
	if len(failures) > 0 {
		// Keep --json output to one error envelope when checks fail.
		return invalid("eval structural checks failed: %s", strings.Join(failures, "; "))
	}
	if *behavioral {
		report, err := runBehavioralEval(ctx, root)
		if err != nil {
			return err
		}
		result["behavioral"] = report
		if report.Status != "passed" {
			result["status"] = "failed"
			a.emit(result)
			return invalid("eval behavioral checks failed: %d case(s)", report.Failed)
		}
		result["status"] = "passed"
		result["note"] = "Structural fixtures and deterministic fake-provider scenarios were executed. No model was invoked; modelBehaviorExecuted remains false and there is no model-quality pass rate."
	} else {
		result["status"] = "passed"
		result["note"] = "Only fixture structure was executed. Provider scoring and behavioral pass rates remain unexecuted; no model was invoked."
	}
	a.emit(result)
	return nil
}

func loadEvalIndex(root string) (evalIndex, error) {
	path := filepath.Join(root, "evaluations", "skills", "index.yaml")
	raw, err := os.ReadFile(path)
	if err != nil {
		return evalIndex{}, invalid("%v", err)
	}
	var index evalIndex
	if err := yaml.Unmarshal(raw, &index); err != nil {
		return evalIndex{}, invalid("%v", err)
	}
	if index.SchemaVersion != "1.0" || len(index.Catalogue) == 0 {
		return evalIndex{}, invalid("evaluation index has an unsupported schemaVersion or no entries")
	}
	return index, nil
}

func checkSkillFixtures(root string) (int, int, []string, error) {
	index, err := loadEvalIndex(root)
	if err != nil {
		return 0, 0, nil, err
	}
	problems := []string{}
	passed := 0
	for _, entry := range index.Catalogue {
		valid := true
		if entry.ID == "" || entry.Fixture == "" || entry.Expected == "" {
			problems = append(problems, fmt.Sprintf("%s: index entry is incomplete", entry.ID))
			continue
		}
		fixture := filepath.Join(root, filepath.FromSlash(entry.Fixture))
		expectedPath := filepath.Join(root, filepath.FromSlash(entry.Expected))
		if !withinRoot(root, fixture) || !withinRoot(root, expectedPath) {
			problems = append(problems, fmt.Sprintf("%s: fixture path escapes the repository", entry.ID))
			continue
		}
		fixtureRaw, err := os.ReadFile(fixture)
		if err != nil || strings.TrimSpace(string(fixtureRaw)) == "" {
			problems = append(problems, fmt.Sprintf("%s: fixture is missing or empty", entry.ID))
			valid = false
		}
		expectedRaw, err := os.ReadFile(expectedPath)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: expected file is missing", entry.ID))
			continue
		}
		var expected evalExpectation
		if err := yaml.Unmarshal(expectedRaw, &expected); err != nil {
			problems = append(problems, fmt.Sprintf("%s: expected file is not valid YAML: %v", entry.ID, err))
			continue
		}
		if expected.Fixture != entry.ID || strings.TrimSpace(expected.ExpectedOutcome) == "" || len(expected.RequiredOutputFields) == 0 || len(expected.Rubric) == 0 {
			problems = append(problems, fmt.Sprintf("%s: expected file lacks fixture id, outcome, fields, or rubric", entry.ID))
			valid = false
		}
		for _, criterion := range expected.Rubric {
			if criterion.ID == "" || criterion.Weight <= 0 || strings.TrimSpace(criterion.Description) == "" {
				problems = append(problems, fmt.Sprintf("%s: rubric entry is incomplete", entry.ID))
				valid = false
			}
		}
		if valid {
			passed++
		}
	}
	return passed, len(index.Catalogue), problems, nil
}

func checkSecurityFixture(root string) (bool, int, []string, error) {
	path := filepath.Join(root, "evaluations", "adversarial", "prompt-injection.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		return false, 0, nil, invalid("%v", err)
	}
	matches := evalCasePattern.FindAll(raw, -1)
	problems := []string{}
	if len(matches) == 0 {
		problems = append(problems, "prompt-injection.md contains no cases")
	}
	text := string(raw)
	if !strings.Contains(text, "Expected refusal behavior") || !strings.Contains(text, "Scoring") {
		problems = append(problems, "prompt-injection.md cases lack refusal behavior or scoring sections")
	}
	return len(problems) == 0, len(matches), problems, nil
}

func withinRoot(root, candidate string) bool {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	absCandidate, err := filepath.Abs(candidate)
	if err != nil {
		return false
	}
	return within(absRoot, absCandidate)
}

type behavioralIndex struct {
	SchemaVersion string           `yaml:"schemaVersion"`
	Provider      string           `yaml:"provider"`
	Note          string           `yaml:"note"`
	Cases         []behavioralCase `yaml:"cases"`
}

type behavioralCase struct {
	ID             string   `yaml:"id"`
	Skill          string   `yaml:"skill"`
	TaskID         string   `yaml:"taskId"`
	PromptFixture  string   `yaml:"promptFixture"`
	Route          string   `yaml:"route"`
	ExpectedFields []string `yaml:"expectedFields"`
}

type behavioralResult struct {
	ID                string   `json:"id"`
	Passed            bool     `json:"passed"`
	Reason            string   `json:"reason"`
	EvidenceRecordIDs []string `json:"evidenceRecordIds"`
	OutputDigest      string   `json:"outputDigest,omitempty"`
}

type behavioralReport struct {
	Provider string             `json:"provider"`
	Cases    int                `json:"cases"`
	Passed   int                `json:"passed"`
	Failed   int                `json:"failed"`
	Status   string             `json:"status"`
	Note     string             `json:"note"`
	Results  []behavioralResult `json:"results"`
}

func loadBehavioralIndex(root string) (behavioralIndex, error) {
	path := filepath.Join(root, "evaluations", "behavioral", "index.yaml")
	raw, err := os.ReadFile(path)
	if err != nil {
		return behavioralIndex{}, invalid("%v", err)
	}
	var index behavioralIndex
	if err := yaml.Unmarshal(raw, &index); err != nil {
		return behavioralIndex{}, invalid("behavioral evaluation index is not valid YAML: %v", err)
	}
	if index.SchemaVersion != "1.0" || index.Provider != "fake" || len(index.Cases) == 0 {
		return behavioralIndex{}, invalid("behavioral evaluation index has an unsupported schemaVersion, provider, or no cases")
	}
	return index, nil
}

func runBehavioralEval(ctx context.Context, root string) (behavioralReport, error) {
	index, err := loadBehavioralIndex(root)
	if err != nil {
		return behavioralReport{}, err
	}
	provider := fake.New()
	if provider.Name() != "fake" || provider.Capabilities().Live {
		return behavioralReport{}, invalid("behavioral evaluation requires the offline fake provider")
	}
	report := behavioralReport{
		Provider: "fake",
		Cases:    len(index.Cases),
		Status:   "passed",
		Note:     "Deterministic fake-provider plumbing scenarios. No model was invoked and no model-quality pass rate is claimed.",
		Results:  []behavioralResult{},
	}
	seen := map[string]bool{}
	for _, item := range index.Cases {
		if seen[item.ID] {
			return behavioralReport{}, invalid("duplicate behavioral case id %q", item.ID)
		}
		seen[item.ID] = true
		result, err := runBehavioralCase(ctx, root, provider, item)
		if err != nil {
			return behavioralReport{}, err
		}
		report.Results = append(report.Results, result)
		if result.Passed {
			report.Passed++
		} else {
			report.Failed++
			report.Status = "failed"
		}
	}
	return report, nil
}

func runBehavioralCase(ctx context.Context, root string, provider *fake.Provider, item behavioralCase) (behavioralResult, error) {
	result := behavioralResult{ID: item.ID, EvidenceRecordIDs: []string{}}
	if !evalIdentifierPattern.MatchString(item.ID) || !evalIdentifierPattern.MatchString(item.Skill) ||
		!evalIdentifierPattern.MatchString(item.TaskID) || len(item.ExpectedFields) == 0 {
		return result, invalid("behavioral case %q is incomplete", item.ID)
	}
	validRoute := false
	for _, profile := range routing.Profiles {
		if item.Route == profile {
			validRoute = true
			break
		}
	}
	if !validRoute {
		return result, invalid("behavioral case %q uses an unknown routing profile", item.ID)
	}
	fixturePath := filepath.Join(root, filepath.FromSlash(item.PromptFixture))
	if !withinRoot(root, fixturePath) {
		return result, invalid("behavioral case %q fixture path escapes the repository", item.ID)
	}
	raw, err := os.ReadFile(fixturePath)
	if err != nil || len(strings.TrimSpace(string(raw))) == 0 {
		return result, invalid("behavioral case %q fixture is missing or empty", item.ID)
	}
	if util.SecretShapeCount(string(raw)) > 0 {
		return result, invalid("behavioral case %q fixture contains a credential shape", item.ID)
	}
	prompt := util.Sanitize(string(raw))
	runResult, err := provider.Run(ctx, integration.RunRequest{
		RunID:          "eval-behavioral-" + item.ID,
		TaskID:         item.TaskID,
		Skill:          item.Skill,
		Prompt:         prompt,
		RoutingProfile: item.Route,
	})
	if err != nil {
		return result, invalid("behavioral case %q provider call failed: %v", item.ID, err)
	}
	problems := []string{}
	if !runResult.Simulated || runResult.ExitStatus == nil || *runResult.ExitStatus != 0 || runResult.Usage.Known {
		problems = append(problems, "fake provider result is not a successful simulated run with unknown usage")
	}
	if runResult.Model != "" || runResult.RequestedModel != "" {
		problems = append(problems, "fake provider reported model identity")
	}
	for _, field := range item.ExpectedFields {
		if _, ok := runResult.Output[field]; !ok {
			problems = append(problems, "missing output field "+field)
		}
	}
	want := map[string]any{
		"simulated":      true,
		"taskId":         item.TaskID,
		"skill":          item.Skill,
		"routingProfile": item.Route,
		"requestedModel": "",
	}
	for key, value := range want {
		if got, ok := runResult.Output[key]; !ok || fmt.Sprint(got) != fmt.Sprint(value) {
			problems = append(problems, "output binding mismatch for "+key)
		}
	}
	outputText, ok := runResult.Output["result"].(string)
	if !ok || outputText != fmt.Sprintf("Simulated response to %q", prompt) {
		problems = append(problems, "sanitized fixture prompt was not echoed by the fake provider")
	}
	encoded, err := json.Marshal(runResult.Output)
	if err != nil {
		return result, invalid("behavioral case %q output cannot be encoded: %v", item.ID, err)
	}
	result.OutputDigest = util.Digest(encoded)
	result.Passed = len(problems) == 0
	result.Reason = "fake provider output matched authored field, routing, and sanitization assertions"
	if !result.Passed {
		result.Reason = strings.Join(problems, "; ")
	}
	return result, nil
}
