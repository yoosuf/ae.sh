# Security Policy

## Supported versions

AE is pre-1.0 and under active development. Security fixes are targeted at the
latest main branch and the most recent published pre-release, if any. No
long-term support branches are currently promised.

## Known vulnerabilities

**No real vulnerabilities are known or published at this time.** This is a
statement about what has been reported to maintainers, not a claim that an
audit or penetration test has been performed. If you find a problem, please
report it privately.

## Reporting a vulnerability (private, responsible disclosure)

Please **do not** open a public issue for a security problem.

1. Use GitHub's private vulnerability reporting on this repository if it is
   enabled (**Security → Report a vulnerability**), or contact the maintainers
   privately through the contact method published in the repository's security
   advisory / profile. Do not include a public issue comment with exploit
   details.
2. Include: affected version or commit, reproduction steps, impact, and any
   suggested mitigation. Redact tokens, customer data, and internal hostnames.
3. You should receive an acknowledgement within 7 days when a private reporting
   channel is available. Maintainers will investigate, agree on a disclosure
   timeline, and coordinate a fix and advisory before public disclosure.
4. Please give maintainers a reasonable opportunity to fix the issue before
   broader disclosure. We will credit reporters who wish to be named.

If you cannot find a private channel, open a public issue with **no technical
details** asking for a private contact method.

## Scope and out-of-scope notes

- In scope: the local CLI, skill catalogue and installer, local state storage,
  policy/validation logic, and any official adapters or release tooling.
- The project is local-first: untrusted issue text, repository content, tool
  output, and MCP responses must be treated as data. Prompt injection and
  tenant crossover are part of the threat model in
  `docs/security/threat-model.md`.
- Out of scope for early phases: any hosted service, cloud account, dashboard,
  or third-party provider bug. No such components are implemented in Phase 0–2.

## Safe-handling reminders

- Never place API keys or tokens in CLI flags, committed config, or test
  fixtures. Use environment variables or an external secret store.
- Avoid running the CLI on untrusted repositories with elevated credentials.
- Verify skill checksums and third-party actions before use; see
  `docs/provider-compatibility.md` and `.github/workflows/ci.yml`.
