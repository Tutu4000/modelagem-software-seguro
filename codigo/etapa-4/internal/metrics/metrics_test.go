package metrics

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// An ID in the URL must not create a series per ID: the label is the
// registered route.
func TestMiddlewareUsesTheRegisteredRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	e := gin.New()
	e.Use(Middleware())
	e.GET("/things/:id", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	before := testutil.CollectAndCount(requests)
	for _, path := range []string{"/things/1", "/things/2", "/things/3", "/nowhere/1", "/nowhere/2"} {
		e.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
	}
	if got := testutil.ToFloat64(requests.WithLabelValues("/things/:id", "GET", "204")); got != 3 {
		t.Errorf("requests on the route = %v, want 3", got)
	}
	if got := testutil.ToFloat64(requests.WithLabelValues(Unmatched, "GET", "404")); got != 2 {
		t.Errorf("unmatched requests = %v, want 2", got)
	}
	if n := testutil.CollectAndCount(requests) - before; n != 2 {
		t.Errorf("%d new series, want 2", n)
	}
}

func TestAuditEventCounter(t *testing.T) {
	AuditEvent("authz.denied", "denied")
	AuditEvent("authz.denied", "denied")
	if got := testutil.ToFloat64(auditEvents.WithLabelValues("authz.denied", "denied")); got != 2 {
		t.Errorf("authz.denied = %v, want 2", got)
	}
}
