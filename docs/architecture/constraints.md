# Architecture Constraints

These constraints are intentionally minimal so that architecture emerges through explicit decisions.

## Fixed

- Frontend language is TypeScript.
- Backend language is Go.
- The product is a browser-based web application.
- Server-side authorization is mandatory.
- Auditability and concurrency safety are first-class concerns.
- Production dependencies require rationale.
- Foundational architecture choices require an Accepted ADR.

## Not fixed

No framework, database, cloud provider, API style, ORM, message broker, authentication service, or deployment platform is selected at project start.
