package echozapmiddleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/adlandh/response-dumper"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestPrepareReqAndResp_NoBodyDump(t *testing.T) {
	t.Parallel()

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("hello"))
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)

	respDumper, reqBody := prepareReqAndResp(ctx, ZapConfig{IsBodyDump: false})

	require.Nil(t, respDumper)
	require.Empty(t, reqBody)
	resp, err := echo.UnwrapResponse(ctx.Response())
	require.NoError(t, err)
	_, isDumper := resp.ResponseWriter.(*response.Dumper)
	require.False(t, isDumper)

	readBody, err := io.ReadAll(ctx.Request().Body)
	require.NoError(t, err)
	require.Equal(t, "hello", string(readBody))
}

func TestPrepareReqAndResp_WithBodyDump(t *testing.T) {
	t.Parallel()

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("hello"))
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)

	respDumper, reqBody := prepareReqAndResp(ctx, ZapConfig{IsBodyDump: true})

	require.NotNil(t, respDumper)
	require.Equal(t, "hello", string(reqBody))
	resp, err := echo.UnwrapResponse(ctx.Response())
	require.NoError(t, err)
	require.Same(t, respDumper, resp.ResponseWriter)

	readBody, err := io.ReadAll(ctx.Request().Body)
	require.NoError(t, err)
	require.Equal(t, "hello", string(readBody))
}

func TestPrepareReqAndResp_LimitBodyPreservesFullRequest(t *testing.T) {
	t.Parallel()

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("hello world"))
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)

	respDumper, reqBody := prepareReqAndResp(ctx, ZapConfig{
		IsBodyDump:    true,
		LimitHTTPBody: true,
		LimitSize:     4,
	})

	require.NotNil(t, respDumper)
	require.Equal(t, "hello", string(reqBody))

	readBody, err := io.ReadAll(ctx.Request().Body)
	require.NoError(t, err)
	require.Equal(t, "hello world", string(readBody))
}

type errReadCloser struct{}

func (errReadCloser) Read([]byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}

func (errReadCloser) Close() error {
	return nil
}

type partialErrReadCloser struct {
	read bool
}

func (r *partialErrReadCloser) Read(p []byte) (int, error) {
	if r.read {
		return 0, io.EOF
	}

	r.read = true

	return copy(p, "hello"), io.ErrUnexpectedEOF
}

func (*partialErrReadCloser) Close() error {
	return nil
}

func TestPrepareReqAndResp_ReadErrorRestoresBody(t *testing.T) {
	t.Parallel()

	e := echo.New()
	originalBody := errReadCloser{}
	req := httptest.NewRequest(http.MethodPost, "/", http.NoBody)
	req.Body = originalBody
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)

	respDumper, reqBody := prepareReqAndResp(ctx, ZapConfig{IsBodyDump: true})

	require.NotNil(t, respDumper)
	require.Empty(t, reqBody)
	_, ok := ctx.Request().Body.(errReadCloser)
	require.True(t, ok)
}

func TestPrepareReqAndResp_PartialReadErrorReplaysCapturedBody(t *testing.T) {
	t.Parallel()

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/", http.NoBody)
	req.Body = &partialErrReadCloser{}
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)

	respDumper, reqBody := prepareReqAndResp(ctx, ZapConfig{IsBodyDump: true})

	require.NotNil(t, respDumper)
	require.Equal(t, "hello", string(reqBody))

	readBody, err := io.ReadAll(ctx.Request().Body)
	require.NoError(t, err)
	require.Equal(t, "hello", string(readBody))
}

