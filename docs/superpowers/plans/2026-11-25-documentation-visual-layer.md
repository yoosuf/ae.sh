# Documentation Visual Layer Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a cohesive, descriptive visual layer to the nine visitor-facing documentation pages selected in the approved design.

**Architecture:** Store repository-local visuals under `assets/docs/` with descriptive stable filenames. Use generated raster illustrations for page-level anchors and preserve existing Mermaid diagrams where they already explain relationships precisely. Each Markdown integration includes descriptive alt text and keeps the surrounding prose authoritative.

**Tech Stack:** PNG assets generated with the image-generation skill; Markdown; existing Mermaid diagrams; shell/link validation.

**Spec:** `docs/superpowers/specs/2026-11-25-documentation-visual-layer-design.md`

## Global Constraints

- Use the existing `ae.sh` midnight-navy, cyan, teal, and amber palette.
- Prefer explanatory visuals over decorative stock imagery.
- Every image gets descriptive alt text and nearby text explanation.
- Use lowercase `ae.sh` branding consistently; never introduce the old `AEF` title in new artwork.
- Keep documentation local/offline with no external image hosting or tracking.
- Preserve unrelated worktree changes.

## Review Focus

- Broken relative image paths: every referenced asset must exist from the Markdown file's directory.
- Branding drift: generated artwork must use `ae.sh`, not the old `AEF` title.
- Accessibility: alt text must describe the visual's purpose, not merely repeat its filename.
- Excessive payload: assets should be limited to the selected pages and reasonable PNG sizes.
- Documentation truthfulness: visuals must not imply live integrations, deployment, or production readiness that the text marks as planned.

### Task 1: Create the documentation visual asset set

**Files:**
- Create: `assets/docs/installation-local-first.png`
- Create: `assets/docs/getting-started-lifecycle.png`
- Create: `assets/docs/architecture-boundaries.png`
- Create: `assets/docs/integrations-boundaries.png`
- Create: `assets/docs/evidence-chain.png`
- Create: `assets/docs/security-fail-closed.png`
- Create: `assets/docs/roadmap-phases.png`
- Create: `assets/docs/profiles-toolchains.png`

**Interfaces:**
- Consumes: the approved design spec and existing documentation terminology.
- Produces: eight repository-local visuals with no obsolete `AEF` branding or external dependencies.

- [ ] Generate one visual per listed concept using the image-generation skill, keeping the palette and composition consistent.
- [ ] Inspect each generated image for exact visible text, branding, legibility, and accidental claims.
- [ ] Copy the accepted images into `assets/docs/` with the stable filenames above.
- [ ] Check dimensions and file sizes; reject unreadable or unnecessarily oversized outputs.

### Task 2: Integrate visuals into visitor-facing documentation

**Files:**
- Modify: `docs/installation.md`
- Modify: `docs/getting-started.md`
- Modify: `docs/architecture.md`
- Modify: `docs/integrations.md`
- Modify: `docs/evidence.md`
- Modify: `docs/security/threat-model.md`
- Modify: `docs/roadmap.md`
- Modify: `docs/profiles.md`

**Interfaces:**
- Consumes: the eight stable assets from Task 1.
- Produces: Markdown pages with one relevant visual anchor, descriptive alt text, and nearby explanatory context.

- [ ] Insert each image near the page introduction or the section it explains using repository-relative paths.
- [ ] Add concise alt text describing the concept and the main relationship shown.
- [ ] Add or retain a text sentence that communicates the same essential information for readers who do not load images.
- [ ] Keep the existing README cover unchanged except for its already-established `ae.sh` asset reference.

### Task 3: Validate the documentation visual layer

**Files:**
- Modify: `CHANGELOG.md`

**Interfaces:**
- Consumes: all Markdown references and assets from Tasks 1–2.
- Produces: verified local documentation with no broken visual references.

- [ ] Run a repository-local script or shell check that extracts image paths from the selected Markdown files and confirms every target exists.
- [ ] Run `git diff --check` and inspect the changed Markdown for accidental claims or stale `AEF` branding.
- [ ] Record the visual-layer addition under `Unreleased` in `CHANGELOG.md`.
- [ ] Report asset count, dimensions, verification commands, and any remaining GitHub-only manual step.
