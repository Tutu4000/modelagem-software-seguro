// Package metrics exposes the operational metrics of a binary to Prometheus
// (C19). The labels are bounded: the registered route, never the raw URL, and
// the audit event name, never the actor or the target. The per-account detail
// the alerts need comes from the audit database.
package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Unmatched is the route label of a request that matched no route.
const Unmatched = "unmatched"

var (
	registry = prometheus.NewRegistry()

	requests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "nugget_http_requests_total",
		Help: "HTTP requests by registered route, method and status.",
	}, []string{"route", "method", "status"})

	duration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "nugget_http_request_duration_seconds",
		Help:    "HTTP request duration by registered route and method.",
		Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10, 30},
	}, []string{"route", "method"})

	auditEvents = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "nugget_audit_events_total",
		Help: "Audit events by name and result.",
	}, []string{"event", "result"})
)

func init() {
	registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		requests, duration, auditEvents,
	)
}

// Middleware counts each request under its registered route.
func Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		route := c.FullPath()
		if route == "" {
			route = Unmatched
		}
		requests.WithLabelValues(route, c.Request.Method, strconv.Itoa(c.Writer.Status())).Inc()
		duration.WithLabelValues(route, c.Request.Method).Observe(time.Since(start).Seconds())
	}
}

// AuditEvent counts one audit event.
func AuditEvent(name, result string) { auditEvents.WithLabelValues(name, result).Inc() }

// Handler serves the metrics, on the internal listener only.
func Handler() http.Handler {
	return promhttp.HandlerFor(registry, promhttp.HandlerOpts{Registry: registry})
}

// Gatherer is for the tests.
func Gatherer() prometheus.Gatherer { return registry }
