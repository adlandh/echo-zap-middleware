# Echo Zap Middleware

[![Go Reference](https://pkg.go.dev/badge/github.com/adlandh/echo-zap-middleware.svg)](https://pkg.go.dev/github.com/adlandh/echo-zap-middleware)
[![Go Report Card](https://goreportcard.com/badge/github.com/adlandh/echo-zap-middleware)](https://goreportcard.com/report/github.com/adlandh/echo-zap-middleware)
[![Go Version](https://img.shields.io/github/go-mod/go-version/adlandh/echo-zap-middleware)](https://github.com/adlandh/echo-zap-middleware)

A powerful and configurable middleware for the [Echo](https://github.com/labstack/echo) web framework that integrates with [Zap](https://github.com/uber-go/zap) logger. This middleware provides detailed request/response logging with customizable options for headers and body capture, request ID tracking, and intelligent log level selection based on HTTP status codes.

## Features

- Structured logging with Zap
- Request/response header logging
- Request/response body capture and logging
- Configurable body size limits to prevent large log entries
- Intelligent log level selection based on HTTP status codes
- Request ID tracking
- Ability to skip specific requests or parts of requests/responses
- Context-aware logging with support for custom context values

## Installation

```shell
go get github.com/adlandh/echo-zap-middleware
```

## Basic Usage

```go
package main

import (
	"net/http"

	echo_zap_middleware "github.com/adlandh/echo-zap-middleware"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

func main() {
	// Create zap logger
	logger, err := zap.NewDevelopment()
	if err != nil {
		panic(err)
	}

	// Create your Echo app
	app := echo.New()

	// Add middleware with default configuration
	app.Use(echo_zap_middleware.Middleware(logger))

	// Add some endpoints
	app.GET("/ping", func(c echo.Context) error {
		return c.String(http.StatusOK, "Hello, World!")
	})

	// Run the server
	app.Logger.Fatal(app.Start(":3000"))
}
```

## Advanced Configuration

The middleware provides several configuration options through the `ZapConfig` struct:

```go
app.Use(echo_zap_middleware.Middleware(
	logger,
	echo_zap_middleware.ZapConfig{
		// Include request and response headers in logs
		AreHeadersDump: true,

		// Include request and response bodies in logs
		IsBodyDump: true,

		// Limit the size of logged bodies
		LimitHTTPBody: true,
		LimitSize: 500, // Maximum size in bytes

		// Skip logging for specific requests
		Skipper: func(c echo.Context) bool {
			return c.Path() == "/health" // Don't log health check requests
		},

		// Skip logging specific parts of requests/responses
		BodySkipper: func(c echo.Context) (skipReqBody, skipRespBody bool) {
			// Skip request bodies for /upload endpoint
			if c.Path() == "/upload" {
				return true, false
			}

			// Skip both request and response bodies for gzipped content
			if c.Request().Header.Get("Content-Encoding") == "gzip" {
				return true, true
			}

			return false, false
		},
	}))
```

### Configuration Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `Skipper` | `middleware.Skipper` | `middleware.DefaultSkipper` | Function to skip middleware execution for certain requests |
| `BodySkipper` | `BodySkipper` | `defaultBodySkipper` | Function to exclude specific request/response bodies from logging |
| `AreHeadersDump` | `bool` | `false` | Controls whether request and response headers are included in logs |
| `IsBodyDump` | `bool` | `false` | Controls whether request and response bodies are included in logs |
| `LimitHTTPBody` | `bool` | `true` | Controls whether to limit the size of logged HTTP bodies |
| `LimitSize` | `int` | `500` | Maximum size (in bytes) for logged HTTP bodies |

## Context-Aware Logging

For more advanced logging with context values, you can use the `MiddlewareWithContextLogger` function:

```go
package main

import (
	"net/http"

	contextlogger "github.com/adlandh/context-logger"
	echo_zap_middleware "github.com/adlandh/echo-zap-middleware"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"go.uber.org/zap"
)

func main() {
	logger, _ := zap.NewDevelopment()

	// Create a context logger with custom value extractors
	ctxLogger := contextlogger.WithContext(logger)

	app := echo.New()

	// Add request ID middleware
	app.Use(middleware.RequestID())

	// Add context-aware logger middleware
	app.Use(echo_zap_middleware.MiddlewareWithContextLogger(ctxLogger))

	app.GET("/ping", func(c echo.Context) error {
		return c.String(http.StatusOK, "Hello, World!")
	})

	app.Logger.Fatal(app.Start(":3000"))
}
```

## Example with Body and Header Logging

Here's a complete example showing how to configure the middleware with body and header logging:

```go
package main

import (
	"net/http"

	echo_zap_middleware "github.com/adlandh/echo-zap-middleware"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

func main() {
	// Create zap logger
	logger, err := zap.NewDevelopment()
	if err != nil {
		panic(err)
	}

	// Then create your app
	app := echo.New()

	// Add middleware
	app.Use(echo_zap_middleware.Middleware(
		logger,
		echo_zap_middleware.ZapConfig{
			// if you would like to save your request or response headers as tags, set AreHeadersDump to true
			AreHeadersDump: true,
			// if you would like to save your request or response body as tags, set IsBodyDump to true
			IsBodyDump: true,
			// No dump for /pong
			// No dump for gzip
			BodySkipper: func(c echo.Context) (bool, bool) {
				if c.Request().URL.Path == "/pong" { 
					return true, true 
				}
				if c.Request().Header.Get("Content-Encoding") == "gzip" {
					return true, true
				}
				return false, false
			},
		}))

	// Add some endpoints
	app.GET("/ping", func(c echo.Context) error {
		return c.String(http.StatusOK, "Hello, World, from Ping!")
	})

	app.GET("/pong", func(c echo.Context) error {
		return c.String(http.StatusOK, "Hello, World, from Pong!")
	})

	// And run it
	app.Logger.Fatal(app.Start(":3000"))
}
```

## Log Levels

The middleware automatically selects the appropriate log level based on the HTTP status code:

- **INFO** for 2xx (Success)
- **INFO** for 3xx (Redirection)
- **WARN** for 4xx (Client Error)
- **ERROR** for 5xx (Server Error)
