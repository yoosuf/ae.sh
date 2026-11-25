// Package policy implements fail-closed authorization and deterministic risk
// classification. Runtime safety controls are code, not editable YAML.
package policy

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/aef-framework/aef/internal/schema"
	"github.com/aef-framework/aef/internal/util"
)

// RiskLevel orders risk from least to most severe.
type RiskLevel string

const (
	RiskLow      RiskLevel = "low"
	RiskMedium   RiskLevel = "medium"
	RiskHigh     RiskLevel = "high"
	RiskCritical RiskLevel = "critical"
	RiskUnknown  RiskLevel = "unknown"
)

var riskRank = map[RiskLevel]int{
	RiskUnknown: 0, RiskLow: 1, RiskMedium: 2, RiskHigh: 3, RiskCritical: 4,
}

// Rank returns the ordinal severity of a risk level.
func Rank(level RiskLevel) int { return riskRank[level] }

// Policy is a validated policy document.
type Policy struct {
	SchemaVersion string         `yaml:"schemaVersion" json:"schemaVersion"`
	ID            string         `yaml:"id" json:"id"`
	Version       string         `yaml:"version" json:"version"`
	Precedence    []string       `yaml:"precedence" json:"precedence"`
	AppliesTo     []RiskLevel    `yaml:"appliesTo,omitempty" json:"appliesTo,omitempty"`
	RiskRules     []RiskRule     `yaml:"riskRules" json:"riskRules"`
	Escalations   []Escalation   `yaml:"escalations" json:"escalations"`
	Gates         Gates          `yaml:"gates" json:"gates"`
	ApprovalRules []ApprovalRule `yaml:"approvalRules" json:"approvalRules"`
	Path          string         `yaml:"-" json:"-"`
}

type RiskRule struct {
	ID        string    `yaml:"id" json:"id"`
	Risk      RiskLevel `yaml:"risk" json:"risk"`
	Paths     []string  `yaml:"paths" json:"paths"`
	Semantics []string  `yaml:"semantics" json:"semantics"`
}

type Escalation struct {
	ID       string   `yaml:"id" json:"id"`
	Triggers []string `yaml:"triggers" json:"triggers"`
	Require  string   `yaml:"require" json:"require"`
}

type Gates struct {
	Required                 []string `yaml:"required" json:"required"`
	NeverMarkMissingAsPassed bool     `yaml:"neverMarkMissingAsPassed" json:"neverMarkMissingAsPassed"`
}

type ApprovalRule struct {
	Gate                 string   `yaml:"gate" json:"gate"`
	MinApprovers         int      `yaml:"minApprovers" json:"minApprovers"`
	RejectSelfApproval   bool     `yaml:"rejectSelfApproval" json:"rejectSelfApproval"`
	RequireDistinctRoles bool     `yaml:"requireDistinctRoles,omitempty" json:"requireDistinctRoles,omitempty"`
	InvalidateOn         []string `yaml:"invalidateOn" json:"invalidateOn"`
}

// Load parses and validates one policy file.
func Load(path string) (Policy, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Policy{}, err
	}
	return Parse(path, raw)
}

func Parse(path string, raw []byte) (Policy, error) {
	var generic map[string]any
	if err := yaml.Unmarshal(raw, &generic); err != nil {
		return Policy{}, fmt.Errorf("parse policy YAML: %w", err)
	}
	doc, err := util.JSONValue(generic)
	if err != nil {
		return Policy{}, err
	}
	if err := schema.Validate("policy", doc); err != nil {
		return Policy{}, err
	}
	var p Policy
	if err := yaml.Unmarshal(raw, &p); err != nil {
		return Policy{}, err
	}
	p.Path = filepath.ToSlash(path)
	if len(p.Precedence) == 0 {
		return Policy{}, fmt.Errorf("policy %s has no precedence declaration", p.ID)
	}
	for _, level := range p.AppliesTo {
		if _, ok := riskRank[level]; !ok || level == RiskUnknown {
			return Policy{}, fmt.Errorf("policy %s has invalid appliesTo level %q", p.ID, level)
		}
	}
	return p, nil
}

// Bundle merges policies in increasing precedence order. Later policies
// override earlier ones for gates and approvals; risk rules accumulate.
type Bundle struct {
	Policies []Policy
}

func NewBundle(policies ...Policy) Bundle { return Bundle{Policies: policies} }

// LoadBundle loads all repository policies in deterministic filename order.
func LoadBundle(root string, names []string) (Bundle, error) {
	var out Bundle
	var errs []string
	for _, name := range names {
		path := filepath.Join(root, filepath.FromSlash(name))
		if filepath.IsAbs(name) || strings.HasPrefix(filepath.Clean(name), "..") {
			return out, fmt.Errorf("policy path must be repository-relative: %q", name)
		}
		p, err := Load(path)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		out.Policies = append(out.Policies, p)
	}
	if len(errs) > 0 {
		return out, fmt.Errorf("policy load failed: %s", strings.Join(errs, "; "))
	}
	return out, nil
}

