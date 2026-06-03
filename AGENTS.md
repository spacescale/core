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

## Product Direction

- Core design must keep SpaceScale Machines in view even while Ignite is the current product surface.
- Treat Ignite and Machines as two surfaces over the same compute foundation, not two unrelated architectures.
- Ignite should hide VM/microVM lifecycle behind a workload contract; Machines may later expose persistent machine lifecycle directly.
- Do not bake Ignite-only assumptions into placement, executor, microVM, networking, auth, logs, or lifecycle code when the same primitive should support Machines later.
- Do not assume every runtime instance is disposable, stateless, HTTP-only, container-only, or unable to receive shell/exec access.
- Do not design networking only around routed workloads; keep public exposure, private/internal paths, and future machine port exposure in view.
- Do not design identity only around workload API tokens; core should remain compatible with one SpaceScale login authorizing CLI, API, shell, exec, files, and logs.
- Keep machine-facing primitives internally clean: create, shell/exec access, upload, expose, stop/start/destroy, logs, identity, persistence, and lifecycle.
- Keep persistent machine identity separate from Ignite workload identity, but let both rely on shared placement, runtime, networking, and observability primitives.
- Any new compute lifecycle decision must be explicit about whether it is Ignite-only or shared foundation behavior.
- Agent, browser, sandbox, and dev-environment capabilities should be modeled as future Machine capabilities, not separate core product primitives.
- Do not expose Machines publicly from core until product work explicitly asks for it.
- Use the private product strategy as context for the future Machines surface: https://github.com/spacescale/ideas/blob/main/iaas-posture.md

## Style

- Hand-written Go files should begin with `// Copyright (c) 2026 SpaceScale Systems Inc. All rights reserved.`. Exclude generated Go files.
- Keep code readable over clever.
- Write Go like a systems language: measured, explicit, allocation-aware, and close to runtime behavior.
- Do not write framework-heavy Go. Keep control flow visible, direct, and boring.
- Keep comments rare and useful; explain lifecycle or boundary decisions, not obvious assignments.
- Prefer direct package calls when there is no real alternate implementation.
- Prefer `errors.AsType[T](err)` for concrete error type checks instead of `errors.As(err, &target)`.
- Do not use reflection in runtime-critical code. Prefer typed structs, small interfaces, and generated code where useful.
- Remove stale guards and comments when ownership moves to another boundary.
- When changing code, update nearby docs and comments in the same pass so implementation and explanation stay in sync.
- After any code, config, runtime asset, or workflow change, check related docs for stale decisions, assumptions, examples, and commands; update docs in the same pass when the change makes them inaccurate.
- Always verify the relevant package or workflow after edits; do not leave changes at "looks right" without running checks when checks are available.

## Error and Protocol Shape

- Use sentinel errors only for generic conditions where the caller needs no extra data to recover. Check them with `errors.Is`.
- Use typed errors when callers need structured fields to retry, route, account, or react programmatically. Include fields such as tenant ID, resource ID, operation, retryability, or capacity details when those values affect behavior.
- Do not make callers parse error strings. Error text is for humans; error shape is for code.
- Keep protocol contracts small, explicit, and versioned when data crosses package, process, network, guest, or persistence boundaries.
- Prefer one clear owner for protocol evolution. Do not let unrelated packages invent parallel meanings for the same state, ID, or lifecycle field.

## Go Generics

- Use Go generics only when the exact same logic is repeated for several concrete types and a generic helper makes the code clearer.
- Do not use generics to hide control flow, avoid naming concrete types, or create framework-like abstractions.
- If the benefit is not obvious at the call site, write boring Go.

## Systems Go

- Keep owned data and borrowed views mentally separate. Make ownership, mutation, and lifetime obvious from the code shape.
- Prefer explicit state transitions over inferred behavior from booleans, nil values, or side effects.
- Keep shared mutation narrow, named, and owned by one type or package.
- Treat cleanup paths as part of the main design, not as deferred cleanup after the happy path is written.
- For SpaceScale, the best Go is explicit, boring, operational, and robust under failure.

