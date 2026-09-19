# API Design Knowledge

Use when proposing or changing a frontend/backend contract.

- Start from product use cases and domain language.
- Make authorization and error semantics explicit.
- Define concurrency behavior for state-changing operations.
- Avoid leaking internal persistence representation into public contracts without rationale.
- Consider idempotency and retry behavior.
- Treat compatibility/versioning as a deliberate decision.
- If choosing REST, RPC, GraphQL, generated clients, schema-first tooling, or another project-wide contract approach, create an ADR before implementation.
