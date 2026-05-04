package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"os"
	"time"

	echootel "github.com/labstack/echo-opentelemetry"
	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

type InterRequest struct {
	log *slog.Logger
}

func (ir *InterRequest) handlePost(c *echo.Context) error {
	// The client certificate is available in the TLS state of the request
	if c.Request().TLS != nil && len(c.Request().TLS.PeerCertificates) > 0 {
		body, err := io.ReadAll(c.Request().Body)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{
				"error": "Failed to read body",
			})
		}

		// Example using gjson:
		// email := gjson.Get(string(body), "user.contact.email").String()

		traceRequest(c, time.Now().String())

		ir.log.Info(string(body))

		return c.JSON(http.StatusOK, map[string]string{
			"raw_length": fmt.Sprintf("%d", len(body)),
			"raw":        string(body),
		})
	}

	return c.String(http.StatusUnauthorized, "No client certificate provided")
}

func NewLogger() (*slog.Logger, error) {
	// Open file for writing, creating it if it doesn't exist
	file, err := os.OpenFile("./requests.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	// Create a logger that writes JSON to the file
	// logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	logger := slog.New(slog.NewTextHandler(file, nil))
	logger.Info("assa")
	return logger, nil
}

func NewTracerProvider(logger *slog.Logger) (*sdktrace.TracerProvider, error) {
	exporter, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
	if err != nil {
		logger.Error("Failed to initialize otel tracer", "error", err)
		return nil, err
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithBatcher(exporter),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))

	return tp, nil
}

func NewServer(logger *slog.Logger) *echo.Echo {
	// Echo instance
	e := echo.New()

	// Middleware
	e.Use(echootel.NewMiddleware("app.example.com"))
	// e.Use(slogecho.New(logger))
	e.Use(middleware.RequestLogger()) // use the RequestLogger middleware with slog logger
	e.Use(middleware.Recover())       // recover panics as errors for proper error handling

	// Routes
	ir := &InterRequest{log: logger}

	e.POST("/inter", ir.handlePost)

	// Configure TLS to request client certificates
	tlsConfig := &tls.Config{
		ClientAuth: tls.RequestClientCert, // Use RequireAndVerifyClientCert to enforce it
	}

	sc := echo.StartConfig{
		Address:   ":8443",
		TLSConfig: tlsConfig,
	}

	if err := sc.StartTLS(context.Background(), e, "server.cert", "server.key"); err != nil {
		slog.Error("failed to start server", "error", err)
	}

	return e
}

func main() {
	logger, err := NewLogger()
	logger.Info("llll")
	if err != nil {
		log.Fatal(err)
		return
	}

	tp, err := NewTracerProvider(logger)

	if err != nil {
		log.Fatal(err)
		return
	}

	defer func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			logger.Error("Failed to shutdown tracer provider", "error", err)
		}
	}()

	NewServer(logger)
}

func traceRequest(c *echo.Context, id string) (string, error) {
	tp, err := echo.ContextGet[trace.Tracer](c, echootel.TracerKey)
	if err != nil {
		return "", err
	}

	_, span := tp.Start(c.Request().Context(), "Request", trace.WithAttributes(attribute.String("id", id)))
	defer span.End()

	return "unknown", nil
}