// RequiredGates returns the union of mandatory gates across all policies.
func (b Bundle) RequiredGates() []string { return b.RequiredGatesFor(RiskCritical) }

// RequiredGatesFor returns gates applicable at a risk level. A policy may
// narrow its scope with appliesTo; policies without that field always apply.
func (b Bundle) RequiredGatesFor(level RiskLevel) []string {
	set := map[string]bool{}
	for _, p := range b.Policies {
		if !policyApplies(p, level) {
			continue
		}
		for _, gate := range p.Gates.Required {
			set[gate] = true
		}
	}
	out := make([]string, 0, len(set))
	for gate := range set {
		out = append(out, gate)
	}
	sort.Strings(out)
	return out
}

func policyApplies(p Policy, level RiskLevel) bool {
	if len(p.AppliesTo) == 0 {
		return true
	}
	for _, allowed := range p.AppliesTo {
		if allowed == level {
			return true
		}
	}
	return false
}

// Classify computes risk from changed paths and semantic tags. It never uses an
// author-provided self-rating. Unknown input yields RiskUnknown.
func (b Bundle) Classify(changedPaths, semantics []string) Classification {
	result := Classification{Level: RiskUnknown, Matched: []string{}, Unknown: []string{}}
	known := map[string]bool{}
	for _, p := range b.Policies {
		for _, rule := range p.RiskRules {
			for _, tag := range rule.Semantics {
				known[tag] = true
			}
		}
	}
	for _, tag := range semantics {
		if !known[tag] {
			result.Unknown = append(result.Unknown, tag)
		}
	}
	for _, p := range b.Policies {
		for _, rule := range p.RiskRules {
			pathMatch := matchesAny(changedPaths, rule.Paths)
			semanticMatch := matchesAny(semantics, rule.Semantics)
			if len(rule.Semantics) > 0 && !semanticMatch {
				continue
			}
			if !pathMatch {
				continue
			}
			level := rule.Risk
			if level == "" {
				level = RiskUnknown
			}
			if Rank(level) > Rank(result.Level) {
				result.Level = level
			}
			result.Matched = append(result.Matched, rule.ID)
		}
	}
	sort.Strings(result.Matched)
	sort.Strings(result.Unknown)
	if len(result.Unknown) > 0 {
		// Unrecognized semantics cannot be safely downgraded by a path rule.
		result.Level = RiskUnknown
	}
	result.Escalations = b.EscalationsFor(semantics)
	return result
}

// Classification is the deterministic result of risk analysis.
type Classification struct {
	Level       RiskLevel `json:"level"`
	Matched     []string  `json:"matched"`
	Unknown     []string  `json:"unknown,omitempty"`
	Escalations []string  `json:"escalations"`
}

