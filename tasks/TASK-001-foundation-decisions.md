# TASK-001: Propose foundational architecture decisions

## Goal

Prepare the minimum set of architecture decisions required before the first vertical slice can be implemented.

## Instructions to Codex

Read:
- `AGENTS.md`
- `docs/product/vision.md`
- `docs/product/requirements.md`
- `docs/architecture/constraints.md`
- `docs/decisions/README.md`
- relevant files under `skills/`

Do not implement production application code in this task.

Identify which foundational decisions are truly required for a first vertical slice consisting of:

1. a user opens the web UI
2. creates a draft request
3. submits it
4. an authorized approver sees the pending request
5. approves it
6. the requester sees the approved state
7. the action is represented in audit history

Avoid deciding infrastructure that the first slice does not require.

For each required significant decision:
- create a Proposed ADR
- compare 2-4 viable options
- define decision drivers
- recommend one option
- include consequences and revisit conditions

At minimum, assess whether decisions are needed for:
- frontend application framework
- backend HTTP/API approach
- frontend/backend contract style
- persistence
- database access
- migrations
- authentication/authorization for the first slice
- testing strategy

Do not mark any ADR Accepted.
End with a compact list of human approvals needed before implementation.
