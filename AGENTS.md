# Agent Guidance

SpaceScale should stay simple and direct. Prefer the smallest correct change that preserves clear subsystem boundaries.

## Do Not Add

- No needless abstractions, interfaces, wrappers, adapters, or package-level function hooks.
- No defensive validation inside a package when an upstream boundary already owns it.
- No backward-compatibility code unless persisted data, shipped behavior, external consumers, or an explicit requirement need it.
- No test-only seams in production code just to make mocking easier.
- No extra local IDs or duplicate ownership concepts when one source of truth exists.
- No broad helper layers for one call site.
- No noisy logs that dump large runtime files when paths are enough.
- No new dependencies without explicit approval. Promoting an indirect dependency to direct use also requires approval.

## Boundary Rules

- Startup validates and reconciles runtime assets.
- Startup/preflight owns host identity setup. If it resolves a value like the Firecracker jailer UID/GID, pass that value down instead of re-looking it up or adding duplicate downstream error paths.
- Placement validates shape and capacity.
- Executor owns launch command validation, reservation commit, and duplicate launch prevention.
- `microvm` owns only local Firecracker lifecycle, workspace files, vsock listeners, CID allocation, and cleanup.
- `daemon.go` orchestrates startup only; it should not know subsystem internals.

## Style

- Hand-written Go files should begin with `// Copyright (c) 2026 SpaceScale Systems Inc. All rights reserved.`. Exclude generated Go files.
- Keep code readable over clever.
- Keep comments rare and useful; explain lifecycle or boundary decisions, not obvious assignments.
- Prefer direct package calls when there is no real alternate implementation.
- Remove stale guards and comments when ownership moves to another boundary.
- When changing code, update nearby docs and comments in the same pass so implementation and explanation stay in sync.
- After any code, config, runtime asset, or workflow change, check related docs for stale decisions, assumptions, examples, and commands; update docs in the same pass when the change makes them inaccurate.
- Always verify the relevant package or workflow after edits; do not leave changes at "looks right" without running checks when checks are available.

## Logging

- Success logs should be concise lifecycle checkpoints, not step-by-step narration. Prefer one summary line per startup phase or subsystem boundary.
- Put the owning subsystem in `component`; avoid `component=scaled subsystem=...` when a more specific component exists.
- Keep detailed paths and noisy runtime output in diagnostic files. Log those paths on failure instead of dumping large file contents or SDK chatter.
- Keep normal Firecracker SDK request/response logs out of stdout. Preserve jailer, Firecracker, and scoutd file logs for debugging.
- Failure logs should include the actionable error and diagnostic paths needed to continue investigation.

## File Organization

- File names should describe the boundary or concept they own.
- If related code has one owner and no real subsystem boundary, keep it together in one file instead of splitting small pieces by taxonomy.
- When merging files, do not just dump code together. Rearrange the merged file into a readable order: constants and types first, main lifecycle flow next, low-level helpers last.
- If a file merge changes package responsibilities, update package docs and nearby comments in the same pass.
- Each package should have one top-level `// Package <name> ...` comment in the file that best represents the package boundary, with enough explanation to make ownership clear.
- Package comments should be specific: describe what the package owns, the lifecycle it participates in, and the upstream or downstream boundaries it relies on.
- Package comments may include concrete file-boundary paragraphs when useful, but do not add standalone generic file inventory comments like "This file owns ...".