func TestLimitBytes(t *testing.T) {
	t.Parallel()

	t.Run("no truncation", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, []byte("hello"), limitBytes([]byte("hello"), 10))
	})

	t.Run("size zero returns nil", func(t *testing.T) {
		t.Parallel()
		require.Nil(t, limitBytes([]byte("hello"), 0))
	})

	t.Run("negative size returns nil", func(t *testing.T) {
		t.Parallel()
		require.Nil(t, limitBytes([]byte("hello"), -1))
	})

	t.Run("truncates invalid utf8 bytes", func(t *testing.T) {
		t.Parallel()
		euro := []byte{0xE2, 0x82, 0xAC}
		input := append([]byte("ab"), euro...)
		input = append(input, []byte("cd")...)
		require.Equal(t, []byte("ab"), limitBytes(input, 3))
	})

	t.Run("only invalid utf8 returns empty", func(t *testing.T) {
		t.Parallel()
		require.Empty(t, limitBytes([]byte{0xE2, 0x82}, 1))
	})
}

func TestLimitBytesWithDots(t *testing.T) {
	t.Parallel()

	t.Run("size ten keeps no dots", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, []byte("0123456789"), limitBytesWithDots([]byte("0123456789ABC"), 10))
	})

	t.Run("size four keeps no dots", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, []byte("0123"), limitBytesWithDots([]byte("0123456789"), 4))
	})

	t.Run("truncated adds dots", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, []byte("012345678..."), limitBytesWithDots([]byte("0123456789ABCDEF"), 12))
	})

	t.Run("no truncation keeps original", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, []byte("short"), limitBytesWithDots([]byte("short"), 20))
	})
}

func TestLimitBody(t *testing.T) {
	t.Parallel()

	config := ZapConfig{LimitHTTPBody: true, LimitSize: 12}
	require.Equal(t, []byte("012345678..."), limitBody(config, []byte("0123456789ABCDEF")))

	config = ZapConfig{LimitHTTPBody: false, LimitSize: 12}
	require.Equal(t, []byte("0123456789ABCDEF"), limitBody(config, []byte("0123456789ABCDEF")))

	config = ZapConfig{LimitHTTPBody: true, LimitSize: 0}
	require.Equal(t, []byte("0123456789ABCDEF"), limitBody(config, []byte("0123456789ABCDEF")))
}

func TestGetRequestID(t *testing.T) {
	t.Parallel()

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)

	req.Header.Set(echo.HeaderXRequestID, "from-request")
	ctx.Response().Header().Set(echo.HeaderXRequestID, "from-response")
	require.Equal(t, "from-request", getRequestID(ctx))

	req.Header.Del(echo.HeaderXRequestID)
	require.Equal(t, "from-response", getRequestID(ctx))

	ctx.Response().Header().Del(echo.HeaderXRequestID)
	require.Equal(t, "", getRequestID(ctx))
}

func TestLogit(t *testing.T) {
	t.Parallel()

	core, observed := observer.New(zapcore.InfoLevel)
	logger := zap.New(core)

	logit(false, 200, logger, nil)
	entries := observed.TakeAll()
	require.Len(t, entries, 1)
	require.Equal(t, zapcore.WarnLevel, entries[0].Level)
	require.Equal(t, "Response not committed", entries[0].Message)

	logit(true, 500, logger, nil)
	entries = observed.TakeAll()
	require.Len(t, entries, 1)
	require.Equal(t, zapcore.ErrorLevel, entries[0].Level)
	require.Equal(t, "Server error", entries[0].Message)

	logit(true, 404, logger, nil)
	entries = observed.TakeAll()
	require.Len(t, entries, 1)
	require.Equal(t, zapcore.WarnLevel, entries[0].Level)
	require.Equal(t, "Client error", entries[0].Message)

	logit(true, 302, logger, nil)
	entries = observed.TakeAll()
	require.Len(t, entries, 1)
	require.Equal(t, zapcore.InfoLevel, entries[0].Level)
	require.Equal(t, "Redirection", entries[0].Message)

	logit(true, 200, logger, nil)
	entries = observed.TakeAll()
	require.Len(t, entries, 1)
	require.Equal(t, zapcore.InfoLevel, entries[0].Level)
	require.Equal(t, "Success", entries[0].Message)
}

