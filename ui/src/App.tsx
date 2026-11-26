import type * as React from "react";
import type {
  Approval,
  EvidenceRow,
  Finding,
  Snapshot,
  TraceStage,
  Transition,
} from "./types";

function Card(props: { title: string; note?: string; children: React.ReactNode }) {
  return (
    <section className="card">
      <h2>{props.title}</h2>
      {props.note ? <p className="muted">{props.note}</p> : null}
      {props.children}
    </section>
  );
}

function Rows(props: { items: Array<[string, React.ReactNode]> }) {
  return (
    <div className="list">
      {props.items.map(([label, value]) => (
        <div className="kv" key={label}>
          <span className="key">{label}</span>
          <span className="value">{value === "" || value == null ? "-" : value}</span>
        </div>
      ))}
    </div>
  );
}

function Stage({ stage }: { stage: TraceStage }) {
  return (
    <div className="item">
      <span className={`badge badge-${stage.status}`}>{stage.status}</span>
      <strong>{stage.title || stage.id}</strong>
      <small>{stage.evidence.length} evidence record(s)</small>
    </div>
  );
}

function ApprovalRow({ approval }: { approval: Approval }) {
  return (
    <div className="item">
      <strong>{approval.gate}</strong>
      <span>
        {approval.approver} · {approval.role || "no role"}
      </span>
      <small>
        {approval.identitySource} · {approval.candidateSHA}
      </small>
    </div>
  );
}

function FindingRow({ finding }: { finding: Finding }) {
  return (
    <div className="item">
      <span className="badge badge-hypothesis">hypothesis</span>
      <strong>{finding.id}</strong>
      <span>
        {finding.severity || "unspecified"} · {finding.ruleId || "no rule"}
      </span>
      <small>{finding.recordId}</small>
    </div>
  );
}

function EvidenceItem({ row }: { row: EvidenceRow }) {
  return (
    <div className="item">
      <span className="seq">{row.sequence}</span>
      <span className="badge">{row.kind}</span>
      <span>{row.subject}</span>
      <small>
        {row.dataDigest}
        {row.dataOmitted ? " · data omitted" : ""}
      </small>
    </div>
  );
}

function HistoryItem({ transition }: { transition: Transition }) {
  return (
    <div className="item">
      <span className="seq">{transition.sequence}</span>
      <span>
        {transition.from} → {transition.to}
      </span>
      <small>
        {transition.taskId || "-"} · {transition.timestamp}
      </small>
    </div>
  );
}

export function App({ snapshot }: { snapshot: Snapshot }) {
  const run = snapshot.run;
  return (
    <main>
      <header className="topbar">
        <span className="brand">ae.sh</span>
        <span className="tag">read-only local snapshot</span>
      </header>
      <h1>Run {run.id}</h1>
      <p className="muted">
        Generated {snapshot.generatedAt} · {snapshot.note}
      </p>
      <Card title="Status">
        <Rows
          items={[
            ["Status", run.status],
            ["State", run.state],
            ["Risk", run.risk],
            ["Provider", run.provider],
            ["Issue", run.issueKey],
            ["Workflow", `${run.workflowId} @ ${run.workflowVersion}`],
            ["Base SHA", run.baseSHA],
            ["Candidate SHA", run.candidateSHA],
            ["Terminal reason", run.terminalReason],
          ]}
        />
      </Card>
      <Card title="Traceability" note={snapshot.trace.note}>
        {snapshot.trace.stages.map((stage) => (
          <Stage key={stage.id} stage={stage} />
        ))}
      </Card>
      <Card title="History">
        {snapshot.history.map((transition) => (
          <HistoryItem key={transition.sequence} transition={transition} />
        ))}
      </Card>
      <Card title="Approvals">
        {snapshot.trace.approvals.map((approval, index) => (
          <ApprovalRow key={`${approval.gate}-${index}`} approval={approval} />
        ))}
      </Card>
      <Card title="Review findings" note={snapshot.review.note}>
        {snapshot.review.findings.map((finding) => (
          <FindingRow key={finding.id} finding={finding} />
        ))}
        {snapshot.review.unresolved.length > 0 ? (
          <p className="muted">
            Unresolved: {snapshot.review.unresolved.join(", ")}
          </p>
        ) : null}
      </Card>
      <Card title="Evidence chain">
        {snapshot.evidence.map((row) => (
          <EvidenceItem key={row.sequence} row={row} />
        ))}
      </Card>
    </main>
  );
}
