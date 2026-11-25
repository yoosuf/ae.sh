# Documentation visual layer

## Intent

Make the project understandable to first-time visitors by adding a small,
cohesive set of explanatory visuals to the documentation pages that describe
the product, setup, workflow, safety model, and supported project profiles.

## Goals

- Improve first-glance comprehension for GitHub visitors.
- Give each high-traffic documentation page one visual anchor tied to its
  subject.
- Preserve fast loading, accessible text, and local/offline documentation.
- Keep assets maintainable with stable names and one source image per concept.

## Scope

Add visuals to:

- `README.md`: project overview and end-to-end workflow cover.
- `docs/installation.md`: local-first installation/setup illustration.
- `docs/getting-started.md`: first-run lifecycle diagram.
- `docs/architecture.md`: component boundary diagram.
- `docs/integrations.md`: provider and external-service boundary diagram.
- `docs/evidence.md`: evidence-chain and gate illustration.
- `docs/security/threat-model.md`: trust-boundary and fail-closed illustration.
- `docs/roadmap.md`: phase progression visual.
- `docs/profiles.md`: profile/toolchain selection visual.

ADR and purely reference-oriented pages remain text-first unless an existing
diagram materially improves a decision record.

## Visual system

- Use the existing `ae.sh` midnight-navy, cyan, teal, and amber palette.
- Prefer diagrams and explanatory infographics over decorative stock imagery.
- Use generated raster assets only where a diagram would be visually richer;
  use Mermaid or simple SVG for precise relationships and accessibility.
- Every image gets descriptive alt text and a nearby text explanation.
- Use lowercase `ae.sh` branding consistently; never introduce the old `AEF`
  title in new artwork.

## Non-goals

- No changes to runtime behavior, CLI contracts, or provider integrations.
- No image on every Markdown file.
- No external image hosting, tracking, or cloud dependency.
- No claim that visuals prove live integrations or production readiness.

## Approach

The selected approach is a curated visual layer for the nine visitor-facing
pages above. Assets will live under `assets/docs/`, use descriptive stable
filenames, and be referenced with repository-relative Markdown paths. The
implementation will add visuals in small batches, validate links and rendered
dimensions, and preserve unrelated worktree changes.
