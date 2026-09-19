# Decision Records

## Why

Specifications describe the current intended behavior.
Decision Records preserve why important choices were made, what alternatives were considered, and when a choice should be revisited.

## Types

### ADR — Architecture Decision Record
Use for significant technical structure, platform, dependency, or cross-cutting engineering choices.

Location: `docs/decisions/architecture/`

### PDR — Product Decision Record
Use for significant product behavior, policy, workflow, or business-rule choices.

Location: `docs/decisions/product/`

## Status lifecycle

`Proposed -> Accepted | Rejected`

An Accepted decision that is later replaced becomes `Superseded` and links to the replacing record.

## Approval rule

Codex may author or update Proposed records.

Only a human may change a record to Accepted or Rejected unless the human explicitly delegates that specific decision.

## What deserves a record?

Prefer a record when at least one is true:

- reversal would be expensive
- multiple credible alternatives exist
- the choice affects multiple modules/teams
- it establishes a reusable convention
- it materially changes security, data consistency, operability, or user behavior
- future maintainers are likely to ask "why is it this way?"

Do not create records for trivial local implementation details.
