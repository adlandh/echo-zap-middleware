---
name: golang-echo-zap
description: Use when adding or configuring github.com/adlandh/echo-zap-middleware/v2 in a Go Echo service.
---

# golang-echo-zap

Use this skill when adding or configuring `github.com/adlandh/echo-zap-middleware/v2` in a Go Echo service.

## Use When
- The user wants Zap request logging in an Echo app.
- Code imports `github.com/adlandh/echo-zap-middleware/v2` or needs this middleware configured.
- The task mentions `ZapConfig`, `Middleware`, `MiddlewareWithContextLogger`, `BodySkipper`, request/response body logging, or Echo request IDs.

## Key Facts
- This middleware targets `github.com/labstack/echo/v5`; do not mix it with Echo v4 imports.
- Echo v5 handlers and skippers use `*echo.Context`, not `echo.Context`.
- Import path is `github.com/adlandh/echo-zap-middleware/v2`; package name is `echozapmiddleware` unless aliased.
- `Middleware(logger, config...)` is the normal entrypoint for a `*zap.Logger`.
- `MiddlewareWithContextLogger(ctxLogger, config...)` is for `github.com/adlandh/context-logger` extractors.
- `ZapConfig` fills nil function fields from defaults. Enabling body dumping with zero-valued limit settings applies the default 500-byte limit.
- Set `LimitSize` to a negative value to explicitly disable body limiting.
- `BodySkipper` runs after the handler; skipped bodies are captured up to the configured limit but logged as `[excluded]`.
- The `uri` field contains the escaped path without query parameters.
- Request IDs are logged from request `X-Request-Id` first, then from Echo's response header.

## Minimal Setup
```go
import (
    echozapmiddleware "github.com/adlandh/echo-zap-middleware/v2"
    "github.com/labstack/echo/v5"
    "github.com/labstack/echo/v5/middleware"
    "go.uber.org/zap"
)

logger, err := zap.NewProduction()
if err != nil {
    return err
}

e := echo.New()
e.Use(middleware.RequestID())
e.Use(echozapmiddleware.Middleware(logger))
```

## Body And Header Logging
```go
e.Use(echozapmiddleware.Middleware(logger, echozapmiddleware.ZapConfig{
    AreHeadersDump: true,
    IsBodyDump:     true,
    LimitHTTPBody:  true,
    LimitSize:      500,
    Skipper: func(c *echo.Context) bool {
        return c.Path() == "/health"
    },
    BodySkipper: func(c *echo.Context) (skipReqBody, skipRespBody bool) {
        if c.Path() == "/upload" {
            return true, false
        }
        if c.Request().Header.Get("Content-Encoding") == "gzip" {
            return true, true
        }
        return false, false
    },
}))
```

## Context Logger Setup
```go
import contextlogger "github.com/adlandh/context-logger"

ctxLogger := contextlogger.WithContext(logger)
e.Use(echozapmiddleware.MiddlewareWithContextLogger(ctxLogger))
```

## Implementation Guidance
- Add `middleware.RequestID()` before this middleware when generated request IDs should appear in logs.
- Keep body dumping off by default for hot paths; enable `IsBodyDump` only when the service can afford body capture.
- Use `BodySkipper` for sensitive or binary payloads, keep the default capture limit, and add Echo's body-limit middleware before this middleware when request size itself must be bounded.
- If changing middleware behavior in this repo, preserve downstream request body replay after capture and update README configuration docs.

## Verify
- In this repo, run `go test ./...` for a fast check.
- For body limit changes, run `go test -run TestLimitBody ./...` and `go test -run 'TestMiddleware/TestBodyLimitApplied' ./...`.
- CI uses `go test -race -coverprofile=coverage.txt -covermode=atomic ./...`.
