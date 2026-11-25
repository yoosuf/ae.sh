## Summary

<!-- What changed and why? Link the issue. -->

## Phase / status

<!-- Which phase does this belong to? If it implements a roadmap item, say so. Do not claim Phase 3-7 features exist. -->

## Checks actually run

<!-- List exact commands you ran and their outcome. Do not claim tests passed unless you ran them. -->

- [ ] `go test ./...` — outcome:
- [ ] `go vet ./...` — outcome:
- [ ] `go test -race ./...` — outcome:
- [ ] Skill/schema validation — outcome:
- [ ] Not run (explain why):

## Documentation and evidence

- [ ] Docs updated for behavior changes.
- [ ] `CHANGELOG.md` updated under `Unreleased`.
- [ ] Skill manifests/checksums regenerated if skills changed.
- [ ] No credentials, tokens, or private issue/customer data in diff or output.

## Risks and review notes

<!-- Call out security-sensitive changes, new dependencies, and anything reviewers should verify. -->

## Checklist

- [ ] I reviewed my own diff.
- [ ] Changes stay local-first and provider-neutral.
- [ ] No push, publish, or deploy side effects were performed.
