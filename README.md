# echo-zap-middleware

Echo Zap Logger middleware

## Usage:

```shell
go get github.com/adlandh/echo-zap-middleware
```

In your app:

```go
package main

import (
	"net/http"

	echo_zap_middleware "github.com/adlandh/echo-zap-middleware"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

func main() {
	// Create zap logger
	logger, err := zap.NewDevelopment()
	if err != nil {
		panic(err)
	}

	// Then create your app
	app := echo.New()

	// Add middleware
	app.Use(echo_zap_middleware.Middleware(
		logger,
		echo_zap_middleware.ZapConfig{
			// if you would like to save your request or response headers as tags, set AreHeadersDump to true
			AreHeadersDump: true,
			// if you would like to save your request or response body as tags, set IsBodyDump to true
			IsBodyDump: true,
			// No dump for /pong
			DumpNoResponseBodyForPaths: []string{"/pong"},
			// No dump for gzip
			BodySkipper: func(c echo.Context) bool {
				if c.Request().Header.Get("Content-Encoding") == "gzip" {
					return true
				}
				return false
			},
		}))

	// Add some endpoints
	app.GET("/ping", func(c echo.Context) error {
		return c.String(http.StatusOK, "Hello, World, from Ping!")
	})

	app.GET("/pong", func(c echo.Context) error {
		return c.String(http.StatusOK, "Hello, World, from Pong!")
	})

	// And run it
	app.Logger.Fatal(app.Start(":3000"))
}


```