func TestAddHeaders(t *testing.T) {
	t.Parallel()

	reqHeaders := http.Header{"X-Req": []string{"1"}}
	resHeaders := http.Header{"X-Resp": []string{"2"}}

	fields := addHeaders(ZapConfig{AreHeadersDump: false}, reqHeaders, resHeaders)
	require.Nil(t, fields)

	fields = addHeaders(ZapConfig{AreHeadersDump: true}, reqHeaders, resHeaders)
	require.Len(t, fields, 2)
	require.Equal(t, "req.headers", fields[0].Key)
	require.Equal(t, "resp.headers", fields[1].Key)
}

func TestRedactHeaders(t *testing.T) {
	t.Parallel()

	h := http.Header{
		"Authorization": []string{"Bearer secret"},
		"Cookie":        []string{"sid=abc", "tracking=xyz"},
		"X-Trace-Id":    []string{"123"},
	}

	out := redactHeaders(h, DefaultRedactHeaders)
	require.Equal(t, []string{"[REDACTED]"}, out["Authorization"])
	require.Equal(t, []string{"[REDACTED]", "[REDACTED]"}, out["Cookie"])
	require.Equal(t, []string{"123"}, out["X-Trace-Id"])
	// Original map is not mutated.
	require.Equal(t, []string{"Bearer secret"}, h["Authorization"])

	// Empty redact list returns input unchanged.
	require.Equal(t, h, redactHeaders(h, nil))

	// Case-insensitive matching: redact entry may be non-canonical.
	canonical := http.Header{}
	canonical.Set("Authorization", "Bearer secret")
	out = redactHeaders(canonical, []string{"AUTHORIZATION"})
	require.Equal(t, []string{"[REDACTED]"}, out.Values("Authorization"))
}

func TestAddBody(t *testing.T) {
	t.Parallel()

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)

	respDumper := response.NewDumper(rec)
	_, err := respDumper.Write([]byte("response"))
	require.NoError(t, err)

	fields := addBody(ZapConfig{IsBodyDump: false}, ctx, []byte("request"), respDumper)
	require.Nil(t, fields)

	config := ZapConfig{
		IsBodyDump:    true,
		LimitHTTPBody: false,
		BodySkipper: func(*echo.Context) (bool, bool) {
			return true, true
		},
	}
	fields = addBody(config, ctx, []byte("request"), respDumper)
	require.Len(t, fields, 2)
	require.Equal(t, "[excluded]", fields[0].String)
	require.Equal(t, "[excluded]", fields[1].String)

	config = ZapConfig{
		IsBodyDump:    true,
		LimitHTTPBody: true,
		LimitSize:     12,
		BodySkipper: func(*echo.Context) (bool, bool) {
			return false, false
		},
	}
	fields = addBody(config, ctx, []byte("0123456789ABCDEF"), respDumper)
	require.Len(t, fields, 2)
	require.Equal(t, "012345678...", string(fields[0].Interface.([]byte)))
	require.Equal(t, "response", string(fields[1].Interface.([]byte)))

	config = ZapConfig{
		IsBodyDump:    true,
		LimitHTTPBody: true,
		LimitSize:     0,
		BodySkipper: func(*echo.Context) (bool, bool) {
			return false, false
		},
	}
	fields = addBody(config, ctx, []byte("0123456789ABCDEF"), respDumper)
	require.Len(t, fields, 2)
	require.Equal(t, "0123456789ABCDEF", string(fields[0].Interface.([]byte)))
	require.Equal(t, "response", string(fields[1].Interface.([]byte)))
}

func TestResponseStatus_NonEchoResponseFallback(t *testing.T) {
	t.Parallel()

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)

	// Swap the response for a non-*echo.Response writer so UnwrapResponse fails.
	ctx.SetResponse(response.NewDumper(rec))

	status, committed := responseStatus(ctx)
	require.Equal(t, 0, status)
	require.False(t, committed)
}

func TestReadCloser_CloseDelegates(t *testing.T) {
	t.Parallel()

	called := false
	rc := readCloser{
		Reader: strings.NewReader(""),
		closeFunc: func() error {
			called = true
			return io.EOF
		},
	}

	require.ErrorIs(t, rc.Close(), io.EOF)
	require.True(t, called)
}
