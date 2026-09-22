package gostatsd

import (
	"context"
	"net/http"
	"slices"
)

const (
	DefaultMetricSuccess = "server.request.success"
	DefaultMetric4xx     = "server.request.error.4xx"
	DefaultMetric5xx     = "server.request.error.5xx"
)

// MiddlewareConfig configures the metric names and ignored paths for Middleware.
// Zero-value metric name fields fall back to their Default* constants.
type MiddlewareConfig struct {
	MetricSuccess string   // default: DefaultMetricSuccess
	Metric4xx     string   // default: DefaultMetric4xx
	Metric5xx     string   // default: DefaultMetric5xx
	IgnoredPaths  []string
}

func (cfg MiddlewareConfig) withDefaults() MiddlewareConfig {
	if cfg.MetricSuccess == "" {
		cfg.MetricSuccess = DefaultMetricSuccess
	}
	if cfg.Metric4xx == "" {
		cfg.Metric4xx = DefaultMetric4xx
	}
	if cfg.Metric5xx == "" {
		cfg.Metric5xx = DefaultMetric5xx
	}
	return cfg
}

type contextKey struct{}

type hint struct {
	skipErrorMetric bool
}

// SkipErrorMetric tells Middleware to count the current request as the
// configured success metric regardless of its HTTP status code. Call it
// from a handler to suppress error counting for expected non-2xx responses
// (e.g. a form that is closed — a business outcome, not a server error).
// It is a no-op when called outside of a Middleware-wrapped handler.
func SkipErrorMetric(ctx context.Context) {
	if h, ok := ctx.Value(contextKey{}).(*hint); ok {
		h.skipErrorMetric = true
	}
}

// Middleware returns an HTTP handler that increments StatsD counters based on the response status code.
// The metric names and ignored paths are controlled by cfg; zero-value metric fields use their Default* values.
//
// A handler may call SkipErrorMetric(r.Context()) to suppress error counting
// for an expected non-2xx response; the request is then counted as success.
//
// Safe to call on a nil *Client — returns next unchanged.
func (c *Client) Middleware(next http.Handler, cfg MiddlewareConfig) http.Handler {
	if c == nil {
		return next
	}
	cfg = cfg.withDefaults()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := &hint{}
		rw := &statusWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(rw, r.WithContext(context.WithValue(r.Context(), contextKey{}, h)))

		if slices.Contains(cfg.IgnoredPaths, r.URL.Path) {
			return
		}

		if h.skipErrorMetric {
			c.Increment(cfg.MetricSuccess)
			return
		}

		// 1xx informational responses produce no metric.
		switch {
		case rw.statusCode >= 200 && rw.statusCode < 400:
			c.Increment(cfg.MetricSuccess)
		case rw.statusCode >= 400 && rw.statusCode < 500:
			c.Increment(cfg.Metric4xx)
		case rw.statusCode >= 500:
			c.Increment(cfg.Metric5xx)
		}
	})
}

type statusWriter struct {
	http.ResponseWriter
	statusCode int
}

func (w *statusWriter) WriteHeader(code int) {
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}
