package execution

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/yoosuf/ae.sh/internal/evidence"
	"github.com/yoosuf/ae.sh/internal/util"
)

// ReviewFindingRef is a stable reference to a finding recorded in evidence.
type ReviewFindingRef struct {
	ID           string `json:"id"`
	RuleID       string `json:"ruleId,omitempty"`
	Severity     string `json:"severity,omitempty"`
	RecordID     string `json:"recordID"`
	DiffDigest   string `json:"diffDigest,omitempty"`
	CandidateSHA string `json:"candidateSHA,omitempty"`
}

// ReviewEvidenceRef identifies one immutable record that contributes to review state.
type ReviewEvidenceRef struct {
	RecordID   string `json:"recordId"`
	Kind       string `json:"kind"`
	DataDigest string `json:"dataDigest"`
}

// ReviewResolution is a human decision about one finding. Findings are never
// resolved by a provider claim alone.
type ReviewResolution struct {
	FindingID string `json:"findingId"`
	Status    string `json:"status"`
	Reason    string `json:"reason"`
	Actor     string `json:"actor"`
}

// ReviewEvidenceState reconstructs review findings and their resolutions from
// immutable run evidence. It is used on resume, not just in the process that
// originally produced a finding.
type ReviewEvidenceState struct {
	Findings   []ReviewFindingRef          `json:"findings"`
	Records    []ReviewEvidenceRef         `json:"records"`
	Resolved   map[string]ReviewResolution `json:"resolved"`
	Unresolved []string                    `json:"unresolved"`
	Digest     string                      `json:"digest"`
}

// CollectReviewEvidence reads review and remediation records for a run.
// Malformed records and stale candidate SHAs fail closed.
func CollectReviewEvidence(ctx context.Context, store Store, run Run) (ReviewEvidenceState, error) {
	state := ReviewEvidenceState{
		Findings: []ReviewFindingRef{}, Records: []ReviewEvidenceRef{},
		Resolved: map[string]ReviewResolution{}, Unresolved: []string{},
	}
	records, err := store.Evidence(ctx, run.ID)
	if err != nil {
		return state, err
	}
	for _, item := range records {
		var record evidence.Record
		if err := json.Unmarshal([]byte(item.JSON), &record); err != nil {
			return state, fmt.Errorf("parse evidence record for review state: %w", err)
		}
		switch record.Kind {
		case "review":
			if err := appendReviewFindings(&state, run, record); err != nil {
				return state, err
			}
			state.Records = append(state.Records, reviewEvidenceRef(record))
		case "remediation":
			if err := appendReviewResolution(&state, record); err != nil {
				return state, err
			}
			state.Records = append(state.Records, reviewEvidenceRef(record))
		}
	}
	for _, finding := range state.Findings {
		if _, ok := state.Resolved[finding.ID]; !ok {
			state.Unresolved = append(state.Unresolved, finding.ID)
		}
	}
	sort.Strings(state.Unresolved)
	state.Digest = reviewStateDigest(state)
	return state, nil
}

type reviewDigestInput struct {
	Findings    []ReviewFindingRef  `json:"findings"`
	Records     []ReviewEvidenceRef `json:"records"`
	Resolutions []ReviewResolution  `json:"resolutions"`
}

func reviewStateDigest(state ReviewEvidenceState) string {
	ids := make([]string, 0, len(state.Resolved))
	for id := range state.Resolved {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	resolutions := make([]ReviewResolution, 0, len(ids))
	for _, id := range ids {
		resolutions = append(resolutions, state.Resolved[id])
	}
	raw, _ := json.Marshal(reviewDigestInput{
		Findings: state.Findings, Records: state.Records, Resolutions: resolutions,
	})
	return util.Digest(raw)
}

func reviewEvidenceRef(record evidence.Record) ReviewEvidenceRef {
	return ReviewEvidenceRef{RecordID: record.RecordID, Kind: record.Kind, DataDigest: record.DataDigest}
}

func appendReviewFindings(state *ReviewEvidenceState, run Run, record evidence.Record) error {
	generic, err := util.JSONValue(record.Data)
	if err != nil || generic == nil {
		return fmt.Errorf("review evidence %s has an invalid data object", record.RecordID)
	}
	data, ok := generic.(map[string]any)
	if !ok {
		return fmt.Errorf("review evidence %s has an invalid data object", record.RecordID)
	}
	candidate, _ := data["candidateSHA"].(string)
	diffDigest, _ := data["diffDigest"].(string)
	findings := reviewFindings(data)
	if len(findings) == 0 {
		if output, ok := data["output"].(map[string]any); ok {
			candidate, _ = output["candidateSHA"].(string)
			diffDigest, _ = output["diffDigest"].(string)
			findings = reviewFindings(output)
		}
	}
	if candidate != "" && run.CandidateSHA != "" && candidate != run.CandidateSHA {
		return fmt.Errorf("review evidence %s has a stale candidate SHA", record.RecordID)
	}
	for _, value := range findings {
		ref := ReviewFindingRef{RecordID: record.RecordID, DiffDigest: diffDigest, CandidateSHA: candidate}
		ref.ID, _ = value["id"].(string)
		ref.RuleID, _ = value["ruleId"].(string)
		ref.Severity, _ = value["severity"].(string)
		if ref.ID == "" {
			return fmt.Errorf("review evidence %s contains a finding without an id", record.RecordID)
		}
		state.Findings = append(state.Findings, ref)
	}
	return nil
}

func reviewFindings(data map[string]any) []map[string]any {
	value, ok := data["findings"].([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(value))
	for _, item := range value {
		finding, ok := item.(map[string]any)
		if ok {
			out = append(out, finding)
		}
	}
	return out
}

func appendReviewResolution(state *ReviewEvidenceState, record evidence.Record) error {
	generic, err := util.JSONValue(record.Data)
	if err != nil || generic == nil {
		return fmt.Errorf("remediation evidence %s has an invalid data object", record.RecordID)
	}
	data, ok := generic.(map[string]any)
	if !ok {
		return fmt.Errorf("remediation evidence %s has an invalid data object", record.RecordID)
	}
	id, _ := data["findingId"].(string)
	status, _ := data["status"].(string)
	reason, _ := data["reason"].(string)
	switch status {
	case "fixed", "accepted", "false-positive":
	default:
		return fmt.Errorf("remediation evidence %s has an unsupported status %q", record.RecordID, status)
	}
	if id == "" || reason == "" || record.Actor == "" {
		return fmt.Errorf("remediation evidence %s is missing finding, reason, or actor", record.RecordID)
	}
	state.Resolved[id] = ReviewResolution{FindingID: id, Status: status, Reason: reason, Actor: record.Actor}
	return nil
}
