// Package echozapmiddleware is a logger middleware for echo framework
package echozapmiddleware

import (
	"bytes"
	"io"
	"net/http"
	"time"

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
		LimitSize:      1024,
	}
)

// Middleware returns a Zap Logger middleware with default config.
func Middleware(logger *zap.Logger) echo.MiddlewareFunc {
	return MiddlewareWithConfig(logger, DefaultZapConfig)
}

// MiddlewareWithConfig returns a Zap Logger middleware with config.
func MiddlewareWithConfig(logger *zap.Logger, config ZapConfig) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(ctx echo.Context) error {
			start := time.Now()

			req, respDumper, reqBody, recoverReq := prepareReqAndResp(ctx, config)
			defer recoverReq()

			err := next(ctx)
			if err != nil {
				ctx.Error(err)
			}

			res := ctx.Response()
			id := getRequestID(ctx)
			fields := []zapcore.Field{
				zap.Int("status", res.Status),
				zap.String("latency", time.Since(start).String()),
				zap.String("request_id", id),
				zap.String("method", req.Method),
				zap.String("uri", req.RequestURI),
				zap.String("host", req.Host),
				zap.String("remote_ip", ctx.RealIP()),
			}
			n := res.Status

			switch {
			case n >= 500:
				logger.Error("Server error", fields...)
			case n >= 400:
				logger.Warn("Client error", fields...)
			case n >= 300:
				logger.Info("Redirection", fields...)
			default:
				logger.Info("Success", fields...)
			}

			if config.IsBodyDump || config.AreHeadersDump {
				additionalFields := make([]zapcore.Field, 0, 4)
				// add headers
				if config.AreHeadersDump {
					additionalFields = append(additionalFields, zap.Any("request headers", req.Header), zap.Any("response headers", res.Header()))
				}

				// add body
				if config.IsBodyDump {
					additionalFields = append(additionalFields,
						zap.String("request body", limitString(config, string(reqBody))),
						zap.String("response body", limitString(config, respDumper.GetResponse())),
					)
				}

				logger.Debug("Additional info", additionalFields...)
			}

			return nil
		}
	}
}

func prepareReqAndResp(ctx echo.Context, config ZapConfig) (*http.Request, *responseDumper, []byte, func()) {
	var respDumper *responseDumper

	var reqBody []byte

	req := ctx.Request()
	savedCtx := req.Context()

	if config.IsBodyDump {
		if req.Body != nil {
			var err error

			reqBody, err = io.ReadAll(req.Body)
			if err == nil {
				_ = req.Body.Close()
				req.Body = io.NopCloser(bytes.NewBuffer(reqBody)) // reset original request body
			}
		}

		respDumper = newResponseDumper(ctx.Response())
		ctx.Response().Writer = respDumper
	}

	return req, respDumper, reqBody, func() {
		ctx.SetRequest(req.WithContext(savedCtx))
	}
}

func limitString(config ZapConfig, str string) string {
	if !config.LimitHTTPBody || len(str) <= config.LimitSize {
		return str
	}

	return str[:config.LimitSize-3] + "..."
}

func getRequestID(ctx echo.Context) string {
	requestID := ctx.Request().Header.Get(echo.HeaderXRequestID) // request-id generated by reverse-proxy
	if requestID == "" {
		// missed request-id from proxy, got generated one by middleware.RequestID()
		requestID = ctx.Response().Header().Get(echo.HeaderXRequestID)
	}

	return requestID
}
