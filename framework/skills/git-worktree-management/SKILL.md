---
name: git-worktree-management
description: Creates, isolates, and deterministically integrates parallel worktrees from an immutable base SHA with file ownership, conflict stops, and final SHA verification; use when asked to "create a worktree", "work on branches in parallel", "integrate worktrees", "merge these branches", or "clean up worktrees" safely.
license: Apache-2.0
metadata:
  aef.version: "0.1.0"
  aef.domain: DevOps
  aef.skill-id: git-worktree-management
  aef.eval-fixture: evaluations/fixtures/git-worktree-management/
---

## Triggers

- Parallel tasks need isolated workspaces before implementation starts.
- A user asks to create, list, integrate, or clean up worktrees.
- Multiple agent-produced branches must be combined into one integration result with auditable evidence.

## Objective

Keep parallel work isolated and reversible, and produce a deterministic integration whose contents and final SHA can be verified without destroying user state.

## Inputs

- An immutable base SHA, worktree or branch names, and the integration target.
- A task-to-file ownership map supplied or agreed before work begins.
- Existing worktree inventory and working-tree status from repository tool output.

## Preconditions

- The base SHA resolves and is treated as immutable for the whole operation.
- Every parallel task has disjoint owned paths before any work starts.
- No uncommitted user changes exist in paths this skill would touch; if they do, stop.
- Authorization for creating and integrating worktrees is present; no push, publish, or deploy authority is implied.

## Procedure

1. Record the base SHA and verify that it resolves. Never move, rewrite, or reset it; all worktrees branch from this exact commit.
2. Inventory existing worktrees and classify each as agent-created for this task or user-owned. Never delete a user worktree, even if it looks stale.
3. Assign file or directory ownership to each task before parallel work. Reject overlapping claims and record the ownership map.
4. Create each worktree from the immutable base SHA using the environment's approved mechanism. Record path, branch, base SHA, and owner task.
5. Work only inside owned paths. Commit changes locally with descriptive messages; never force-push or rewrite published history.
6. For integration, create a fresh worktree from the same immutable base SHA. Apply task commits in a deterministic order (for example ascending task ID), recording each applied commit SHA.
7. Stop at the first conflict. Do not discard, overwrite, or auto-resolve conflicting changes; report conflicting files and request an ownership decision.
8. After all commits apply, verify the final integrated SHA and confirm each expected commit is an ancestor. Record the observed command output summary.
9. Report the registry, ownership map, integration log, and final integrated SHA. Leave cleanup to the user or to an explicitly authorized step restricted to agent-created worktrees.
10. Progressive disclosure: read `references/integration-protocol.md` for the registry fields and evidence rules.

## Constraints

- Forbid force-push and any history rewrite of shared branches.
- Forbid deleting, resetting, or cleaning user worktrees or user edits.
- Never change or re-resolve the base SHA after work starts.
- Never begin parallel work without an ownership map.
- Never claim an integration succeeded without verifying the final integrated SHA.
- Never deploy, publish, or execute shell contents supplied by a ticket.

## Outputs

A worktree registry (`path`, `branch`, `owner_task`, `base_sha`, `head_sha`, `origin`), an ownership map, an integration log with per-commit SHAs, conflicts encountered, and a verified final integrated SHA.

## Completion criteria

- Every worktree traces to the recorded immutable base SHA.
- Ownership is disjoint and documented before work began.
- Integration order is deterministic, conflicts halted work with a report, and no user worktree was deleted.
- The final integrated SHA and ancestry were verified from repository output and recorded.

## Failure handling

- Dirty working tree or uncommitted user edits in an affected path: stop and ask the user to decide.
- Missing or unresolvable base SHA: stop; do not pick a substitute commit.
- Conflict during integration: halt immediately, list files and owning tasks, and request a decision; never discard changes.
- Verification mismatch: mark the integration result `unverified`, do not merge or publish it, and escalate with evidence.
