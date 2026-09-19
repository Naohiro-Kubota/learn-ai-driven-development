# Product Requirements

Status: Initial baseline

These requirements define intended capabilities. Details marked as undecided must be resolved through Product Decision Records or ADRs before implementation when they affect significant behavior or architecture.

## Functional requirements

### FR-001 Organization membership
The system shall represent organizations and their members.

### FR-002 Roles and authorization
The system shall support at least administrators, requesters, and approvers.
Detailed permission rules are not yet decided.

### FR-003 Request lifecycle
A member shall be able to create a draft request and submit it for approval.

### FR-004 Approval workflow
A submitted request shall proceed through one or more approval steps.
Whether parallel approval, quorum approval, delegation, or conditional routing is supported is initially undecided.

### FR-005 Approval actions
An authorized approver shall be able to approve or reject a pending request.
Comment behavior and mandatory rejection reasons are initially undecided.

### FR-006 Cancellation
A requester shall be able to cancel a request in some lifecycle states.
Exact cancellation rules require a Product Decision Record.

### FR-007 Audit history
Important changes to requests and approval actions shall be auditable.
Audit records must allow the system to explain who performed an action and when.

### FR-008 Notifications
Relevant users shall be notified when actions require their attention or a request reaches a meaningful state.
Delivery channels and asynchronous architecture are undecided.

### FR-009 Search and filtering
Users shall be able to view and filter requests relevant to them.

### FR-010 Workflow administration
An administrator shall be able to define or manage approval workflow configuration.
The exact configuration model is undecided.

### FR-011 Concurrency safety
The system shall behave deterministically when multiple actors attempt conflicting actions on the same request.

### FR-012 Web user interface
The primary user experience shall be a browser-based frontend written in TypeScript.

### FR-013 Backend
Backend application code shall be written in Go.

## Non-functional requirements

### NFR-001 Security
Authentication and authorization must be designed explicitly. Authorization must not rely only on frontend enforcement.

### NFR-002 Traceability
Significant product and architecture choices must be traceable to Decision Records.

### NFR-003 Testability
Business rules and state transitions must be testable without requiring a full end-to-end environment for every test.

### NFR-004 Maintainability
The implementation should favor clear boundaries and a dependency set small enough to understand and maintain.

### NFR-005 Observability
The system must provide enough operational signals to diagnose failed requests and background processing once those mechanisms exist.

### NFR-006 Local development
A new developer should eventually be able to run the application and required services locally using documented commands.

## Explicitly undecided at project start

The following are intentionally not selected yet:

- frontend framework
- frontend routing approach
- frontend server-state / client-state strategy
- frontend component library
- backend HTTP/router framework
- API style and contract strategy
- persistent datastore
- database access strategy
- migration tool
- authentication mechanism/provider
- authorization model details
- asynchronous processing mechanism
- notification channel(s)
- deployment platform
- observability stack
- test frameworks and test pyramid details

Selecting these without the appropriate Decision Record defeats the experiment.
