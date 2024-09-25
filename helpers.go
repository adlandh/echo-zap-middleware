package echozapmiddleware

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"regexp"
	"unicode/utf8"

	contextlogger "github.com/adlandh/context-logger"
	"github.com/adlandh/response-dumper"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func prepareReqAndResp(c echo.Context, config ZapConfig) (*response.Dumper, []byte) {
	var respDumper *response.Dumper

	var reqBody []byte

	req := c.Request()

	if config.IsBodyDump {
		if req.Body != nil {
			var err error

			reqBody, err = io.ReadAll(req.Body)
			if err == nil {
				_ = req.Body.Close()
				req.Body = io.NopCloser(bytes.NewBuffer(reqBody)) // reset original request body
			}
		}

		respDumper = response.NewDumper(c.Response().Writer)
		c.Response().Writer = respDumper
	}

	return respDumper, reqBody
}

func limitString(str string, size int) string {
	if len(str) <= size {
		return str
	}

	bytes := []byte(str)

	if len(bytes) <= size {
		return str
	}

	validBytes := bytes[:size]
	for !utf8.Valid(validBytes) {
		validBytes = validBytes[:len(validBytes)-1]
	}

	return string(validBytes)
}

func limitStringWithDots(str string, size int) string {
	if size <= 10 {
		return limitString(str, size)
	}

	result := limitString(str, size-3)
	if result == str {
		return str
	}

	return result + "..."
}

func limitBody(config ZapConfig, str string) string {
	if !config.LimitHTTPBody {
		return str
	}

	return limitStringWithDots(str, config.LimitSize)
}

func getRequestID(ctx echo.Context) string {
	requestID := ctx.Request().Header.Get(echo.HeaderXRequestID) // request-id generated by reverse-proxy
	if requestID == "" {
		// missed request-id from proxy, got generated one by middleware.RequestID()
		requestID = ctx.Response().Header().Get(echo.HeaderXRequestID)
	}

	return requestID
}

func logit(status int, logger *zap.Logger, fields []zapcore.Field) {
	switch {
	case status >= 500:
		logger.Error("Server error", fields...)
	case status >= 400:
		logger.Warn("Client error", fields...)
	case status >= 300:
		logger.Info("Redirection", fields...)
	default:
		logger.Info("Success", fields...)
	}
}

func isExcluded(path string, endpoint string, regexs []*regexp.Regexp, endpoints []string) bool {
	if len(endpoints) > 0 {
		for _, endpointExcluded := range endpoints {
			if endpointExcluded == endpoint {
				return true
			}
		}
	}

	if len(regexs) > 0 {
		for _, regexExcludedPath := range regexs {
			if regexExcludedPath.MatchString(path) {
				return true
			}
		}
	}

	return false
}

func prepareRegexs(ctxLogger *contextlogger.ContextLogger, config ZapConfig) {
	regexExcludedPathsResp = make([]*regexp.Regexp, 0, len(config.DumpNoResponseBodyForPaths))
	regexExcludedPathsReq = make([]*regexp.Regexp, 0, len(config.DumpNoRequestBodyForPaths))

	if !config.IsBodyDump {
		return
	}

	if len(config.DumpNoResponseBodyForPaths) > 0 {
		for _, path := range config.DumpNoResponseBodyForPaths {
			regexExcludedPath, err := regexp.Compile(path)
			if err != nil {
				// Just warn and continue
				ctxLogger.Ctx(context.Background()).Warn("error to compile regex", zap.String("path", path), zap.Error(err))
				continue
			}

			regexExcludedPathsResp = append(regexExcludedPathsResp, regexExcludedPath)
		}
	}

	if len(config.DumpNoRequestBodyForPaths) > 0 {
		for _, path := range config.DumpNoRequestBodyForPaths {
			regexExcludedPath, err := regexp.Compile(path)
			if err != nil {
				ctxLogger.Ctx(context.Background()).Warn("error to compile regex", zap.String("path", path), zap.Error(err))
				continue
			}

			regexExcludedPathsReq = append(regexExcludedPathsReq, regexExcludedPath)
		}
	}
}

func addHeaders(config ZapConfig, reqHeaders http.Header, resHeaders http.Header) []zapcore.Field {
	if !config.AreHeadersDump {
		return nil
	}

	return []zapcore.Field{
		zap.Any("req.headers", reqHeaders),
		zap.Any("resp.headers", resHeaders),
	}
}

func addBody(config ZapConfig, path string, endpoint string, reqBody string, respDumper *response.Dumper) []zapcore.Field {
	if !config.IsBodyDump {
		return nil
	}

	var fields []zapcore.Field

	body := limitBody(config, reqBody)
	if len(body) > 0 && isExcluded(path, endpoint, regexExcludedPathsReq, config.DumpNoRequestBodyForPaths) {
		body = "[excluded]"
	}

	fields = append(fields, zap.String("req.body", body))

	body = limitBody(config, respDumper.GetResponse())
	if len(body) > 0 && isExcluded(path, endpoint, regexExcludedPathsResp, config.DumpNoResponseBodyForPaths) {
		body = "[excluded]"
	}

	fields = append(fields, zap.String("resp.body", body))

	return fields
}
