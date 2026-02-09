> This file defines how agents should work inside `apps/api`.
> Keep behavior changes maintainable, testable, and easy to onboard.

## Scope

- This guidance applies to files under `apps/api/**`.
- Do not edit root governance files (`/AGENTS.md`) or the primary repo README.

## Engineering Principles

- Prioritize maintainability and clarity over quick one-off fixes.
- Keep transport layers thin; place business rules in service/domain code.
- Prefer explicit errors and straightforward control flow over clever shortcuts.
- Match existing project conventions unless a change clearly improves maintainability.

## Testing Policy

- Use black-box testing by default.
- Do not write tests that directly target private/internal helper details.
- Use table-driven tests where they improve coverage and readability.
- Use subtests where case isolation or output clarity benefits from it.

## Tooling And Workflow

- API runtime baseline: Go `1.25+`.
- Local infra, compose, and migration flows are managed from the repo root `Makefile`.
- Common repo-root commands: `make compose-up`, `make compose-down`, `make migrate-up`, `make migrate-up-test`, `make test`.
- Common API-local commands: `go run ./cmd/api`, `go test ./...`.

## Comments And Documentation Rules

- Every edited code file must keep an accurate file header comment describing the file's responsibilities.
- Every function should have a clear, onboarding-friendly comment that explains purpose and behavior.
- Add inline comments for complex logic or state transitions where intent is not obvious.
- Keep comments in sync with code whenever behavior changes.

## Schema And Generated Code

- Database migrations live in `apps/db/migrations`.
- SQLC-generated query types live in `apps/api/internal/postgres/gen`.
- When changing SQL or query contracts, regenerate related generated artifacts and verify build/tests.

## Branching And Commits

- Use branch names in the form `feature/<name>` or `fix/<name>`.
- Keep commits focused, small, and descriptive, with one concern per commit when practical.

## Definition Of Done

Before finishing API changes, ensure all of the following are true:

1. `go test ./...` passes from `apps/api` (or run `make test` from repo root when DB-backed behavior is changed).
2. File/function comments remain accurate and understandable for junior onboarding.
3. No unrelated files are modified by accident.
