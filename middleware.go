// Package echozapmiddleware is a logger middleware for echo framework
package echozapmiddleware

import (
	"regexp"
	"time"

	contextlogger "github.com/adlandh/context-logger"
	"github.com/adlandh/response-dumper"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type (
	// ZapConfig defines the config for Zap Logger middleware.
	ZapConfig struct {
		// Skipper defines a function to skip middleware.
		Skipper middleware.Skipper

		// BodySkipper defines a function to exclude body from logging
		BodySkipper middleware.Skipper

		// paths (regular expressions) or endpoints (ex: `/ping/:id`) to exclude from dumping response bodies
		DumpNoResponseBodyForPaths []string

		// paths (regular expressions) or endpoints (ex: `/ping/:id`) to exclude from dumping request bodies (regular expressions)
		DumpNoRequestBodyForPaths []string

		// add req headers & resp headers to tracing tags
		AreHeadersDump bool

		// add req body & resp body to attributes
		IsBodyDump bool

		// prevent logging long http request bodies
		LimitHTTPBody bool

		// http body limit size (in bytes)
		LimitSize int
	}
)

var regexExcludedPathsReq, regexExcludedPathsResp []*regexp.Regexp

var (
	// DefaultZapConfig is the default Zap Logger middleware config.
	DefaultZapConfig = ZapConfig{
		Skipper:        middleware.DefaultSkipper,
		AreHeadersDump: false,
		IsBodyDump:     false,
		LimitHTTPBody:  true,
		LimitSize:      500,
	}
)

func makeHandler(ctxLogger *contextlogger.ContextLogger, config ZapConfig) echo.MiddlewareFunc {
	prepareRegexs(ctxLogger, config)

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if config.Skipper(c) || c.Request() == nil || c.Response() == nil {
				return next(c)
			}

			start := time.Now()
			req := c.Request()
			ctx := req.Context()

			var respDumper *response.Dumper

			var reqBody []byte

			if config.IsBodyDump {
				defer func() {
					c.SetRequest(req.WithContext(ctx))
				}()

				respDumper, reqBody = prepareReqAndResp(c, config)
			}

			err := next(c)
			if err != nil {
				c.Error(err)
			}

			res := c.Response()

			fields := []zapcore.Field{
				zap.Int("status", res.Status),
				zap.String("latency", time.Since(start).String()),
				zap.String("request_id", getRequestID(c)),
				zap.String("method", req.Method),
				zap.String("uri", req.RequestURI),
				zap.String("host", req.Host),
				zap.String("remote_ip", c.RealIP()),
			}

			// add headers
			fields = append(fields, addHeaders(config, req.Header, res.Header())...)

			// add body
			fields = append(fields, addBody(config, c, string(reqBody), respDumper)...)

			logit(res.Status, ctxLogger.Ctx(ctx), fields)

			return nil
		}
	}
}

// MiddlewareWithContextLogger returns a Zap Logger middleware with context logger.
func MiddlewareWithContextLogger(ctxLogger *contextlogger.ContextLogger, config ...ZapConfig) echo.MiddlewareFunc {
	if len(config) == 0 {
		config = []ZapConfig{DefaultZapConfig}
	}

	if config[0].Skipper == nil {
		config[0].Skipper = middleware.DefaultSkipper
	}

	if config[0].BodySkipper == nil {
		config[0].BodySkipper = middleware.DefaultSkipper
	}

	return makeHandler(ctxLogger, config[0])
}

// Middleware returns a Zap Logger middleware with config.
// If config is not passed, DefaultZapConfig will be used.
func Middleware(logger *zap.Logger, config ...ZapConfig) echo.MiddlewareFunc {
	return MiddlewareWithContextLogger(contextlogger.WithContext(logger), config...)
}
