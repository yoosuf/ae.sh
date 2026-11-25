package cli

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/aef-framework/aef/internal/util"
)

const reviewNote = "Local pattern-scan hypotheses only. Not an independent review; no code was changed and no checks were run."

// ReviewFinding is a hypothesis derived from a local diff. It is not a
// verified vulnerability and never substitutes for human review or tests.
type ReviewFinding struct {
	ID          string `json:"id"`
	RuleID      string `json:"ruleId"`
	Severity    string `json:"severity"`
	Status      string `json:"status"`
	File        string `json:"file"`
	Line        int    `json:"line"`
	Evidence    string `json:"evidence"`
	Impact      string `json:"impact"`
	Remediation string `json:"remediation"`
	Verified    bool   `json:"verified"`
}

// ReviewResult is the canonical structured output of a local review scan.
type ReviewResult struct {
	SchemaVersion         string          `json:"schemaVersion"`
	Mode                  string          `json:"mode"`
	Verified              bool            `json:"verified"`
	PR                    int             `json:"pr,omitempty"`
	Source                string          `json:"source"`
	DiffDigest            string          `json:"diffDigest"`
	CandidateSHA          string          `json:"candidateSHA,omitempty"`
	BaseSHA               string          `json:"baseSHA,omitempty"`
	EvidenceRecordID      string          `json:"evidenceRecordID,omitempty"`
	Findings              []ReviewFinding `json:"findings"`
	ChecksWithoutFindings []string        `json:"checksWithoutFindings"`
	Note                  string          `json:"note"`
}

type reviewRule struct {
	id                            string
	pattern                       *regexp.Regexp
	severity, impact, remediation string
}

var reviewPatterns = []reviewRule{
	{"unsafe-html", regexp.MustCompile(`dangerouslySetInnerHTML`), "high", "Untrusted HTML may be rendered, enabling stored XSS.", "Sanitize input or use a safe renderer."},
	{"shell-concatenation", regexp.MustCompile(`(?i)exec\.Command\([^)]*\+`), "high", "Command arguments may be assembled from untrusted input.", "Use argument arrays with fixed program names."},
	{"credential-literal", regexp.MustCompile(`(?i)(password|secret|token)\s*=\s*"`), "high", "A credential-like literal may be committed.", "Move the value to a secret store."},
	{"credential-env", regexp.MustCompile(`(?i)os\.Getenv\("(AWS_SECRET|OPENAI_API|ANTHROPIC_API)`), "medium", "Credential environment variables may be exposed in logs.", "Load secrets through a scoped adapter and redact logs."},
	{"sql-injection", regexp.MustCompile(`(?i)(SELECT|UPDATE|DELETE).*\+.*req\.`), "critical", "Possible SQL injection from request data.", "Use parameterized queries."},
	{"tenant-scope", regexp.MustCompile(`(?i)tenant(Id|_id)?\s*==\s*`), "medium", "Tenant checks may be incomplete; verify all data paths.", "Enforce tenant isolation in the data layer."},
}

// ScanDiff performs a deterministic local pattern scan over a unified diff.
// Findings are explicitly hypotheses: the scanner does not understand control
// flow and does not claim to be an independent code review.
func ScanDiff(path string) (ReviewResult, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ReviewResult{}, err
	}
	sum := sha256.Sum256(raw)
	result := ReviewResult{
		SchemaVersion:         "1.0",
		Mode:                  "local-pattern-scan",
		Verified:              false,
		Source:                util.Sanitize(path),
		DiffDigest:            "sha256:" + hex.EncodeToString(sum[:]),
		Findings:              []ReviewFinding{},
		ChecksWithoutFindings: []string{},
		Note:                  reviewNote,
	}
	hits := make(map[string]bool, len(reviewPatterns))
	currentFile := ""
	lineNo := 0
	scanner := bufio.NewScanner(strings.NewReader(string(raw)))
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "+++ "):
			currentFile = strings.TrimPrefix(line, "+++ ")
		case strings.HasPrefix(line, "@@"):
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				_, _ = fmt.Sscanf(parts[2], "+%d", &lineNo)
				if lineNo > 0 {
					lineNo--
				}
			}
		case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
			lineNo++
			for _, rule := range reviewPatterns {
				if rule.pattern.MatchString(line) {
					hits[rule.id] = true
					result.Findings = append(result.Findings, newFinding(rule, currentFile, lineNo, line))
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return ReviewResult{}, err
	}
	for _, rule := range reviewPatterns {
		if !hits[rule.id] {
			result.ChecksWithoutFindings = append(result.ChecksWithoutFindings, rule.id)
		}
	}
	return result, nil
}

func newFinding(rule reviewRule, file string, line int, raw string) ReviewFinding {
	evidence := util.Sanitize(strings.TrimPrefix(strings.TrimSpace(raw), "+"))
	seed := strings.Join([]string{rule.id, file, fmt.Sprint(line), evidence}, "\x00")
	sum := sha256.Sum256([]byte(seed))
	return ReviewFinding{
		ID:          "rev-" + hex.EncodeToString(sum[:])[:16],
		RuleID:      rule.id,
		Severity:    rule.severity,
		Status:      "hypothesis",
		File:        file,
		Line:        line,
		Evidence:    evidence,
		Impact:      rule.impact,
		Remediation: rule.remediation,
		Verified:    false,
	}
}
