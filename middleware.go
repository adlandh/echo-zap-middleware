// Package echozapmiddleware is a logger middleware for echo framework
package echozapmiddleware

import (
	"time"

	contextlogger "github.com/adlandh/context-logger"
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

// MiddlewareWithContextLogger returns a Zap Logger middleware with context logger.
func MiddlewareWithContextLogger(ctxLogger *contextlogger.ContextLogger, config ...ZapConfig) echo.MiddlewareFunc {
	if len(config) == 0 {
		config = append(config, DefaultZapConfig)
	}

	if config[0].Skipper == nil {
		config[0].Skipper = middleware.DefaultSkipper
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if config[0].Skipper(c) || c.Request() == nil || c.Response() == nil {
				return next(c)
			}

			start := time.Now()
			req := c.Request()
			ctx := req.Context()

			defer func() {
				c.SetRequest(req.WithContext(ctx))
			}()

			respDumper, reqBody := prepareReqAndResp(c, config[0])

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
			if config[0].AreHeadersDump {
				fields = append(fields, zap.Any("req.headers", req.Header), zap.Any("resp.headers", res.Header()))
			}

			// add body
			if config[0].IsBodyDump {
				fields = append(fields, zap.String("req.body", limitString(config[0], string(reqBody))),
					zap.String("resp.body", limitString(config[0], respDumper.GetResponse())))
			}

			log(res.Status, ctxLogger.Ctx(ctx), fields)

			return nil
		}
	}
}

// Middleware returns a Zap Logger middleware with config.
// If config is not passed, DefaultZapConfig will be used.
func Middleware(logger *zap.Logger, config ...ZapConfig) echo.MiddlewareFunc {
	if len(config) == 0 {
		config = append(config, DefaultZapConfig)
	}

	return MiddlewareWithContextLogger(contextlogger.WithContext(logger), config[0])
}
