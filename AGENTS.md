# Repository Notes

## Shape
- This is a single-package Go module: `github.com/adlandh/echo-zap-middleware/v2`, package name `echozapmiddleware`.
- Runtime code is only `middleware.go` and `helpers.go`; exported entrypoints are `Middleware`, `MiddlewareWithContextLogger`, `ZapConfig`, and `BodySkipper`.
- The implementation targets `github.com/labstack/echo/v5`; do not copy older Echo v4 imports from stale examples.

## Verification
- CI test command is `go test -race -coverprofile=coverage.txt -covermode=atomic ./...` on Go 1.25.
- Fast local check: `go test ./...`.
- Focused helper test: `go test -run TestLimitBody ./...`.
- Focused testify suite test: `go test -run 'TestMiddleware/TestBodyLimitApplied' ./...`.
- Benchmarks live in `middleware_benchmark_test.go`; run all with `go test -bench=. -benchmem` or one with `go test -bench=BenchmarkMiddlewareWithBodyLimit -benchmem`.

## Linting
- `.golangci.yml` is intentionally ignored locally; CI downloads it from `https://raw.githubusercontent.com/adlandh/golangci-lint-config/refs/heads/main/.golangci.yml` before running `golangci/golangci-lint-action@v9`.
- To mirror CI locally, refresh the ignored config first, then run `golangci-lint run ./...`.

## Middleware Gotchas
- `ZapConfig` defaults are filled only for nil `Skipper` and nil `BodySkipper`; changing zero-value behavior can affect callers that pass partial configs.
- `LimitHTTPBody: true` with `LimitSize <= 0` means no body limit; tests cover this explicitly.
- Body dumping must preserve the downstream request body. Limited reads intentionally read `LimitSize+1` bytes and replay them with the original body.
- `BodySkipper` does not prevent capture; it replaces non-empty logged bodies with `[excluded]`.
- Request IDs are read from the request `X-Request-Id` header first, then the response header set by Echo request ID middleware.
- A handler that returns nil without committing a response logs `Response not committed` at warn level.

## Docs
- Update `README.md` when public API behavior or configuration semantics change.
