# AGENTS.md

## Shape
- Single-package Go module: `github.com/adlandh/echo-zap-middleware/v2`, package `echozapmiddleware`.
- Runtime code is only `middleware.go` and `helpers.go`; exported API is `Middleware`, `MiddlewareWithContextLogger`, `ZapConfig`, and `BodySkipper`.
- This library targets `github.com/labstack/echo/v5`; Echo v5 handlers/skippers use `*echo.Context`, not `echo.Context`.
- README's first basic example still imports Echo v4; trust `go.mod` and source imports over that stale snippet.

## Verification
- CI uses Go 1.25 and runs `go test -race -coverprofile=coverage.txt -covermode=atomic ./...`.
- Lefthook pre-push runs lint and `go test -cover -race ./...` in parallel for `*.go` changes.
- Fast local check: `go test ./...`.
- Focused helper test: `go test -run TestLimitBody ./...`.
- Focused testify suite test: `go test -run 'TestMiddleware/TestBodyLimitApplied' ./...`.
- Benchmarks live in `middleware_benchmark_test.go`; run all with `go test -bench=. -benchmem`, or one with `go test -bench=BenchmarkMiddlewareWithBodyLimit -benchmem`.

## Linting
- `.golangci.yml` is gitignored; CI and lefthook refresh it from `https://raw.githubusercontent.com/adlandh/golangci-lint-config/refs/heads/main/.golangci.yml` before `golangci-lint`.
- To mirror CI locally: `curl -sS https://raw.githubusercontent.com/adlandh/golangci-lint-config/refs/heads/main/.golangci.yml -o .golangci.yml && golangci-lint run`.

## Middleware Gotchas
- `ZapConfig` defaults are filled only for nil `Skipper` and nil `BodySkipper`; changing zero-value behavior can affect callers that pass partial configs.
- `LimitHTTPBody: true` with `LimitSize <= 0` means no body limit; tests cover this explicitly.
- Body dumping must preserve the downstream request body. Limited reads intentionally read `LimitSize+1` bytes and replay them with the original body.
- `BodySkipper` does not prevent capture; it replaces non-empty logged bodies with `[excluded]`.
- Request IDs are read from the request `X-Request-Id` header first, then the response header set by Echo request ID middleware.
- A handler that returns nil without committing a response logs `Response not committed` at warn level.
- `Middleware(nil)` is allowed: `context-logger` wraps the logger, and nil context loggers fall back to `zap.NewNop()`.

## Docs
- Update `README.md` when public API behavior or configuration semantics change.
- Repo-local usage examples for consumers also live in `skills/golang-echo-zap/SKILL.md`; keep it aligned when changing public behavior.