## Performance

- Do not optimize blindly. Measure first with benchmarks, `pprof`, traces, and allocation profiles.
- Do not guess where the bottleneck is. Profile CPU, memory, allocations, goroutines, locks, syscalls, and network latency.
- Do not allocate casually in hot paths. Preallocate slices, maps, buffers, and structs when the shape is known.
- Do not create garbage in tight loops. Reuse buffers and objects carefully when ownership is clear.
- Do not use `fmt.Sprintf` in performance-sensitive paths. Prefer `strconv`, `strings.Builder`, byte buffers, or direct append patterns.
- Do not use `sync.Pool` by default. Use it only for measured high-churn allocations like buffers, protobuf scratch objects, request envelopes, or temporary objects.
- - **No:** Do not treat `core` like a low-level dataplane repo.
- **Instead:** Treat `core` as the orchestration brain of SpaceScale: Go-first, explicit, observable, allocation-aware, and operationally boring.

## Control Plane

- Keep NATS, protobuf, scheduling, routing, auth, and API hot paths allocation-conscious.
- Do not copy large payloads unnecessarily. Pass references, stream data, or reuse buffers where safe.

## Concurrency

- Do not create unbounded goroutines. Use bounded workers, contexts, timeouts, and backpressure.
- Do not create unbounded channels. Size queues intentionally and define overflow behavior.
- Do not ignore context cancellation. Thread `context.Context` through request, NATS, DB, runtime, and shutdown paths.
- Do not hold locks while doing I/O. Lock only around shared memory mutation.
- Do not use one giant mutex for unrelated state. Split locks by ownership boundary and keep critical sections small.
- Do not make shared state implicit. Own state inside clear structs with clear lifecycle rules.

## Lifecycle Correctness

- Do not leave lifecycle states informal. Model states explicitly: created, reserved, launching, active, draining, stopped, failed.
- Do not allow capacity accounting to drift. Make reserve, commit, release, revert, and expiry explicit and tested.
- Do not make cleanup a best-effort afterthought. Treat cleanup, rollback, release, and shutdown as first-class correctness paths.
- Do not hide errors or collapse them into vague messages. Wrap errors with operational context.
- Do not make production behavior depend on local assumptions. Validate config, runtime assets, host capabilities, permissions, and network state at startup.

## Observability and Testing

- Do not use logs as the only observability layer. Add metrics, counters, timings, health signals, and structured events where they support operations.
- Do not trust happy-path behavior. Test failure paths, retries, duplicate messages, stale boot IDs, timeouts, and partial cleanup.
- Do not optimize for beautiful code over debuggable code. Write code that can be understood during an outage.

> "Let all things be done decently and in order." — 1 Corinthians 14:40

## Logging

- Success logs should be concise lifecycle checkpoints, not step-by-step narration. Prefer one summary line per startup phase or subsystem boundary.
- Put the owning subsystem in `component`; avoid `component=scaled subsystem=...` when a more specific component exists.
- Keep detailed paths and noisy runtime output in diagnostic files. Log those paths on failure instead of dumping large file contents or SDK chatter.
- Keep normal Firecracker SDK request/response logs out of stdout. Preserve jailer, Firecracker, and guestd file logs for debugging.
- Failure logs should include the actionable error and diagnostic paths needed to continue investigation.

## File Organization

- File names should describe the boundary or concept they own.
- If related code has one owner and no real subsystem boundary, keep it together in one file instead of splitting small pieces by taxonomy.
- When merging files, do not just dump code together. Rearrange the merged file into a readable order: constants and types first, main lifecycle flow next, low-level helpers last.
- If a file merge changes package responsibilities, update package docs and nearby comments in the same pass.
- Each package should have one top-level `// Package <name> ...` comment in the file that best represents the package boundary, with enough explanation to make ownership clear.
- Package comments should be specific: describe what the package owns, the lifecycle it participates in, and the upstream or downstream boundaries it relies on.
- Package comments may include concrete file-boundary paragraphs when useful, but do not add standalone generic file inventory comments like "This file owns ...".
