# Project Instructions for Codex

## 1. Mission

This repository is an experiment in AI-driven software development.

Your goal is not merely to produce working code. You must preserve decision traceability, respect human approval gates, keep documentation synchronized, and provide evidence that changes satisfy the requested behavior.

## 2. Fixed constraints

- Frontend implementation language: TypeScript
- Backend implementation language: Go
- Do not assume a frontend framework, backend HTTP framework, database, ORM/query library, migration tool, state-management library, authentication provider, message broker, or test framework unless an Accepted decision explicitly selects it.
- Prefer the smallest set of dependencies that satisfies Accepted requirements and decisions.

## 3. Authority order

When instructions conflict, use this order:

1. Explicit human instruction in the current task
2. Accepted Product Decision Records
3. Accepted Architecture Decision Records
4. Product requirements and architecture constraints
5. This AGENTS.md
6. Project knowledge / skills
7. Existing code conventions

If you discover a conflict between higher and lower authority sources, do not silently resolve it. Report the conflict and propose the appropriate document change.

## 4. Human approval gate

Before production implementation, determine whether the requested work requires a new significant decision.

A significant decision includes a choice that is costly to reverse, affects multiple modules, establishes a reusable project convention, changes security/data consistency/operability characteristics, or introduces a foundational dependency.

Examples:
- frontend framework
- backend web/API framework
- API interaction style or contract strategy
- persistent datastore
- database access strategy
- authentication / authorization architecture
- asynchronous processing architecture
- migration strategy
- cross-cutting observability architecture

When a significant decision is required:

1. Investigate the context and constraints.
2. Propose 2-4 viable options.
3. Compare trade-offs against explicit criteria.
4. Create or update a Proposed ADR or Product Decision Record.
5. State the recommended option and why.
6. STOP before production implementation.
7. Ask for human approval of the Decision Record.

You may perform non-production spikes only when explicitly requested. A spike must not be silently promoted into production code.

## 5. Product decisions vs architecture decisions

Create a Product Decision Record when the primary question is "What behavior or business rule should the product have?"

Create an ADR when the primary question is "How should the system be structured or what technical approach should it adopt?"

If both are involved, separate them and link the records.

## 6. Decision status rules

Valid statuses:

- Proposed
- Accepted
- Rejected
- Superseded

Only Accepted decisions are normative.

Do not modify an Accepted decision to rewrite history. If a decision changes materially, create a new record and mark the old one Superseded with a link.

## 7. Task workflow

For every non-trivial task:

1. Read the relevant requirements and Accepted decisions.
2. Summarize the constraints you found.
3. Identify whether a new Decision Record is required.
4. If approval is required, create the proposal and stop.
5. If no approval is required, implement the smallest coherent change.
6. Add or update tests.
7. Run available formatters, linters, type checks, and tests.
8. Update affected documentation.
9. Report:
   - files changed
   - decisions relied upon
   - validation commands and results
   - unresolved risks / assumptions
   - follow-up decisions, if any

## 8. Dependency policy

Do not add a new production dependency merely because it is familiar or popular.

For each new dependency:
- identify the requirement it satisfies
- check whether the standard library or an existing dependency is sufficient
- consider maintenance, ecosystem maturity, security, testability, operational cost, and lock-in
- use an ADR when the dependency is foundational or establishes project-wide architecture
- keep narrowly scoped/reversible dependencies lightweight; document the rationale in the PR/task summary when no ADR is needed

## 9. Implementation principles

- Keep domain/business rules isolated from infrastructure-specific code where practical.
- Make state transitions explicit and testable.
- Treat authorization as a server-side responsibility.
- Treat auditability and concurrency behavior as first-class requirements.
- Prefer explicit contracts over hidden framework behavior.
- Do not weaken tests or quality gates merely to make CI pass.
- Do not delete failing tests unless the corresponding requirement or decision changed and that change is approved.

## 10. Documentation synchronization

When behavior changes, check:
- `docs/product/requirements.md`
- relevant Product Decision Records
- relevant ADRs
- API/architecture documentation once created

When architecture changes, check:
- architecture overview
- relevant ADRs
- development instructions
- CI/tooling documentation

## 11. Project knowledge

When touching Go, read `skills/go-development/SKILL.md`.

When proposing API design, read `skills/api-design/SKILL.md`.

When evaluating a dependency, read `skills/dependency-selection/SKILL.md`.

These files are project-local knowledge. Their guidance is subordinate to Accepted decisions.

## 12. Definition of done

A task is not done until:
- requested behavior is implemented
- appropriate tests exist and pass
- relevant quality checks pass
- no unapproved significant architectural/product decision was smuggled into implementation
- docs are synchronized
- decision traceability is stated in the completion report

See `docs/development/definition-of-done.md`.