// EscalationsFor returns required escalation identifiers for semantic tags.
func (b Bundle) EscalationsFor(semantics []string) []string {
	set := map[string]bool{}
	for _, p := range b.Policies {
		for _, escalation := range p.Escalations {
			for _, trigger := range escalation.Triggers {
				if matchesAny(semantics, []string{trigger}) {
					set[escalation.ID] = true
				}
			}
		}
	}
	out := make([]string, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// Approval binds a human decision to an exact artifact and candidate SHA.
type Approval struct {
	Gate           string `json:"gate"`
	Approver       string `json:"approver"`
	IdentitySource string `json:"identitySource"` // "organization" or "local-manual"
	ArtifactDigest string `json:"artifactDigest"`
	CandidateSHA   string `json:"candidateSHA"`
	Timestamp      string `json:"timestamp"`
}

// Decision is the outcome of a policy check.
type Decision struct {
	Allowed          bool     `json:"allowed"`
	Reasons          []string `json:"reasons"`
	RequiredGates    []string `json:"requiredGates"`
	RequiredApproval string   `json:"requiredApproval,omitempty"`
}

// Check evaluates an action against immutable safety controls and loaded
// policies. It denies on unknown risk, missing evidence, or unknown action.
func (b Bundle) Check(action Action, classification Classification, evidence map[string]bool) Decision {
	d := Decision{Allowed: true, RequiredGates: b.RequiredGatesFor(action.Risk)}
	reasons := []string{}
	deny := func(reason string) {
		d.Allowed = false
		reasons = append(reasons, reason)
	}
	if classification.Level == RiskUnknown {
		if len(classification.Unknown) > 0 {
			deny("unknown semantics: " + strings.Join(classification.Unknown, ", ") + " (fail closed)")
		} else {
			deny("unknown risk: fail closed")
		}
	}
	if action.GatesRequired {
		for _, gate := range d.RequiredGates {
			if !evidence[gate] {
				deny("missing required evidence: " + gate)
			}
		}
	}
	for _, rule := range immutableSafetyRules {
		if rule.matches(action) {
			if rule.deny {
				deny(rule.reason)
			} else {
				reasons = append(reasons, rule.reason)
			}
		}
	}
	if Rank(classification.Level) >= Rank(RiskCritical) && action.OrganizationIdentityRequired && action.Approval == nil {
		deny("critical changes require an organization-authenticated approver")
	}
	if action.Approval != nil && action.Approval.IdentitySource == "local-manual" && action.OrganizationIdentityRequired {
		deny("local manual approval cannot satisfy organization identity verification")
	}
	if action.Approval != nil {
		approval := action.Approval
		if approval.Gate == "" || approval.Approver == "" || approval.ArtifactDigest == "" || approval.CandidateSHA == "" {
			deny("approval is missing gate, approver, artifact digest, or candidate SHA")
		}
		if approval.ArtifactDigest != action.ArtifactDigest {
			deny("stale approval: artifact digest does not match candidate")
		}
		if approval.CandidateSHA != action.CandidateSHA {
			deny("stale approval: candidate SHA changed")
		}
		if action.SelfApproval {
			deny("self-approval is not permitted")
		}
	}
	d.Reasons = reasons
	return d
}

// Action describes a candidate mutation awaiting authorization.
type Action struct {
	Name                         string
	CandidateSHA                 string
	ArtifactDigest               string
	Risk                         RiskLevel
	GatesRequired                bool
	OrganizationIdentityRequired bool
	SelfApproval                 bool
	Approval                     *Approval
}

type safetyRule struct {
	name    string
	deny    bool
	reason  string
	matches func(Action) bool
}

// Immutable runtime controls cannot be relaxed by repository policy.
var immutableSafetyRules = []safetyRule{
	{
		name:    "no-agent-deploy",
		deny:    true,
		reason:  "coding agents must not receive deployment permissions",
		matches: func(a Action) bool { return a.Name == "deploy" && a.Approval == nil },
	},
	{
		name:    "no-force-push",
		deny:    true,
		reason:  "force push to shared branches is forbidden",
		matches: func(a Action) bool { return a.Name == "force-push" },
	},
	{
		name:    "no-unrestricted-shell",
		deny:    true,
		reason:  "arbitrary shell execution from task content is forbidden",
		matches: func(a Action) bool { return a.Name == "unrestricted-shell" },
	},
	{
		name:    "no-failed-gate-bypass",
		deny:    true,
		reason:  "failed gates cannot be bypassed",
		matches: func(a Action) bool { return a.Name == "bypass-gate" },
	},
}

// ApprovalRule returns the highest-precedence rule for a gate, or an error for
// unknown gates. It is equivalent to ApprovalRuleFor(gate, RiskCritical).
func (b Bundle) ApprovalRule(gate string) (ApprovalRule, error) {
	return b.ApprovalRuleFor(gate, RiskCritical)
}

// ApprovalRuleFor returns the applicable rule for a gate at a risk level.
// Policies scoped with appliesTo are skipped when their scope does not match.
func (b Bundle) ApprovalRuleFor(gate string, risk RiskLevel) (ApprovalRule, error) {
	for i := len(b.Policies) - 1; i >= 0; i-- {
		if !policyApplies(b.Policies[i], risk) {
			continue
		}
		for _, rule := range b.Policies[i].ApprovalRules {
			if rule.Gate == gate {
				return rule, nil
			}
		}
	}
	return ApprovalRule{}, fmt.Errorf("unknown approval gate %q: fail closed", gate)
}

func matchesAny(values, patterns []string) bool {
	for _, value := range values {
		for _, pattern := range patterns {
			if globMatch(pattern, value) {
				return true
			}
		}
	}
	return false
}

// globMatch supports '*' and '**' path segments without regexp surprises.
func globMatch(pattern, value string) bool {
	pattern = filepath.ToSlash(strings.TrimPrefix(pattern, "./"))
	value = filepath.ToSlash(strings.TrimPrefix(value, "./"))
	if pattern == value {
		return true
	}
	if pattern == "**" || pattern == "*" {
		return true
	}
	pSegments := strings.Split(pattern, "/")
	vSegments := strings.Split(value, "/")
	return matchSegments(pSegments, vSegments)
}

func matchSegments(p, v []string) bool {
	if len(p) == 0 {
		return len(v) == 0
	}
	if p[0] == "**" {
		for i := 0; i <= len(v); i++ {
			if matchSegments(p[1:], v[i:]) {
				return true
			}
		}
		return false
	}
	if len(v) == 0 {
		return false
	}
	if segmentMatch(p[0], v[0]) {
		return matchSegments(p[1:], v[1:])
	}
	return false
}

func segmentMatch(pattern, value string) bool {
	if pattern == "*" {
		return true
	}
	if !strings.Contains(pattern, "*") {
		return pattern == value
	}
	parts := strings.Split(pattern, "*")
	if !strings.HasPrefix(value, parts[0]) {
		return false
	}
	value = value[len(parts[0]):]
	for i := 1; i < len(parts)-1; i++ {
		idx := strings.Index(value, parts[i])
		if idx < 0 {
			return false
		}
		value = value[idx+len(parts[i]):]
	}
	return len(parts) == 1 || strings.HasSuffix(value, parts[len(parts)-1])
}
