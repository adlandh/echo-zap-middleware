package echozapmiddleware

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	contextlogger "github.com/adlandh/context-logger"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/stretchr/testify/suite"
	"go.uber.org/zap"
)

type contextKey string

func (c contextKey) String() string {
	return string(c)
}

func (c contextKey) Saver(e echo.Context, id string) {
	ctx := context.WithValue(e.Request().Context(), c, id)
	e.SetRequest(e.Request().WithContext(ctx))
}

var requestID = contextKey("request_id_from_context")

type MemorySink struct {
	*bytes.Buffer
}

// Implement Close and Sync as no-ops to satisfy the interface. The Write
// method is provided by the embedded buffer.

func (*MemorySink) Close() error { return nil }
func (*MemorySink) Sync() error  { return nil }

type MiddlewareTestSuite struct {
	suite.Suite
	sink      *MemorySink
	router    *echo.Echo
	logger    *zap.Logger
	ctxLogger *contextlogger.ContextLogger
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
	s.ctxLogger = contextlogger.WithContext(s.logger, contextlogger.WithValueExtractor(requestID))
}

func (s *MiddlewareTestSuite) SetupTest() {
	s.sink.Reset()
	s.router = echo.New()
	s.router.Use(middleware.RequestID())
}

func (s *MiddlewareTestSuite) TearDownTest() {
	s.Contains(s.sink.String(), "GET")
	s.Contains(s.sink.String(), "/ping")
	s.Contains(s.sink.String(), "request_id")
}

func (s *MiddlewareTestSuite) TestWithExcludedPath() {
	s.Run("exclude ping from resp", func() {
		rx := regexp.MustCompile("^/ping/121")
		s.sink.Reset()
		s.router = echo.New()
		s.router.Use(middleware.RequestID())
		s.router.Use(Middleware(s.logger, ZapConfig{
			IsBodyDump: true,
			BodySkipper: func(c echo.Context) (skipReq, skipResp bool) {
				if rx.MatchString(c.Request().URL.Path) {
					return false, true
				}

				return
			},
		}))
		s.router.GET("/ping/:id", func(c echo.Context) error {
			return c.String(http.StatusOK, "ok")
		})
		r := httptest.NewRequest("GET", "/ping/121?sdsdds=1212", strings.NewReader("test"))
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, r)

		response := w.Result()
		s.Equal(http.StatusOK, response.StatusCode)
		s.Contains(s.sink.String(), "\"resp.body\": \"[excluded]\"")
		s.Contains(s.sink.String(), "\"req.body\": \"test\"")
	})

	s.Run("exclude ping from req", func() {
		s.sink.Reset()
		s.router = echo.New()
		s.router.Use(middleware.RequestID())
		s.router.Use(Middleware(s.logger, ZapConfig{
			IsBodyDump: true,
			BodySkipper: func(c echo.Context) (skipReq, skipResp bool) {
				if c.Path() == "/ping/:id" {
					return true, false
				}

				return
			},
		}))
		s.router.GET("/ping/:id", func(c echo.Context) error {
			return c.String(http.StatusOK, "ok")
		})
		r := httptest.NewRequest("GET", "/ping/123", strings.NewReader("test"))
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, r)

		response := w.Result()
		s.Equal(http.StatusOK, response.StatusCode)
		s.Contains(s.sink.String(), "\"resp.body\": \"ok\"")
		s.Contains(s.sink.String(), "\"req.body\": \"[excluded]\"")
	})

	s.Run("exclude gzip from req and resp", func() {
		s.sink.Reset()
		s.router = echo.New()
		s.router.Use(middleware.RequestID())
		s.router.Use(Middleware(s.logger, ZapConfig{
			IsBodyDump: true,
			BodySkipper: func(c echo.Context) (bool, bool) {
				if c.Request().Header.Get("Content-Encoding") == "gzip" {
					return true, true
				}
				return false, false
			},
		}))
		s.router.GET("/ping/:id", func(c echo.Context) error {
			return c.String(http.StatusOK, "ok")
		})
		r := httptest.NewRequest("GET", "/ping/121?sdsdds=1212", strings.NewReader("test"))
		r.Header.Set("Content-Encoding", "gzip")
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, r)

		response := w.Result()
		s.Equal(http.StatusOK, response.StatusCode)
		s.Contains(s.sink.String(), "\"resp.body\": \"[excluded]\"")
		s.Contains(s.sink.String(), "\"req.body\": \"[excluded]\"")
	})

}

