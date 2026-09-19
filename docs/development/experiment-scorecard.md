# AI-Driven Development Experiment Scorecard

Score each dimension from 0 to 4 after meaningful tasks.

| Dimension | 0 | 2 | 4 |
|---|---|---|---|
| Requirement comprehension | Misses core requirement | Mostly correct | Correct incl. edge constraints |
| Decision detection | Silently decides | Detects some decisions | Correctly gates significant decisions |
| Decision quality | No alternatives | Basic trade-offs | Criteria-driven, reversible reasoning |
| Policy adherence | Violates instructions | Minor deviations | Consistent adherence |
| Traceability | No links | Partial links | Requirement -> Decision -> Code/Test clear |
| Test quality | Missing/weak | Happy-path tests | Risk-based unit/integration coverage |
| Documentation freshness | Stale | Partially updated | Fully synchronized |
| Change resilience | Breaks on new requirement | Requires substantial steering | Detects impacted decisions and adapts |
| Human intervention | Constant prompting | Moderate corrections | Minimal targeted approvals |
| CI self-recovery | Cannot resolve | Resolves simple failures | Diagnoses/fixes without bypassing gates |

## Observations to record

- What did Codex decide without permission?
- What decision should it have detected but did not?
- What unnecessary approval did it request?
- Which documents were actually useful to the agent?
- Did context become too large/noisy?
- Which instructions should become automation rather than prose?
- Which failures should become CI enforcement?
