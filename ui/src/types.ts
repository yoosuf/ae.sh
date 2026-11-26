export interface Run {
  id: string;
  workflowId: string;
  workflowVersion: string;
  issueKey: string;
  state: string;
  status: string;
  risk: string;
  provider: string;
  baseSHA: string;
  candidateSHA: string;
  terminalReason?: string;
}

export interface TraceStage {
  id: string;
  title: string;
  status: "missing" | "observed" | "failed" | "approved";
  evidence: unknown[];
}

export interface Approval {
  gate: string;
  approver: string;
  role?: string;
  identitySource: string;
  candidateSHA: string;
}

export interface Finding {
  id: string;
  ruleId?: string;
  severity?: string;
  recordId: string;
  hypothesis: boolean;
}

export interface EvidenceRow {
  sequence: number;
  kind: string;
  subject: string;
  dataDigest: string;
  dataOmitted?: boolean;
}

export interface Transition {
  sequence: number;
  from: string;
  to: string;
  taskId?: string;
  timestamp: string;
}

export interface Snapshot {
  schemaVersion: string;
  generatedAt: string;
  run: Run;
  trace: { stages: TraceStage[]; approvals: Approval[]; note?: string };
  review: { findings: Finding[]; unresolved: string[]; note?: string };
  evidence: EvidenceRow[];
  history: Transition[];
  note: string;
}
