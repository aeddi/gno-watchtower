package scribemetrics

import (
	"strings"
	"testing"
)

func TestMetricsExposed(t *testing.T) {
	r := New()
	r.IngestObservations.WithLabelValues("fast").Inc()
	r.EventsWritten.WithLabelValues("validator.height_advanced").Inc()

	gathered, err := r.Registry.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	if len(gathered) == 0 {
		t.Fatal("no metrics gathered")
	}
	var found bool
	for _, mf := range gathered {
		if strings.HasPrefix(mf.GetName(), "scribe_") {
			found = true
		}
	}
	if !found {
		t.Error("no scribe_ metrics gathered")
	}
}
