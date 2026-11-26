package cli

// These assets are written by `ae dashboard snapshot`. They are deliberately
// dependency-free so a snapshot opens from disk without a server, CDN, or API.
const dashboardIndexHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>ae.sh · run snapshot</title>
  <link rel="stylesheet" href="./styles.css">
</head>
<body>
  <header class="topbar">
    <span class="brand">ae.sh</span>
    <span class="tag">read-only local snapshot</span>
  </header>
  <main id="root"></main>
  <script id="ae-snapshot" type="application/json">__AE_SNAPSHOT__</script>
  <script src="./app.js"></script>
</body>
</html>
`

const dashboardAppJS = `(function () {
  "use strict";
  var source = document.getElementById("ae-snapshot");
  var root = document.getElementById("root");
  if (!source || !root) return;

  function el(tag, className, value) {
    var node = document.createElement(tag);
    if (className) node.className = className;
    if (value !== undefined && value !== null) node.textContent = String(value);
    return node;
  }

  function section(title, children, subtitle) {
    var box = el("section", "card");
    box.appendChild(el("h2", "", title));
    if (subtitle) box.appendChild(el("p", "muted", subtitle));
    children.forEach(function (child) { box.appendChild(child); });
    return box;
  }

  function row(label, value) {
    var line = el("div", "kv");
    line.appendChild(el("span", "key", label));
    line.appendChild(el("span", "value", value === undefined || value === null || value === "" ? "-" : value));
    return line;
  }

  function list(items, render, emptyText) {
    if (!items || items.length === 0) return el("p", "muted", emptyText || "none");
    var wrapper = el("div", "list");
    items.forEach(function (item, index) { wrapper.appendChild(render(item, index)); });
    return wrapper;
  }

  var snapshot;
  try {
    snapshot = JSON.parse(source.textContent);
  } catch (error) {
    root.appendChild(section("Snapshot error", [el("pre", "error", error.message)]));
    return;
  }

  var run = snapshot.run || {};
  root.appendChild(el("h1", "", "Run " + (run.id || "unknown")));
  root.appendChild(el("p", "muted", "Generated " + (snapshot.generatedAt || "unknown") + " · " + (snapshot.note || "")));

  root.appendChild(section("Status", [
    row("Status", run.status), row("State", run.state), row("Risk", run.risk),
    row("Provider", run.provider), row("Issue", run.issueKey),
    row("Workflow", run.workflowId + " @ " + (run.workflowVersion || "-")),
    row("Base SHA", run.baseSHA), row("Candidate SHA", run.candidateSHA),
    row("Terminal reason", run.terminalReason)
  ]));

  var trace = snapshot.trace || {};
  root.appendChild(section("Traceability", list(trace.stages, function (stage) {
    var item = el("div", "item");
    item.appendChild(el("span", "badge badge-" + stage.status, stage.status));
    item.appendChild(el("strong", "", stage.title || stage.id));
    if (stage.evidence && stage.evidence.length) {
      item.appendChild(el("small", "", stage.evidence.length + " evidence record(s)"));
    }
    return item;
  }, "No trace stages recorded"), trace.note));

  root.appendChild(section("History", list(snapshot.history, function (transition) {
    var item = el("div", "item");
    item.appendChild(el("span", "seq", transition.sequence));
    item.appendChild(el("span", "", transition.from + " → " + transition.to));
    item.appendChild(el("small", "", (transition.taskId || "-") + " · " + (transition.timestamp || "-")));
    return item;
  }, "No transitions recorded")));

  root.appendChild(section("Approvals", list(trace.approvals, function (approval) {
    var item = el("div", "item");
    item.appendChild(el("strong", "", approval.gate));
    item.appendChild(el("span", "", approval.approver + " · " + (approval.role || "no role")));
    item.appendChild(el("small", "", approval.identitySource + " · " + approval.candidateSHA));
    return item;
  }, "No approvals recorded")));

  var review = snapshot.review || {};
  root.appendChild(section("Review findings", list(review.findings, function (finding) {
    var item = el("div", "item");
    item.appendChild(el("span", "badge badge-hypothesis", "hypothesis"));
    item.appendChild(el("strong", "", finding.id));
    item.appendChild(el("span", "", (finding.severity || "unspecified") + " · " + (finding.ruleId || "no rule")));
    item.appendChild(el("small", "", finding.recordID));
    return item;
  }, "No review findings recorded"), review.note));

  root.appendChild(section("Evidence chain", list(snapshot.evidence, function (record) {
    var item = el("div", "item");
    item.appendChild(el("span", "seq", record.sequence));
    item.appendChild(el("span", "badge", record.kind));
    item.appendChild(el("span", "", record.subject));
    item.appendChild(el("small", "", record.dataDigest + (record.dataOmitted ? " · data omitted" : "")));
    return item;
  }, "No evidence recorded")));
})();
`

const dashboardStylesCSS = `:root {
  color-scheme: dark;
  --bg: #071421;
  --panel: #0e2233;
  --line: #1d3a4f;
  --text: #e8f4f8;
  --muted: #8fb0bd;
  --accent: #49d5c4;
  --warn: #f0b35a;
}

* { box-sizing: border-box; }
body {
  margin: 0;
  background: var(--bg);
  color: var(--text);
  font: 15px/1.55 -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
}
.topbar {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 18px 24px;
  border-bottom: 1px solid var(--line);
}
.brand { color: var(--accent); font-weight: 700; letter-spacing: .04em; }
.tag { color: var(--muted); font-size: 12px; text-transform: uppercase; letter-spacing: .08em; }
main { max-width: 1080px; margin: 0 auto; padding: 24px; }
h1 { font-size: 28px; margin: 8px 0; }
h2 { font-size: 17px; margin: 0 0 12px; color: var(--accent); }
.card {
  background: var(--panel);
  border: 1px solid var(--line);
  border-radius: 12px;
  padding: 18px 20px;
  margin: 16px 0;
}
.muted { color: var(--muted); }
.error { color: var(--warn); white-space: pre-wrap; }
.kv { display: grid; grid-template-columns: 160px 1fr; gap: 12px; padding: 4px 0; border-bottom: 1px dotted var(--line); }
.key { color: var(--muted); }
.value { overflow-wrap: anywhere; }
.list { display: flex; flex-direction: column; gap: 8px; }
.item { display: flex; align-items: baseline; flex-wrap: wrap; gap: 8px; padding: 8px 0; border-bottom: 1px solid var(--line); }
.item small { color: var(--muted); overflow-wrap: anywhere; }
.seq { color: var(--muted); font-variant-numeric: tabular-nums; min-width: 32px; }
.badge { border: 1px solid var(--line); border-radius: 999px; padding: 1px 8px; font-size: 12px; color: var(--muted); }
.badge-approved, .badge-observed, .badge-passed { color: var(--accent); border-color: var(--accent); }
.badge-failed, .badge-missing, .badge-hypothesis { color: var(--warn); border-color: var(--warn); }
@media (max-width: 640px) { .kv { grid-template-columns: 1fr; } main { padding: 16px; } }
`
