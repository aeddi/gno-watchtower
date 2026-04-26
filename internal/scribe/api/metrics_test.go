package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aeddi/gno-watchtower/internal/scribe/scribemetrics"
)

func TestMetricsEndpoint(t *testing.T) {
	srv := New(Deps{Metrics: scribemetrics.New(), ClusterID: "c1"})
	rr := httptest.NewRecorder()
	srv.http().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rr.Code != 200 {
		t.Fatalf("status = %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "scribe_") {
		t.Error("no scribe_ metrics in output")
	}
}
