package echozapmiddleware

import (
	"bytes"
	"io"
	"net/http"
	"unicode/utf8"

	"github.com/adlandh/response-dumper"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// prepareReqAndResp sets up request body capture and response dumping if enabled in config.
// Returns the response dumper and captured request body.
func prepareReqAndResp(c echo.Context, config ZapConfig) (*response.Dumper, []byte) {
	// If body dumping is not enabled, return nil values
	if !config.IsBodyDump {
		return nil, nil
	}

	var reqBody []byte

	req := c.Request()

	// Capture request body if present
	if req.Body != nil {
		var err error

		reqBody, err = io.ReadAll(req.Body)
		if err == nil {
			_ = req.Body.Close()
			// Reset original request body so it can be read again by handlers
			req.Body = io.NopCloser(bytes.NewBuffer(reqBody))
		}
	}

	// Set up response dumper
	respDumper := response.NewDumper(c.Response().Writer)
	c.Response().Writer = respDumper

	return respDumper, reqBody
}

// limitString truncates a string to the specified size while ensuring UTF-8 validity.
func limitString(str string, size int) string {
	// Quick check if truncation is needed
	if len(str) <= size {
		return str
	}

	// Convert to bytes for UTF-8 handling
	strBytes := []byte(str)
	if len(strBytes) <= size {
		return str
	}

	// Truncate and ensure UTF-8 validity
	validBytes := strBytes[:size]
	for !utf8.Valid(validBytes) && len(validBytes) > 0 {
		validBytes = validBytes[:len(validBytes)-1]
	}

	return string(validBytes)
}

// limitStringWithDots truncates a string and adds "..." if truncated.
func limitStringWithDots(str string, size int) string {
	// For very small sizes, just truncate without dots
	if size <= 10 {
		return limitString(str, size)
	}

	// Reserve space for "..." if needed
	result := limitString(str, size-3)

	// If no truncation occurred, return original string
	if result == str {
		return str
	}

	return result + "..."
}

// limitBody applies size limits to HTTP body content if configured.
func limitBody(config ZapConfig, str string) string {
	if !config.LimitHTTPBody {
		return str
	}

	return limitStringWithDots(str, config.LimitSize)
}

// getRequestID extracts the request ID from headers, checking both request and response headers.
func getRequestID(ctx echo.Context) string {
	// First check request header (usually set by reverse-proxy)
	requestID := ctx.Request().Header.Get(echo.HeaderXRequestID)
	if requestID == "" {
		// If not found, check response header (might be generated by middleware.RequestID())
		requestID = ctx.Response().Header().Get(echo.HeaderXRequestID)
	}

	return requestID
}

// logit logs the request with appropriate level based on HTTP status code.
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

// addHeaders adds request and response headers to log fields if enabled in config.
func addHeaders(config ZapConfig, reqHeaders http.Header, resHeaders http.Header) []zapcore.Field {
	if !config.AreHeadersDump {
		return nil
	}

	return []zapcore.Field{
		zap.Any("req.headers", reqHeaders),
		zap.Any("resp.headers", resHeaders),
	}
}

// addBody adds request and response body fields to the log if body dumping is enabled.
// Bodies can be excluded based on the BodySkipper function in the config.
func addBody(config ZapConfig, c echo.Context, reqBody string, respDumper *response.Dumper) []zapcore.Field {
	if !config.IsBodyDump {
		return nil
	}

	skipReq, skipResp := config.BodySkipper(c)
	fields := make([]zapcore.Field, 0, 2) // Pre-allocate for 2 fields

	// Process request body
	reqBodyContent := limitBody(config, reqBody)
	if len(reqBodyContent) > 0 && skipReq {
		reqBodyContent = "[excluded]"
	}

	fields = append(fields, zap.String("req.body", reqBodyContent))

	// Process response body
	respBodyContent := limitBody(config, respDumper.GetResponse())
	if len(respBodyContent) > 0 && skipResp {
		respBodyContent = "[excluded]"
	}

	fields = append(fields, zap.String("resp.body", respBodyContent))

	return fields
}
