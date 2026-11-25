package execution

import "context"

// Run is a persisted workflow execution.
type Run struct {
	ID                  string   `json:"id"`
	WorkflowID          string   `json:"workflowId"`
	WorkflowVersion     string   `json:"workflowVersion"`
	IssueKey            string   `json:"issueKey"`
	IssuePath           string   `json:"issuePath"`
	State               string   `json:"state"`
	Status              string   `json:"status"`
	Risk                string   `json:"risk"`
	Provider            string   `json:"provider"`
	BaseSHA             string   `json:"baseSHA"`
	CandidateSHA        string   `json:"candidateSHA"`
	PlanDigest          string   `json:"planDigest"`
	Workspace           string   `json:"workspace,omitempty"`
	CurrentTask         int      `json:"currentTask"`
	RemediationAttempts int      `json:"remediationAttempts"`
	Profiles            []string `json:"profiles"`
	Actor               string   `json:"actor"`
	TerminalReason      string   `json:"terminalReason,omitempty"`
	CreatedAt           string   `json:"createdAt"`
	UpdatedAt           string   `json:"updatedAt"`
	Version             int64    `json:"version"`
}

// Transition is an immutable state change record.
type Transition struct {
	RunID          string `json:"runId"`
	Sequence       int    `json:"sequence"`
	From           string `json:"from"`
	To             string `json:"to"`
	TaskID         string `json:"taskId"`
	Timestamp      string `json:"timestamp"`
	EvidenceDigest string `json:"evidenceDigest,omitempty"`
}

// ApprovalRecord is an immutable, SHA-bound human decision.
type ApprovalRecord struct {
	RunID          string `json:"runId"`
	Gate           string `json:"gate"`
	Approver       string `json:"approver"`
	Role           string `json:"role,omitempty"`
	IdentitySource string `json:"identitySource"`
	ArtifactDigest string `json:"artifactDigest"`
	CandidateSHA   string `json:"candidateSHA"`
	ReviewDigest   string `json:"reviewDigest,omitempty"`
	Timestamp      string `json:"timestamp"`
}

// EvidenceRecord is the JSON representation of one evidence entry.
type EvidenceRecord struct {
	RunID    string `json:"runId"`
	Sequence int    `json:"sequence"`
	JSON     string `json:"json"`
}

// Artifact is a content-addressed object.
type Artifact struct {
	Digest string `json:"digest"`
	Kind   string `json:"kind"`
	Raw    []byte `json:"raw"`
}

// Store is the persistence port used by the engine. Implementations must make
// UpdateRun conditional on the expected version and append transitions and
// evidence in the same transaction where possible.
type Store interface {
	CreateRun(ctx context.Context, run Run) error
	GetRun(ctx context.Context, id string) (Run, error)
	UpdateRun(ctx context.Context, run Run, expectedVersion int64) error
	ListRuns(ctx context.Context, limit int) ([]Run, error)

	AppendTransition(ctx context.Context, transition Transition) error
	// AppendStep records a transition and its evidence atomically.
	AppendStep(ctx context.Context, transition Transition, evidence *EvidenceRecord) error
	Transitions(ctx context.Context, runID string) ([]Transition, error)

	SaveApproval(ctx context.Context, approval ApprovalRecord) error
	Approval(ctx context.Context, runID, gate string) (ApprovalRecord, error)
	Approvals(ctx context.Context, runID, gate string) ([]ApprovalRecord, error)

	AppendEvidence(ctx context.Context, record EvidenceRecord) error
	Evidence(ctx context.Context, runID string) ([]EvidenceRecord, error)

	PutArtifact(ctx context.Context, artifact Artifact) error
	GetArtifact(ctx context.Context, digest string) (Artifact, error)

	Enqueue(ctx context.Context, kind, payload string) error
	Outbox(ctx context.Context, runID string) ([]OutboxEntry, error)

	Close() error
}

// OutboxEntry is a pending side effect.
type OutboxEntry struct {
	ID        int64  `json:"id"`
	RunID     string `json:"runId"`
	Kind      string `json:"kind"`
	Payload   string `json:"payload"`
	CreatedAt string `json:"createdAt"`
}

// ErrNotFound is returned when a requested entity does not exist.
type ErrNotFound struct {
	Kind string
	ID   string
}

func (e ErrNotFound) Error() string { return e.Kind + " not found: " + e.ID }

// ErrConflict is returned on optimistic concurrency failure.
type ErrConflict struct {
	ID string
}

func (e ErrConflict) Error() string {
	return "run " + e.ID + " was modified concurrently; retry with fresh state"
}