func (s *MiddlewareTestSuite) TestWithNoBodyNoHeaders() {
	s.router.Use(Middleware(s.logger))
	s.router.GET("/ping", func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})
	r := httptest.NewRequest("GET", "/ping", http.NoBody)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, r)

	response := w.Result()
	s.Equal(http.StatusOK, response.StatusCode)
	s.NotContains(s.sink.String(), "body")
	s.NotContains(s.sink.String(), "headers")
	s.NotContains(s.sink.String(), "request_id_from_context")
}

func (s *MiddlewareTestSuite) TestWithSilentHandler() {
	s.router.Use(Middleware(s.logger))
	s.router.GET("/ping", func(_ echo.Context) error {
		// return nothing as response
		return nil
	})
	r := httptest.NewRequest("GET", "/ping", http.NoBody)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, r)

	response := w.Result()
	s.Equal(http.StatusOK, response.StatusCode)
	s.Contains(s.sink.String(), "WARN")
	s.Contains(s.sink.String(), "Response not committed")
}

func (s *MiddlewareTestSuite) TestWithClientCanceledContext() {
	s.router.Use(Middleware(s.logger))
	s.router.GET("/ping", func(_ echo.Context) error {
		// add delay to make sure the request is canceled
		time.Sleep(10 * time.Millisecond)
		return nil
	})
	r := httptest.NewRequest("GET", "/ping", http.NoBody)
	ctx, cancel := context.WithCancel(r.Context())
	cancel() // cancel context immediately
	r = r.WithContext(ctx)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, r)

	response := w.Result()
	s.Equal(http.StatusOK, response.StatusCode)
	s.Contains(s.sink.String(), "WARN")
	s.Contains(s.sink.String(), "Response not committed")
	s.Contains(s.sink.String(), "context canceled")
}

func (s *MiddlewareTestSuite) TestWithBodyAndHeaders() {
	s.router.Use(Middleware(s.logger, ZapConfig{
		AreHeadersDump: true,
		IsBodyDump:     true,
	}))
	s.router.GET("/ping", func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})
	r := httptest.NewRequest("GET", "/ping", http.NoBody)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, r)

	response := w.Result()
	s.Equal(http.StatusOK, response.StatusCode)
	s.Contains(s.sink.String(), "req.body")
	s.Contains(s.sink.String(), "req.headers")
	s.Contains(s.sink.String(), "resp.body")
	s.Contains(s.sink.String(), "resp.headers")
	s.NotContains(s.sink.String(), "trace_id")
	s.NotContains(s.sink.String(), "span_id")
}

func (s *MiddlewareTestSuite) TestWithBodyAndHeadersWithContextLogger() {
	s.router.Use(middleware.RequestIDWithConfig(middleware.RequestIDConfig{
		RequestIDHandler: requestID.Saver,
	}))
	s.router.Use(MiddlewareWithContextLogger(s.ctxLogger))
	s.router.GET("/ping", func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})
	r := httptest.NewRequest("GET", "/ping", http.NoBody)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, r)

	response := w.Result()
	s.Equal(http.StatusOK, response.StatusCode)
	s.NotContains(s.sink.String(), "body")
	s.NotContains(s.sink.String(), "headers")
	s.Contains(s.sink.String(), "request_id_from_context")
}

func TestMiddleware(t *testing.T) {
	suite.Run(t, new(MiddlewareTestSuite))
}
