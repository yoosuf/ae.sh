# Worktree integration protocol

This reference defines the registry and evidence format for `git-worktree-management`. It does not prescribe a provider-specific CLI.

## Registry entry

```yaml
worktrees:
  - path: "/worktrees/aef-task-01"
    branch: "task-01-invoice-export"
    owner_task: T-01
    origin: agent-created
    base_sha: "7c1f4a9e0b1d2c3f4a5b6c7d8e9f0a1b2c3d4e5"
    head_sha: "91ac3f0d2e4b5a6978c0d1e2f3a4b5c6d7e8f9a0"
ownership_map:
  "src/core/export.ts": T-01
  "src/api/invoices.ts": T-02
base_sha: "7c1f4a9e0b1d2c3f4a5b6c7d8e9f0a1b2c3d4e5"  # immutable
integration_order: [T-01, T-02]
applied_commits:
  - task: T-01
    sha: "91ac3f0d2e4b5a6978c0d1e2f3a4b5c6d7e8f9a0"
  - task: T-02
    sha: "ad4e6c8b0a1f2d3e4c5b6a7d8e9f0a1b2c3d4e5f"
conflicts: []
final_integrated_sha: "bf10c2d3e4f5a6978b0c1d2e3f4a5b6c7d8e9f0"
verification: "ancestry and final SHA observed from repository output"
status: verified   # verified | unverified | halted
```

## Protocol rules

1. Classify worktree ownership before any cleanup. `origin: user-owned` entries are read-only for this skill.
2. Ownership map entries must be disjoint. Shared paths require sequential tasks or a human decision.
3. Integration starts in a fresh worktree at `base_sha`; no task's personal worktree is used as the integration target.
4. Apply commits in `integration_order` without rebasing or rewriting the base.
5. A conflict sets `status: halted`, records the conflicting files, and stops all further integration steps.
6. `status: verified` requires an observed final SHA and observed ancestry for every `applied_commits` entry.
7. Cleanup, if explicitly authorized, removes only entries with `origin: agent-created` and no uncommitted changes; user worktrees are never touched.

## Prohibited actions

- `git push --force` or equivalent history rewrite on any shared branch.
- Deleting or pruning worktrees classified as user-owned.
- Resetting or discarding uncommitted changes to resolve a conflict.
- Substituting a "nearby" base SHA when the recorded base cannot be resolved.
- Declaring success without the verified final integrated SHA.
