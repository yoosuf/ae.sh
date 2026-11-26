import type { Snapshot } from "./types";

export function readSnapshot(): Snapshot {
  const source = document.getElementById("ae-snapshot");
  if (!source) throw new Error("snapshot script element is missing");
  const parsed: unknown = JSON.parse(source.textContent ?? "");
  if (!parsed || typeof parsed !== "object") throw new Error("snapshot is not an object");
  const snapshot = parsed as Partial<Snapshot>;
  if (snapshot.schemaVersion !== "1.0" || !snapshot.run?.id || !Array.isArray(snapshot.trace?.stages)) {
    throw new Error("snapshot has an unsupported shape");
  }
  return snapshot as Snapshot;
}
