package echozapmiddleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	contextlogger "github.com/adlandh/context-logger"
	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	"go.uber.org/zap"
)

// setupBenchmarkRouter creates a new Echo router with the specified middleware configuration
func setupBenchmarkRouter(b *testing.B, logger *zap.Logger, config ...ZapConfig) *echo.Echo {
	b.Helper()
	router := echo.New()
	router.Use(middleware.RequestID())
	router.Use(Middleware(logger, config...))
	router.GET("/ping", func(c *echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})
	router.POST("/echo", func(c *echo.Context) error {
		body := new(bytes.Buffer)
		_, err := body.ReadFrom(c.Request().Body)
		if err != nil {
			return err
		}
		return c.String(http.StatusOK, body.String())
	})
	return router
}

// discardSinkRegistered is used to track if the discard sink has been registered
var discardSinkRegistered bool

// setupBenchmarkLogger creates a new Zap logger that discards all output
func setupBenchmarkLogger(b *testing.B) *zap.Logger {
	b.Helper()

	// Register a no-op sink that discards all output (only once)
	if !discardSinkRegistered {
		err := zap.RegisterSink("discard", func(*url.URL) (zap.Sink, error) {
			return &discardSink{}, nil
		})
		if err != nil {
			b.Fatal(err)
		}
		discardSinkRegistered = true
	}

	conf := zap.NewProductionConfig()
	conf.OutputPaths = []string{"discard://"}
	logger, err := conf.Build()
	if err != nil {
		b.Fatal(err)
	}
	return logger
}

// discardSink is a zap.Sink that discards all writes
type discardSink struct{}

func (*discardSink) Write(p []byte) (n int, err error) { return len(p), nil }
func (*discardSink) Sync() error                       { return nil }
func (*discardSink) Close() error                      { return nil }

// BenchmarkMiddlewareDefault benchmarks the middleware with default configuration
func BenchmarkMiddlewareDefault(b *testing.B) {
	logger := setupBenchmarkLogger(b)
	router := setupBenchmarkRouter(b, logger)
	req := httptest.NewRequest(http.MethodGet, "/ping", http.NoBody)
	w := httptest.NewRecorder()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		router.ServeHTTP(w, req)
	}
}

// BenchmarkMiddlewareWithBodyAndHeaders benchmarks the middleware with body and header logging enabled
func BenchmarkMiddlewareWithBodyAndHeaders(b *testing.B) {
	logger := setupBenchmarkLogger(b)
	router := setupBenchmarkRouter(b, logger, ZapConfig{
		AreHeadersDump: true,
		IsBodyDump:     true,
	})
	req := httptest.NewRequest(http.MethodGet, "/ping", http.NoBody)
	w := httptest.NewRecorder()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		router.ServeHTTP(w, req)
	}
}

// BenchmarkMiddlewareWithLargeBody benchmarks the middleware with a large request body
func BenchmarkMiddlewareWithLargeBody(b *testing.B) {
	logger := setupBenchmarkLogger(b)
	router := setupBenchmarkRouter(b, logger, ZapConfig{
		IsBodyDump: true,
	})

	// Create a large body (10KB)
	largeBody := strings.Repeat("abcdefghij", 1000)
	req := httptest.NewRequest(http.MethodPost, "/echo", strings.NewReader(largeBody))
	w := httptest.NewRecorder()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		router.ServeHTTP(w, req)
	}
}

// BenchmarkMiddlewareWithBodyLimit benchmarks the middleware with body size limiting
func BenchmarkMiddlewareWithBodyLimit(b *testing.B) {
	logger := setupBenchmarkLogger(b)
	router := setupBenchmarkRouter(b, logger, ZapConfig{
		IsBodyDump:    true,
		LimitHTTPBody: true,
		LimitSize:     100, // Limit to 100 bytes
	})

	// Create a large body (10KB)
	largeBody := strings.Repeat("abcdefghij", 1000)
	req := httptest.NewRequest(http.MethodPost, "/echo", strings.NewReader(largeBody))
	w := httptest.NewRecorder()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		router.ServeHTTP(w, req)
	}
}

// BenchmarkMiddlewareWithBodySkipper benchmarks the middleware with a body skipper function
func BenchmarkMiddlewareWithBodySkipper(b *testing.B) {
	logger := setupBenchmarkLogger(b)
	router := setupBenchmarkRouter(b, logger, ZapConfig{
		IsBodyDump: true,
		BodySkipper: func(*echo.Context) (skipReq, skipResp bool) {
			return true, true // Always skip both request and response bodies
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/echo", strings.NewReader("test body"))
	w := httptest.NewRecorder()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		router.ServeHTTP(w, req)
	}
}

// BenchmarkMiddlewareWithContextLogger benchmarks the middleware with context logger
func BenchmarkMiddlewareWithContextLogger(b *testing.B) {
	logger := setupBenchmarkLogger(b)
	ctxLogger := contextlogger.WithContext(logger)

	router := echo.New()
	router.Use(middleware.RequestID())
	router.Use(MiddlewareWithContextLogger(ctxLogger))
	router.GET("/ping", func(c *echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", http.NoBody)
	w := httptest.NewRecorder()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		router.ServeHTTP(w, req)
	}
}

// BenchmarkMiddlewareWithCustomSkipper benchmarks the middleware with a custom skipper function
func BenchmarkMiddlewareWithCustomSkipper(b *testing.B) {
	logger := setupBenchmarkLogger(b)
	router := setupBenchmarkRouter(b, logger, ZapConfig{
		Skipper: func(c *echo.Context) bool {
			// Skip logging for GET requests to /ping
			return c.Request().Method == http.MethodGet && c.Path() == "/ping"
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", http.NoBody)
	w := httptest.NewRecorder()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		router.ServeHTTP(w, req)
	}
}
