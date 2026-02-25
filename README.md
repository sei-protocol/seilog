# seilog

[![Go Reference](https://pkg.go.dev/badge/github.com/sei-protocol/seilog.svg)](https://pkg.go.dev/github.com/sei-protocol/seilog)

Structured logging with per-logger runtime level control, built on [`log/slog`](https://pkg.go.dev/log/slog).

seilog was created to provide a uniform logging configuration experience across artifacts produced
by [Sei Labs](https://www.sei.io/). Nothing in the library is Sei-specific — it is a general-purpose `slog` extension that
any Go project can use.

## Why seilog?

The standard library's `slog` gives you structured logging and pluggable handlers, but it lacks two things that matter
in production:

1. **Per-logger level control.** `slog` has a single global level. When you're debugging a database issue in production,
   you want `DEBUG` on your `db` package without drowning in noise from every other subsystem.

2. **Runtime level changes.** Restarting a process to flip a log level is slow and disruptive. seilog lets you change
   levels on the fly — via code, an admin endpoint, or a signal handler — with immediate effect across all goroutines.

seilog adds both while staying out of your way: `NewLogger` returns a plain `*slog.Logger`. There is no wrapper type, no
custom interface, and no lock-in. Your code uses the standard `slog` API everywhere.

## Features

- **Hierarchical logger names** — `"myapp/db/pool"` mirrors your package structure and enables targeted level changes.
- **Runtime level control** — change levels per logger, by glob pattern, or recursively across an entire subtree without
  restarting.
- **Zero-alloc hot path** — the enabled-level check is a single atomic load; disabled log calls cost ~5 ns.
- **Standard `*slog.Logger` return type** — no wrapper, no lock-in, full compatibility with the `slog` ecosystem.
- **Strict naming validation** — enforced at creation time to prevent typos, injection, and naming inconsistencies
  across a large codebase.
- **Environment-variable configuration** — format, output destination, and default level are set once at startup with no
  code changes.
- **Concurrent safety** — all functions are safe for concurrent use. Level changes are atomic and visible immediately.

## Quick start

```go
package main

import (
	"log/slog"
	"github.com/sei-protocol/seilog"
)

// Create loggers at package level — they're cheap and reusable.
var log = seilog.NewLogger("myapp", "db")

func main() {
	defer seilog.Close()

	log.Info("connected", "host", "localhost", "port", 5432)

	// Turn on debug for the db subtree at runtime.
	seilog.SetLevel("myapp/db/**", slog.LevelDebug)

	log.Debug("query plan", "sql", "SELECT ...")
}
```

## Logger naming

Logger names form a `/`-separated hierarchy. Use the variadic form of `NewLogger` to build them — each segment is
validated individually:

```go
seilog.NewLogger("myapp") // "myapp"
seilog.NewLogger("myapp", "db") // "myapp/db"
seilog.NewLogger("myapp", "db", "pool") // "myapp/db/pool"
```

Each segment must match `[a-z0-9]+(-[a-z0-9]+)*` (lowercase alphanumerics and hyphens). This is enforced via panic at
creation time. The constraint is intentional:

| Reason          | Detail                                                                                                                          |
|-----------------|---------------------------------------------------------------------------------------------------------------------------------|
| **Consistency** | Prevents `"MyApp"` vs `"myapp"` from silently creating separate loggers in a large codebase.                                    |
| **Glob safety** | Segments cannot contain `*`, `?`, or `[`, so a bare name is always an exact match in `SetLevel` — never accidentally a pattern. |
| **Log hygiene** | No whitespace, newlines, or special characters means log output stays parseable and injection-free.                             |

## Setting levels

Levels can be changed at any time without restarting the process:

```go
// Exact match
seilog.SetLevel("myapp/db", slog.LevelDebug)

// Glob — direct children only (path.Match semantics, * doesn't cross /)
seilog.SetLevel("myapp/*", slog.LevelDebug)

// Glob — grandchildren only
seilog.SetLevel("myapp/*/*", slog.LevelWarn)

// Recursive — myapp and ALL descendants at any depth
seilog.SetLevel("myapp/**", slog.LevelDebug)

// Everything
seilog.SetLevel("*", slog.LevelWarn)
```

`SetLevel` returns the number of loggers matched, which helps catch typos:

```go
if n := seilog.SetLevel("myap/db", slog.LevelDebug); n == 0 {
fmt.Println("typo? no loggers matched")
}
```

## Querying levels

```go
lvl, ok := seilog.GetLevel("myapp/db")
if ok {
fmt.Printf("myapp/db is at %s\n", lvl)
}

// List all registered loggers.
for _, name := range seilog.ListLoggers() {
lvl, _ := seilog.GetLevel(name)
fmt.Printf("  %-30s %s\n", name, lvl)
}
```

## Environment variables

Output format, destination, and default level are configured at startup via environment variables. These are read once
during package init and cannot be changed afterward.

| Variable             | Values                                       | Default  |
|----------------------|----------------------------------------------|----------|
| `SEI_LOG_LEVEL`      | `debug`, `info`, `warn`, `error`             | `info`   |
| `SEI_LOG_FORMAT`     | `json`, `text`                               | `json`   |
| `SEI_LOG_OUTPUT`     | `stdout`, `stderr`, or an absolute file path | `stdout` |
| `SEI_LOG_ADD_SOURCE` | `true`, `false`                              | `false`  |

When `SEI_LOG_OUTPUT` is a file path:

- The path must be absolute and must not contain `..` components.
- Files are opened with mode `0600` and `O_APPEND` for atomic POSIX writes.
- seilog does not perform log rotation — use an external tool like `logrotate`.
- Call `seilog.Close()` during graceful shutdown to flush and close the file descriptor.

## API

```go
func NewLogger(name string, subs ...string) *slog.Logger
func SetLevel(name string, level slog.Level) int
func GetLevel(name string) (slog.Level, bool)
func SetDefaultLevel(level slog.Level, updateExisting bool)
func ListLoggers() []string
func Close() error
```

Full documentation: [pkg.go.dev/github.com/sei-protocol/seilog](https://pkg.go.dev/github.com/sei-protocol/seilog)

## Performance

seilog's goal is to add per-logger level control with negligible overhead compared to using `slog` directly. Benchmarks
on Apple M2 Max (arm64), comparing seilog against stdlib `slog` with the same `JSONHandler` writing to `io.Discard`:

| Benchmark                 | seilog    | stdlib slog | Overhead | Allocs |
|---------------------------|-----------|-------------|----------|--------|
| Info (3 attrs, JSON)      | 582 ns/op | 563 ns/op   | +3%      | 0      |
| Disabled level            | 5.0 ns/op | 5.9 ns/op   | **−15%** | 0      |
| Typed attrs (`LogAttrs`)  | 619 ns/op | 607 ns/op   | +2%      | 0      |
| Pre-bound attrs (`.With`) | 451 ns/op | 441 ns/op   | +2%      | 0      |
| Parallel (12 goroutines)  | 230 ns/op | 225 ns/op   | +2%      | 0      |
| Text handler              | 646 ns/op | 634 ns/op   | +2%      | 0      |

Key takeaways:

- **Zero allocations** on every hot path. seilog adds no allocations beyond what `slog` itself performs.
- **Disabled-level calls are ~5 ns** — a single atomic load short-circuits before touching the handler. This is 15%
  faster than stdlib because seilog's `LevelVar` check avoids the handler dispatch entirely.
- **Enabled-level overhead is 2–3%** across all scenarios (Info, typed attrs, pre-bound attrs, text handler, parallel).
  This is within benchmark noise — seilog is effectively free at log time.
- **Contention under concurrent `SetLevel` mutations** adds modest overhead (~325 ns/op) with no lock contention
  surprises — the `RWMutex` + atomic `LevelVar` design holds up cleanly.

```
go test -bench=. -benchmem ./...
```

## Design choices

| Choice                                    | Rationale                                                                                                                                                                                                                                           |
|-------------------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Returns `*slog.Logger`, not a custom type | No lock-in. Callers use the standard API; seilog can be swapped out without changing application code.                                                                                                                                              |
| Panics on invalid names                   | Invalid logger names are programmer errors (wrong arguments at call sites). Panicking catches them immediately during development rather than masking bugs at runtime. A default or error return would silently produce untracked loggers.          |
| Env-var configuration, not programmatic   | Keeps the API surface minimal. Output format and destination rarely change in code — they're deployment concerns. This avoids a builder/options API and keeps `NewLogger` to a single clean call.                                                   |
| Handler captured at creation time         | Avoids per-log-call overhead of reconstructing the handler chain. The tradeoff is that existing loggers won't see handler reconfiguration (format/output swaps), but runtime level changes work because they mutate a shared `LevelVar` atomically. |
| Strict naming regex                       | Prevents a class of bugs (inconsistent casing, accidental glob injection, whitespace in log output) that are painful to debug in production across a multi-team codebase.                                                                           |
| `/**` recursive match                     | `path.Match` has no recursive glob. Without `/**`, there's no way to target an entire subtree — you'd need one `SetLevel` call per depth level, which is impractical.                                                                               |

## License

[Apache License v2](LICENSE.md).
