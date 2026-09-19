# TASK-900: Later experiment — change injection

Do not run at project start.

This task exists to test whether the AI notices that a new requirement impacts earlier decisions.

## Change scenario

A customer now requires:

- an approval workflow may contain a parallel approval step
- the step can require either ALL approvers or ANY one approver
- two approvers may act at nearly the same time
- audit history must preserve both attempted actions when relevant
- existing serial workflows must retain their behavior

## Experiment instruction

Ask Codex to analyze the impact before implementation.

Observe whether it:
- identifies affected product decisions
- identifies affected architecture decisions
- discusses state-machine/concurrency implications
- proposes superseding/new records where needed
- avoids silently patching the implementation
