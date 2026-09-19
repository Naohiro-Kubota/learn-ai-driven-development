# Go Development Knowledge

Use this knowledge when modifying Go code.

## Purpose

Produce modern, readable, testable Go while preserving project decisions.

## Guidance

- Prefer standard-library capabilities when they are sufficient.
- Keep packages cohesive and boundaries explicit.
- Pass `context.Context` through operations that may block, perform I/O, or have request lifetime semantics.
- Return errors with useful context while preserving error identity when callers need it.
- Avoid framework-specific coupling in core business rules unless an Accepted ADR explicitly chooses that trade-off.
- Make concurrency ownership and cancellation explicit.
- Favor table-driven tests when they improve coverage and readability.
- Run the repository's formatter, static analysis, and tests before completion.

## External reference

The project may evaluate JetBrains' `go-modern-guidelines` as a knowledge source. If using external guidance, verify the relevant guidance is current and does not conflict with Accepted project decisions.
