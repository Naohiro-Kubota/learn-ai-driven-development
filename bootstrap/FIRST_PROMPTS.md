# Suggested first Codex prompts

## Prompt 1 — repository comprehension

Read the repository instructions and product documents. Do not write production code.

Summarize:
1. the product goal
2. fixed constraints
3. what is intentionally undecided
4. the human/Codex responsibility boundary
5. situations in which you must stop for human approval

Also identify any contradiction or ambiguity in the repository instructions.

## Prompt 2 — foundational decisions

Execute `tasks/TASK-001-foundation-decisions.md`.

Do not implement production application code.
Create Proposed ADRs only and stop for human approval.

## Prompt 3 — after human approval

After I explicitly Accept the required ADRs, execute `tasks/TASK-002-first-vertical-slice.md`.

Before implementation, list the Accepted decisions you are relying on and identify any remaining approval gate.
