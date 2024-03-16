package echozapmiddleware

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	echootelmiddleware "github.com/adlandh/echo-otel-middleware"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/stretchr/testify/suite"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"
)

type MemorySink struct {
	*bytes.Buffer
}

// Implement Close and Sync as no-ops to satisfy the interface. The Write
// method is provided by the embedded buffer.

func (*MemorySink) Close() error { return nil }
func (*MemorySink) Sync() error  { return nil }

type MiddlewareTestSuite struct {
	suite.Suite
	sink   *MemorySink
	router *echo.Echo
	logger *zap.Logger
}

func (s *MiddlewareTestSuite) SetupSuite() {
	s.sink = &MemorySink{new(bytes.Buffer)}
	err := zap.RegisterSink("memory", func(*url.URL) (zap.Sink, error) {
		return s.sink, nil
	})
	s.Require().NoError(err)

	conf := zap.NewDevelopmentConfig()
	// Redirect all messages to the MemorySink.
	conf.OutputPaths = []string{"memory://"}
	s.logger, err = conf.Build()
	s.Require().NoError(err)
}

func (s *MiddlewareTestSuite) SetupTest() {
	s.sink.Reset()
	s.router = echo.New()
	s.router.Use(middleware.RequestID())
}

func (s *MiddlewareTestSuite) TestWithNoBodyNoHeaders() {
	s.router.Use(Middleware(s.logger))
	s.router.GET("/ping", func(c echo.Context) error {
		// Assert we don't have a span on the context.
		span := trace.SpanFromContext(c.Request().Context())
		ok := !span.SpanContext().IsValid()
		s.True(ok)
		return c.String(http.StatusOK, "ok")
	})
	r := httptest.NewRequest("GET", "/ping", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, r)

	response := w.Result()
	s.Equal(http.StatusOK, response.StatusCode)
	s.Contains(s.sink.String(), "GET")
	s.Contains(s.sink.String(), "/ping")
	s.Contains(s.sink.String(), "request_id")
	s.NotContains(s.sink.String(), "body")
	s.NotContains(s.sink.String(), "headers")
	s.NotContains(s.sink.String(), "trace_id")
	s.NotContains(s.sink.String(), "span_id")
}
func (s *MiddlewareTestSuite) TestWithBodyAndHeaders() {
	s.router.Use(MiddlewareWithConfig(s.logger, ZapConfig{
		AreHeadersDump: true,
		IsBodyDump:     true,
	}))
	s.router.GET("/ping", func(c echo.Context) error {
		// Assert we don't have a span on the context.
		span := trace.SpanFromContext(c.Request().Context())
		ok := !span.SpanContext().IsValid()
		s.True(ok)
		return c.String(http.StatusOK, "ok")
	})
	r := httptest.NewRequest("GET", "/ping", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, r)

	response := w.Result()
	s.Equal(http.StatusOK, response.StatusCode)
	s.Contains(s.sink.String(), "GET")
	s.Contains(s.sink.String(), "/ping")
	s.Contains(s.sink.String(), "request_id")
	s.Contains(s.sink.String(), "req.body")
	s.Contains(s.sink.String(), "req.headers")
	s.Contains(s.sink.String(), "resp.body")
	s.Contains(s.sink.String(), "resp.headers")
	s.NotContains(s.sink.String(), "trace_id")
	s.NotContains(s.sink.String(), "span_id")
}

func (s *MiddlewareTestSuite) TestWithOtelMiddleware() {
	provider := noop.NewTracerProvider()
	otel.SetTextMapPropagator(propagation.TraceContext{})
	ctx := context.Background()
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: trace.TraceID{0x01},
		SpanID:  trace.SpanID{0x01},
	})

	r := httptest.NewRequest("GET", "/ping", nil)
	w := httptest.NewRecorder()

	ctx = trace.ContextWithRemoteSpanContext(ctx, sc)
	ctx, _ = provider.Tracer("test").Start(ctx, "test")

	s.router.Use(echootelmiddleware.Middleware()) // should go first
	s.router.Use(MiddlewareWithConfig(s.logger, DefaultZapConfig))
	s.router.GET("/ping", func(c echo.Context) error {
		span := trace.SpanFromContext(c.Request().Context())
		s.Equal(sc.TraceID(), span.SpanContext().TraceID())
		s.Equal(sc.SpanID(), span.SpanContext().SpanID())
		return c.NoContent(http.StatusOK)
	})
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(r.Header))
	s.router.ServeHTTP(w, r)
	response := w.Result()
	s.Equal(http.StatusOK, response.StatusCode)
	s.Contains(s.sink.String(), "GET")
	s.Contains(s.sink.String(), "/ping")
	s.Contains(s.sink.String(), "request_id")
	s.Contains(s.sink.String(), "trace_id")
	s.Contains(s.sink.String(), "span_id")
}

func TestMiddleware(t *testing.T) {
	suite.Run(t, new(MiddlewareTestSuite))
}
