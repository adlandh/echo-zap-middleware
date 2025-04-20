// Package echozapmiddleware is a logger middleware for echo framework
package echozapmiddleware

import (
	"time"

	contextlogger "github.com/adlandh/context-logger"
	"github.com/adlandh/response-dumper"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// BodySkipper is a function type that determines whether to exclude request and/or response bodies from logging.
// It receives the Echo context and returns two boolean values:
//   - skipReqBody: When true, the request body will be marked as "[excluded]" in logs
//   - skipRespBody: When true, the response body will be marked as "[excluded]" in logs
//
// This is useful for excluding sensitive data or large binary content from logs.
type BodySkipper func(c echo.Context) (skipReqBody, skipRespBody bool)

// defaultBodySkipper is the default implementation of BodySkipper that doesn't exclude any bodies.
// It always returns false for both skipReqBody and skipRespBody, meaning all bodies will be logged.
func defaultBodySkipper(_ echo.Context) (skipReqBody, skipRespBody bool) {
	return false, false
}

// ZapConfig defines the configuration options for the Zap Logger middleware.
// It allows customizing which requests to log, what parts of requests/responses to include,
// and how to handle request/response bodies.
type ZapConfig struct {
	// Skipper defines a function to skip middleware execution for certain requests.
	// If the function returns true, the middleware will not log the request.
	Skipper middleware.Skipper

	// BodySkipper defines a function to exclude specific request/response bodies from logging.
	// It returns two booleans: skipReqBody and skipRespBody.
	// If skipReqBody is true, the request body will be marked as "[excluded]" in logs.
	// If skipRespBody is true, the response body will be marked as "[excluded]" in logs.
	BodySkipper BodySkipper

	// AreHeadersDump controls whether request and response headers are included in logs.
	// When true, all headers will be logged as structured fields.
	AreHeadersDump bool

	// IsBodyDump controls whether request and response bodies are included in logs.
	// When true, bodies will be captured and logged as structured fields.
	IsBodyDump bool

	// LimitHTTPBody controls whether to limit the size of logged HTTP bodies.
	// When true, bodies larger than LimitSize will be truncated.
	LimitHTTPBody bool

	// LimitSize specifies the maximum size (in bytes) for logged HTTP bodies.
	// Bodies larger than this will be truncated with "..." appended.
	// Only used when LimitHTTPBody is true.
	LimitSize int
}

var (
	// DefaultZapConfig is the default Zap Logger middleware config.
	DefaultZapConfig = ZapConfig{
		Skipper:        middleware.DefaultSkipper,
		BodySkipper:    defaultBodySkipper,
		AreHeadersDump: false,
		IsBodyDump:     false,
		LimitHTTPBody:  true,
		LimitSize:      500,
	}
)

// createLogFields creates the standard log fields for a request/response.
func createLogFields(c echo.Context, start time.Time) []zapcore.Field {
	req := c.Request()
	res := c.Response()

	return []zapcore.Field{
		zap.Int("status", res.Status),
		zap.String("latency", time.Since(start).String()),
		zap.String("request_id", getRequestID(c)),
		zap.String("method", req.Method),
		zap.String("uri", req.RequestURI),
		zap.String("host", req.Host),
		zap.String("remote_ip", c.RealIP()),
	}
}

// makeHandler creates the middleware handler function.
func makeHandler(ctxLogger *contextlogger.ContextLogger, config ZapConfig) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Skip logging if configured to do so or if request/response is nil
			if config.Skipper(c) || c.Request() == nil || c.Response() == nil {
				return next(c)
			}

			start := time.Now()
			req := c.Request()
			ctx := req.Context()

			var respDumper *response.Dumper

			var reqBody []byte

			// Set up body dumping if enabled
			if config.IsBodyDump {
				defer func() {
					c.SetRequest(req.WithContext(ctx))
				}()

				respDumper, reqBody = prepareReqAndResp(c, config)
			}

			// Process the request
			err := next(c)
			if err != nil {
				c.Error(err)
			}

			// Create log fields
			fields := createLogFields(c, start)

			// Add headers if configured
			fields = append(fields, addHeaders(config, req.Header, c.Response().Header())...)

			// Add request/response body if configured
			fields = append(fields, addBody(config, c, string(reqBody), respDumper)...)

			// Log with appropriate level based on status code
			logit(c.Response().Status, ctxLogger.Ctx(ctx), fields)

			return nil
		}
	}
}

// MiddlewareWithContextLogger returns a Zap Logger middleware with context logger.
// It allows for more advanced logging with context-aware information.
//
// Parameters:
//   - ctxLogger: A context logger that can extract values from the context
//   - config: Optional configuration for the middleware. If not provided, DefaultZapConfig is used
//
// Returns:
//   - An Echo middleware function that logs requests and responses
func MiddlewareWithContextLogger(ctxLogger *contextlogger.ContextLogger, config ...ZapConfig) echo.MiddlewareFunc {
	// Use default config if none provided
	if len(config) == 0 {
		config = []ZapConfig{DefaultZapConfig}
	}

	// Ensure Skipper is set
	if config[0].Skipper == nil {
		config[0].Skipper = middleware.DefaultSkipper
	}

	// Ensure BodySkipper is set
	if config[0].BodySkipper == nil {
		config[0].BodySkipper = defaultBodySkipper
	}

	return makeHandler(ctxLogger, config[0])
}

// Middleware returns a Zap Logger middleware with the provided configuration.
// This is the main entry point for using this middleware in an Echo application.
//
// Parameters:
//   - logger: A Zap logger instance
//   - config: Optional configuration for the middleware. If not provided, DefaultZapConfig is used
//
// Returns:
//   - An Echo middleware function that logs requests and responses
//
// Example:
//
//	app.Use(echozapmiddleware.Middleware(
//	    logger,
//	    echozapmiddleware.ZapConfig{
//	        AreHeadersDump: true,
//	        IsBodyDump: true,
//	    }))
func Middleware(logger *zap.Logger, config ...ZapConfig) echo.MiddlewareFunc {
	return MiddlewareWithContextLogger(contextlogger.WithContext(logger), config...)
}
