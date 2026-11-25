# ADR-0009: Optional AWS adapters and dashboard

## Status

Accepted (Phase 0 architecture decision). Both are **planned / not implemented**; nothing in Phase 0–2 creates cloud resources.

## Context

Some teams will want webhook ingress, managed orchestration, remote execution, and a visual view of runs. Others require fully offline operation with no cloud account. Making these mandatory would violate portability and local-first goals.

## Decision

Treat AWS and the dashboard as optional layers behind ports:

- AWS adapters (planned Phase 6): API Gateway webhook, SQS, Step Functions Standard, ECS Fargate, DynamoDB, S3, Secrets Manager, CloudWatch/OTel, plus Terraform reference architecture. They consume/replicate local state and evidence; they never become the authoritative source.
- Dashboard (planned Phase 7): optional React/Vite/TypeScript read-only view over local state/evidence. No workflow depends on it.
- Core packages, CLI offline mode, and the fake provider must work identically with both absent.

## Consequences

- No account, credentials, or network is needed to use AE.
- Adapter work can proceed without destabilizing core.
- Two projections of state (local + cloud) need consistency docs when implemented.
- Contributors must not imply these components exist in current docs or CLI output.

## Alternatives considered

- **AWS-native architecture as default:** rejected — vendor lock-in and local-first conflict.
- **Built-in mandatory web UI:** rejected — unnecessary for CLI workflows.
- **No cloud support ever:** rejected — real teams need deployment hooks later.
