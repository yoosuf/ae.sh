package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/aef-framework/aef/internal/config"
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

var evalCasePattern = regexp.MustCompile(`(?m)^## Case [0-9]+`)

func (a *App) cmdEval(args []string) error {
	fs := newFlagSet("eval")
	suite := fs.String("suite", "skills", "skills, security, or all")
	execute := fs.Bool("execute", false, "run deterministic structural checks on authored fixtures")
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
	result := map[string]any{
		"suite": *suite, "executed": false, "modelBehaviorExecuted": false,
		"mode": "catalogue-only",
	}
	if !*execute {
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
	result["status"] = "passed"
	result["note"] = "Only fixture structure was executed. Provider scoring and behavioral pass rates remain unexecuted; no model was invoked."
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
