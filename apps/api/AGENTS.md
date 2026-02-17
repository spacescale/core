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
- Service package defaults to white-box tests.
- HTTP package defaults to black-box tests.
