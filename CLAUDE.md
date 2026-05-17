# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**echo-zap-middleware v2**: A single-package Go middleware library for the Echo web framework (v5) that integrates structured logging with Zap. The library provides request/response logging with configurable options for headers, bodies, body size limits, and intelligent log level selection based on HTTP status codes.

- **Module**: `github.com/adlandh/echo-zap-middleware/v2`
- **Package**: `echozapmiddleware`
- **Go Version**: 1.25+

See [AGENTS.md](AGENTS.md) for detailed agent context, verification steps, and implementation gotchas.

## Quick Start

```bash
# Run all tests
go test ./...

# Run with coverage and race detection
go test -cover -race ./...

# Run specific test
go test -run TestMiddleware ./...

# Run all benchmarks
go test -bench=. -benchmem
```

For linting, see [AGENTS.md § Linting](AGENTS.md#linting).

## Code Structure

- **Runtime**: `middleware.go` (entry points) and `helpers.go` (internal helpers)
- **Tests**: `middleware_test.go` (testify suite) and `helpers_test.go`
- **Benchmarks**: `middleware_benchmark_test.go`

## Public API

- `Middleware(logger *zap.Logger, config ...ZapConfig)` — main entry point
- `MiddlewareWithContextLogger(ctxLogger *contextlogger.ContextLogger, config ...ZapConfig)` — context-aware logging
- `ZapConfig` — configuration struct (`Skipper`, `BodySkipper`, `RedactHeaders`, `AreHeadersDump`, `IsBodyDump`, `LimitHTTPBody`, `LimitSize`)
- `DefaultZapConfig`, `DefaultRedactHeaders` — exported defaults
- `BodySkipper` — function type for excluding bodies from logs

The library targets Echo v5, which uses `*echo.Context` (pointer, not value type).

## What to Know Before Editing

Consult [AGENTS.md § Middleware Gotchas](AGENTS.md#middleware-gotchas) for:
- How `ZapConfig` defaults are applied (only to nil fields; an explicit empty `RedactHeaders` slice opts out of redaction)
- Body capture and replay semantics
- Request ID resolution order
- Response commitment detection
- Nil logger fallback behavior

## Linting locally

`.golangci.yml` is gitignored; lefthook and CI fetch it fresh from
`https://raw.githubusercontent.com/adlandh/golangci-lint-config/refs/heads/main/.golangci.yml`.
To mirror CI:

```bash
curl -sS https://raw.githubusercontent.com/adlandh/golangci-lint-config/refs/heads/main/.golangci.yml -o .golangci.yml
golangci-lint run
```

`.lefthook.yml` wires a pre-push hook that runs lint and `go test -cover -race ./...` in parallel on `*.go` changes — expect this to fire on `git push`.

## CI gates

- **DeepSource** (blocking commit status): Go analyzer. When it fails, the status page lists issue shortcodes (e.g. `GO-W6007`, `SCC-SA1008`); look them up at `https://deepsource.com/directory/go/issues/<code>`.
- **Codecov** (`codecov/patch`, `codecov/project`): patch and project coverage thresholds.
- **SonarCloud**: quality gate.
- GitHub Actions workflows live in `.github/workflows/` (`test.yml`, `lint.yml`).

## Related docs

- **README.md** — user-facing API, examples, configuration reference; update when public behavior changes.
- **AGENTS.md** — verification commands, middleware gotchas, full linting setup.
- **skills/golang-echo-zap/SKILL.md** — consumer examples; keep aligned with public API changes.
