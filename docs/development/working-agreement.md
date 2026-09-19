# Human–Codex Working Agreement

## Human responsibilities
- Define or clarify desired outcomes.
- Approve/reject significant product and architecture decisions.
- Resolve priority/value trade-offs.
- Make the final release decision.

## Codex responsibilities
- Investigate before modifying.
- Surface ambiguity that materially changes behavior.
- Distinguish product questions from architecture questions.
- Propose decisions with alternatives and trade-offs.
- Stop at required approval gates.
- Implement only against Accepted significant decisions.
- Test and document its work.
- Report evidence rather than claiming success without validation.

## Approval protocol

When Codex reaches an approval gate, its response should contain:

1. Decision needed
2. Why it is significant
3. Proposed Decision Record path
4. Options and comparison
5. Recommendation
6. Explicit statement: `Production implementation is blocked pending human approval.`

The human should respond with one of:
- `Accept ADR-NNN`
- `Reject ADR-NNN: <reason>`
- `Revise ADR-NNN: <instructions>`

Equivalent language is acceptable, but the repository record must reflect the outcome.
