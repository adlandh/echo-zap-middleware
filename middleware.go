// Package echozapmiddleware is a logger middleware for echo framework
package echozapmiddleware

import (
	"time"

	contextlogger "github.com/adlandh/context-logger"
	"github.com/adlandh/response-dumper"
	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// BodySkipper is a function type that determines whether to exclude request and/or response bodies from logging.
// It receives the Echo context and returns two boolean values:
//   - skipReqBody: When true, the request body will be marked as "[excluded]" in logs
//   - skipRespBody: When true, the response body will be marked as "[excluded]" in logs
//
// This is useful for excluding sensitive data or large binary content from logs.
type BodySkipper func(c *echo.Context) (skipReqBody, skipRespBody bool)

// defaultBodySkipper is the default implementation of BodySkipper that doesn't exclude any bodies.
// It always returns false for both skipReqBody and skipRespBody, meaning all bodies will be logged.
func defaultBodySkipper(_ *echo.Context) (skipReqBody, skipRespBody bool) {
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

	// RedactHeaders lists header names whose values are replaced with "[REDACTED]"
	// when AreHeadersDump is enabled. Matched case-insensitively.
	// When nil, DefaultRedactHeaders is used. Set to an empty (non-nil) slice
	// to disable redaction entirely.
	RedactHeaders []string

	// AreHeadersDump controls whether request and response headers are included in logs.
	// When true, all headers will be logged as structured fields.
	// Sensitive headers listed in RedactHeaders are redacted before logging.
	AreHeadersDump bool

	// IsBodyDump controls whether request and response bodies are included in logs.
	// When true, bodies will be captured and logged as structured fields.
	IsBodyDump bool

	// LimitHTTPBody controls whether to limit the size of logged HTTP bodies.
	// Body dumping automatically enables it for non-negative LimitSize values.
	LimitHTTPBody bool

	// LimitSize specifies the maximum size (in bytes) for logged HTTP bodies.
	// Bodies larger than this will be truncated with "..." appended.
	// When body dumping is enabled, zero uses the default size. A negative value
	// explicitly disables limiting.
	LimitSize int
}

// DefaultRedactHeaders lists header names whose values are redacted by default
// when header dumping is enabled. Names are matched case-insensitively.
var DefaultRedactHeaders = []string{
	"Authorization",
	"Proxy-Authorization",
	"Cookie",
	"Set-Cookie",
	"X-Api-Key",
	"X-Auth-Token",
}

var (
	// DefaultZapConfig is the default Zap Logger middleware config.
	DefaultZapConfig = ZapConfig{
		Skipper:        middleware.DefaultSkipper,
		BodySkipper:    defaultBodySkipper,
		RedactHeaders:  DefaultRedactHeaders,
		AreHeadersDump: false,
		IsBodyDump:     false,
		LimitHTTPBody:  true,
		LimitSize:      500,
	}
)

// createLogFields creates the standard log fields for a request/response.
func createLogFields(c *echo.Context, start time.Time) []zapcore.Field {
	req := c.Request()
	status, _ := responseStatus(c)

	fields := make([]zapcore.Field, 0, 8)
	fields = append(fields,
		zap.Int("status", status),
		zap.Duration("latency", time.Since(start)),
		zap.String("request_id", getRequestID(c)),
		zap.String("method", req.Method),
		zap.String("uri", req.URL.EscapedPath()),
		zap.String("host", req.Host),
		zap.String("remote_ip", c.RealIP()),
	)

	ctxErr := req.Context().Err()
	if ctxErr != nil {
		fields = append(fields, zap.NamedError("context_error", ctxErr))
	}

	return fields
}

// makeHandler creates the middleware handler function.
func makeHandler(ctxLogger *contextlogger.ContextLogger, config ZapConfig) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
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
				respDumper, reqBody = prepareReqAndResp(c, config)
			}

			// Invoke the handler chain. We render the error via the configured
			// HTTPErrorHandler here (instead of returning it) so the response is
			// committed before we read its status for logging; we then return nil
			// so Echo does not invoke HTTPErrorHandler a second time.
			err := next(c)
			if err != nil {
				c.Echo().HTTPErrorHandler(c, err)
			}

			// Create log fields
			fields := createLogFields(c, start)

			// Add headers if configured
			fields = append(fields, addHeaders(config, req.Header, c.Response().Header())...)

			// Add request/response body if configured
			fields = append(fields, addBody(config, c, reqBody, respDumper)...)

			// Log with appropriate level based on status code and commit status
			status, committed := responseStatus(c)
			logit(committed, status, ctxLogger.Ctx(ctx), fields)

			return nil
		}
	}
}

// normalizeConfig returns a ZapConfig with nil fields populated from defaults.
// When no config is provided, normalization starts from DefaultZapConfig.
// When body dumping is enabled, non-negative sizes enable limiting, zero uses
// the default size, and a negative size explicitly disables limiting.
func normalizeConfig(config []ZapConfig) ZapConfig {
	cfg := DefaultZapConfig
	if len(config) > 0 {
		cfg = config[0]
	}

	if cfg.Skipper == nil {
		cfg.Skipper = DefaultZapConfig.Skipper
	}

	if cfg.BodySkipper == nil {
		cfg.BodySkipper = DefaultZapConfig.BodySkipper
	}

	if cfg.RedactHeaders == nil {
		cfg.RedactHeaders = DefaultZapConfig.RedactHeaders
	}

	if cfg.IsBodyDump {
		if cfg.LimitSize < 0 {
			cfg.LimitHTTPBody = false
		} else {
			cfg.LimitHTTPBody = true
			if cfg.LimitSize == 0 {
				cfg.LimitSize = DefaultZapConfig.LimitSize
			}
		}
	}

	return cfg
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
	if ctxLogger == nil {
		ctxLogger = contextlogger.WithContext(zap.NewNop())
	}

	cfg := normalizeConfig(config)

	return makeHandler(ctxLogger, cfg)
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
