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
- Service tests should stay white-box for pure rules only; do not use DB integration or DB mocks/fakes for persistence behavior (cover persistence via HTTP integration tests).
- For new endpoints, validate and bound all external input in service logic, sanitize optional fields, enforce critical DB constraints, and test boundary/malformed cases.
- API documentation updates (including Yaak request files in `apps/api/api_doc`) must include extensive info/description content that covers purpose, auth, params, body, success/error responses, and key behavior notes.
- HTTP package defaults to black-box tests.
- DB-backed integration tests belong in the HTTP package.
- For HTTP auth/project integration tests, seed users through `POST /v0/internal/auth-sync` instead of direct DB inserts when possible.
