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

## Boundary Rules

- Startup validates and reconciles runtime assets.
- Placement validates shape and capacity.
- Executor owns launch command validation, reservation commit, and duplicate launch prevention.
- `microvm` owns only local Firecracker lifecycle, workspace files, vsock listeners, CID allocation, and cleanup.
- `daemon.go` orchestrates startup only; it should not know subsystem internals.

## Style

- Keep code readable over clever.
- Keep comments rare and useful; explain lifecycle or boundary decisions, not obvious assignments.
- Prefer direct package calls when there is no real alternate implementation.
- Remove stale guards and comments when ownership moves to another boundary.
