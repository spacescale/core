# RULES TO FOLLOW

## Boundary and Validation

- Validate and normalize data exactly once at the true boundary.
- Boundaries are: API input, env/config, file input, from outside the process, and user-controlled strings.
- Do not repeat `strings.TrimSpace`, empty checks, nil checks, or format checks on trusted internal values.
- Do not add defensive error returns for impossible states in internal runtime paths.
- Do not add boilerplate checks just for ceremony or "safety" if the invariant is already guaranteed by earlier code.
- Constructors and internal helpers should not panic or re-validate dependencies when wiring is centralized and
  controlled.
- you should only log an error once, at the highest possible level (the "boundary")
- Use "Fail Fast" returns. It keeps the main logic at the zero-indentation level.
- Only return errors for operations that can actually fail in a meaningful way , database I/O, FS, Net, OS, external
  calls, marshal unmarshal
- Constructors and internal helpers should not panic or re-validate dependencies when wiring is centralized and
  controlled
- avoid wrapper layers that only re-check already-validated data
- sub packages have their own orchestrators to make central orchestrators clean the sub orchestrators can log user the logger
- If a helper is used by both a boundary path and an internal runtime path, keep validation in the boundary-facing path

## Concurrency and LifeCycles

- never launch a go func() without a way deterministic to stop it. Always wire goroutines to a context cancellation or
  an error.Group.
- When using sync.Mutex or sync.RWMutex, lock only the memory mutation. Never perform I/O (Database, NATS, HTTP) while
  holding a lock

## Distributed NATS Mindset

- Assume NATS will deliver a message twice, or the network will partition. Database writes and edge daemon actions must
  be
  safe to run multiple times without corrupting state.
- Never make a network call (nats.Request, database query, external API) with context.Background()
  unless it is a long-lived watcher. Always apply a timeout context so the system fails fast under load.
- The Control Plane must hold zero cluster state in memory. All placement, capacity, and scheduling decisions must rely
  on real-time NATS auctions or JetStream KV lookups. Do not build centralized "ledgers" of available resources

## Databases and SQLC

- Do not execute multiple sequential SELECT queries to stitch data together in Go. Use SQL JOINs to enforce ownership
  hierarchies and relationships in a single network round-trip.
- No N+1 Queries in Loops: Never place an INSERT, UPDATE, or SELECT inside a for loop. If inserting multiple rows (like
  environment variables), you must use sqlc bulk operations
- strict Transaction Teardown: Immediately after tx, err := pool.Begin(ctx), you must defer tx.Rollback(ctx). Postgres
  permanently poisons transactions on the first error; do not attempt to recover a failed transaction.


## Architecture
- optimize for architectural readability and maintainability over speed
- optimize for the right ay and right design over compromises
- split domains by concerns and don't create spaghetti code
- avoid interfaces unless we  come to a  point where we cant avoid but to use it  Only define an interface if a service has multiple concrete implementations. e.g a providers
- use single responsibility principles for creating files so codebase is easier to reason about