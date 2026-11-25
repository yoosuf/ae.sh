// Package execution implements the single authoritative workflow state model
// and the local run engine.
package execution

import "fmt"

// State is a workflow execution state.
type State string

const (
	Queued                 State = "queued"
	Analyzing              State = "analyzing"
	Planning               State = "planning"
	AwaitingApproval       State = "awaiting_approval"
	Implementing           State = "implementing"
	Verifying              State = "verifying"
	Reviewing              State = "reviewing"
	Remediating            State = "remediating"
	AwaitingMerge          State = "awaiting_merge"
	Staging                State = "staging"
	AwaitingRelease        State = "awaiting_release"
	ProductionVerification State = "production_verification"
	Completed              State = "completed"
	Blocked                State = "blocked"
	Failed                 State = "failed"
	Cancelled              State = "cancelled"
)

// Status is the lifecycle of a run independent of its workflow state.
const (
	StatusRunning          = "running"
	StatusAwaitingApproval = "awaiting-approval"
	StatusCompleted        = "completed"
	StatusFailed           = "failed"
	StatusCancelled        = "cancelled"
	StatusBlocked          = "blocked"
)

// AllowedTransitions is the complete state graph. Cycles are limited to the
// explicit remediation path; all other regressions are rejected.
var AllowedTransitions = map[State][]State{
	Queued:                 {Analyzing, Blocked, Failed, Cancelled},
	Analyzing:              {Planning, Blocked, Failed, Cancelled},
	Planning:               {AwaitingApproval, Blocked, Failed, Cancelled},
	AwaitingApproval:       {Implementing, Planning, Blocked, Failed, Cancelled},
	Implementing:           {Verifying, Remediating, Blocked, Failed, Cancelled},
	Verifying:              {Reviewing, Remediating, Blocked, Failed, Cancelled},
	Reviewing:              {Remediating, AwaitingMerge, Blocked, Failed, Cancelled},
	Remediating:            {Implementing, Blocked, Failed, Cancelled},
	AwaitingMerge:          {Staging, Completed, Blocked, Failed, Cancelled},
	Staging:                {AwaitingRelease, Blocked, Failed, Cancelled},
	AwaitingRelease:        {ProductionVerification, Blocked, Failed, Cancelled},
	ProductionVerification: {Completed, Blocked, Failed, Cancelled},
	Completed:              {},
	Blocked:                {Analyzing, Planning, Implementing, Cancelled, Failed},
	Failed:                 {},
	Cancelled:              {},
}

// ValidateTransition returns an error when from -> to is not permitted.
func ValidateTransition(from, to State) error {
	for _, allowed := range AllowedTransitions[from] {
		if allowed == to {
			return nil
		}
	}
	return fmt.Errorf("invalid state transition %s -> %s", from, to)
}

// IsTerminal reports whether a state ends the run.
func IsTerminal(state State) bool {
	return state == Completed || state == Failed || state == Cancelled
}

// ParseState parses and validates a state name.
func ParseState(value string) (State, error) {
	state := State(value)
	if _, ok := AllowedTransitions[state]; !ok {
		return "", fmt.Errorf("unknown workflow state %q", value)
	}
	return state, nil
}

// LocalTerminalState is used when deployment is out of scope. AEF stops at the
// merge-eligible state and records why it did not invent production evidence.
const LocalTerminalState = AwaitingMerge
