# Optional local dashboard UI

This directory is an optional React + Vite + TypeScript source viewer for an AE
run snapshot. It is not required by any workflow, does not call a service, and
has no write path. The CLI itself generates a self-contained viewer with:

```sh
ae dashboard snapshot --run <run-id> --out DIR
```

That generated `index.html`, `app.js`, `styles.css`, and `snapshot.json` open
directly from disk with no build step. This directory is the authored source
alternative for contributors who want to change the UI.

To use it locally:

1. Generate a snapshot and copy its `snapshot.json` contents into the
   `__AE_SNAPSHOT__` placeholder in `ui/index.html`.
2. Run `npm install` once, then `npm run dev` for local editing.
3. Run `npm test`, `npm run typecheck`, and `npm run build` before submitting UI
   changes.

No AE command depends on npm, this app, or a build artifact. Credentials and
raw untrusted provider payloads must never be added to snapshots or this source.
