> This file guides agents working in the API app.

- We prioritize maintainability over quick one-off fixes.
- Use `make` at repo root for build/test/dev workflows.
- Local development is primarily in GoLand; IDE handles vet/lint/fmt routines.
- Test style is chosen by behavior under test (table-driven, subtests, or focused tests).

#### To Agents

> Required working rules:

- Keep file headers accurate, concise, and aligned with file responsibility.
- Write comments for **why**, invariants, and security/operational intent.
- Avoid line-by-line narration of obvious code.
- Add function comments for exported functions and non-obvious private logic.
- For `const`/`var`, prefer inline trailing comments only when value intent is not obvious.
- Avoid duplicated comments that restate identifiers.
- Service package test policy:
  - Keep pure helper/business-rule tests white-box.
  - Do not run DB-backed integration tests in the service package.
  - Do not use DB mocks/fakes for persistence behavior in service tests.
  - For service methods that touch persistence, cover behavior via HTTP integration tests against a real DB.
- HTTP package defaults to black-box tests.
- DB-backed integration tests belong in the HTTP package.
- For HTTP auth/project integration tests, seed users through `POST /v0/internal/auth-sync` instead of direct DB inserts when possible.
