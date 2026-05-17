package echozapmiddleware

import (
	"bytes"
	"io"
	"net/http"
	"unicode/utf8"

	"github.com/adlandh/response-dumper"
	"github.com/labstack/echo/v5"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type readCloser struct {
	io.Reader
	closeFunc func() error
}

func (r readCloser) Close() error {
	return r.closeFunc()
}

// restoreRequestBody returns an io.ReadCloser that replays the captured prefix
// followed (when useOriginal is true) by whatever is left on the original body.
// In the useOriginal path Close delegates to the original body; callers should
// drain the body fully before closing it, otherwise the underlying HTTP/1.1
// connection may not be reusable.
func restoreRequestBody(original io.ReadCloser, captured []byte, useOriginal bool) io.ReadCloser {
	if useOriginal {
		replay := bytes.NewBuffer(captured)

		return readCloser{
			Reader:    io.MultiReader(replay, original),
			closeFunc: original.Close,
		}
	}

	return io.NopCloser(bytes.NewBuffer(captured))
}

// prepareReqAndResp sets up request body capture and response dumping if enabled in config.
// Returns the response dumper and captured request body.
func prepareReqAndResp(c *echo.Context, config ZapConfig) (*response.Dumper, []byte) {
	// If body dumping is not enabled, return nil values
	if !config.IsBodyDump {
		return nil, nil
	}

	var reqBody []byte

	req := c.Request()

	// Capture request body if present
	if req.Body != nil {
		originalBody := req.Body

		var err error

		if config.LimitHTTPBody && config.LimitSize > 0 {
			limitedReader := io.LimitReader(req.Body, int64(config.LimitSize)+1)

			reqBody, err = io.ReadAll(limitedReader)
			if err != nil {
				req.Body = originalBody
			} else {
				req.Body = restoreRequestBody(originalBody, reqBody, true)
			}
		} else {
			reqBody, err = io.ReadAll(req.Body)
			if err != nil {
				req.Body = originalBody
			} else {
				_ = req.Body.Close()
				// Reset original request body so it can be read again by handlers
				req.Body = restoreRequestBody(originalBody, reqBody, false)
			}
		}
	}

	// Set up response dumper
	respWriter := c.Response()

	echoResp, err := echo.UnwrapResponse(respWriter)
	if err != nil {
		respDumper := response.NewDumper(respWriter)
		c.SetResponse(respDumper)

		return respDumper, reqBody
	}

	respDumper := response.NewDumper(echoResp.ResponseWriter)
	echoResp.ResponseWriter = respDumper
	c.SetResponse(echoResp)

	return respDumper, reqBody
}

// limitString truncates a string to the specified size while ensuring UTF-8 validity.
func limitString(str string, size int) string {
	if size <= 0 {
		return ""
	}

	// Quick check if truncation is needed
	if len(str) <= size {
		return str
	}

	if utf8.ValidString(str[:size]) {
		return str[:size]
	}

	// Convert to bytes for UTF-8 handling
	strBytes := []byte(str)

	// Truncate and ensure UTF-8 validity
	validBytes := strBytes[:size]
	for !utf8.Valid(validBytes) && len(validBytes) > 0 {
		validBytes = validBytes[:len(validBytes)-1]
	}

	return string(validBytes)
}

// limitStringWithDots truncates a string and appends "..." if truncation
// occurred.
//
// When size <= 10 the trailing dots are intentionally omitted: at small
// budgets, three of the few available bytes spent on an ellipsis are more
// costly than the missing truncation signal. Callers that need a visible
// indicator at small sizes should configure a larger LimitSize.
func limitStringWithDots(str string, size int) string {
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

// limitBytesWithDots is the []byte counterpart to limitStringWithDots. It
// always returns a fresh slice when truncation occurs so callers can safely
// retain the result without aliasing the source buffer.
func limitBytesWithDots(b []byte, size int) []byte {
	if size <= 10 {
		return limitBytes(b, size)
	}

	truncated := limitBytes(b, size-3)
	if len(truncated) == len(b) {
		return b
	}

	out := make([]byte, len(truncated)+3)
	copy(out, truncated)
	copy(out[len(truncated):], "...")

	return out
}

// limitBytes truncates b to at most size bytes, trimming any trailing partial
// UTF-8 sequence so the result remains valid UTF-8.
func limitBytes(b []byte, size int) []byte {
	if size <= 0 {
		return nil
	}

	if len(b) <= size {
		return b
	}

	valid := b[:size]
	for !utf8.Valid(valid) && len(valid) > 0 {
		valid = valid[:len(valid)-1]
	}

	return valid
}

// limitBody applies the configured size limit to a body byte slice.
func limitBody(config ZapConfig, b []byte) []byte {
	if !config.LimitHTTPBody || config.LimitSize <= 0 {
		return b
	}

	return limitBytesWithDots(b, config.LimitSize)
}

// limitBodyString applies the configured size limit to a body string.
func limitBodyString(config ZapConfig, s string) string {
	if !config.LimitHTTPBody || config.LimitSize <= 0 {
		return s
	}

	return limitStringWithDots(s, config.LimitSize)
}

// getRequestID extracts the request ID from headers, checking both request and response headers.
func getRequestID(ctx *echo.Context) string {
	// First check request header (usually set by reverse-proxy)
	requestID := ctx.Request().Header.Get(echo.HeaderXRequestID)
	if requestID == "" {
		// If not found, check response header (might be generated by middleware.RequestID())
		requestID = ctx.Response().Header().Get(echo.HeaderXRequestID)
	}

	return requestID
}

// logit logs the request with appropriate level based on HTTP status code.
func logit(committed bool, status int, logger *zap.Logger, fields []zapcore.Field) {
	switch {
	case !committed:
		logger.Warn("Response not committed", fields...)
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
// Header values listed in config.RedactHeaders are replaced with "[REDACTED]"
// to avoid leaking secrets such as Authorization tokens or session cookies.
func addHeaders(config ZapConfig, reqHeaders http.Header, resHeaders http.Header) []zapcore.Field {
	if !config.AreHeadersDump {
		return nil
	}

	return []zapcore.Field{
		zap.Any("req.headers", redactHeaders(reqHeaders, config.RedactHeaders)),
		zap.Any("resp.headers", redactHeaders(resHeaders, config.RedactHeaders)),
	}
}

// redactHeaders returns a copy of h with the values of any header listed in
// redact (matched case-insensitively) replaced by "[REDACTED]". The input
// header map is never mutated. When redact is empty the original map is
// returned unchanged.
func redactHeaders(h http.Header, redact []string) http.Header {
	if len(redact) == 0 || len(h) == 0 {
		return h
	}

	redactSet := make(map[string]struct{}, len(redact))
	for _, name := range redact {
		redactSet[http.CanonicalHeaderKey(name)] = struct{}{}
	}

	out := make(http.Header, len(h))

	for name, values := range h {
		if _, ok := redactSet[http.CanonicalHeaderKey(name)]; ok {
			masked := make([]string, len(values))
			for i := range values {
				masked[i] = "[REDACTED]"
			}

			out[name] = masked

			continue
		}

		out[name] = values
	}

	return out
}

// addBody adds request and response body fields to the log if body dumping is enabled.
// Bodies can be excluded based on the BodySkipper function in the config.
func addBody(config ZapConfig, c *echo.Context, reqBody []byte, respDumper *response.Dumper) []zapcore.Field {
	if !config.IsBodyDump {
		return nil
	}

	if respDumper == nil {
		return nil
	}

	skipReq, skipResp := config.BodySkipper(c)
	fields := make([]zapcore.Field, 0, 2) // Pre-allocate for 2 fields

	// Process request body
	reqBodyContent := limitBody(config, reqBody)
	if len(reqBodyContent) > 0 && skipReq {
		fields = append(fields, zap.String("req.body", "[excluded]"))
	} else {
		fields = append(fields, zap.ByteString("req.body", reqBodyContent))
	}

	// Process response body
	respBodyContent := limitBodyString(config, respDumper.GetResponse())
	if respBodyContent != "" && skipResp {
		respBodyContent = "[excluded]"
	}

	fields = append(fields, zap.String("resp.body", respBodyContent))

	return fields
}

// responseStatus reports the HTTP status written so far and whether the
// response has been committed. In normal Echo flow c.Response() is always an
// *echo.Response, so the unwrap succeeds; the (0, false) fallback is only
// reached if a caller has swapped in a non-Echo response (unusual) and is
// indistinguishable from a handler that never wrote anything.
func responseStatus(c *echo.Context) (status int, committed bool) {
	resp, err := echo.UnwrapResponse(c.Response())
	if err == nil && resp != nil {
		return resp.Status, resp.Committed
	}

	return 0, false
}
