package scribemetrics

import (
	"strings"
	"testing"
)

func TestAnalysisMetricsRegistered(t *testing.T) {
	r := New()
	if r.AnalysisEvalDuration == nil ||
		r.AnalysisEmissions == nil ||
		r.AnalysisQueueDrops == nil ||
		r.AnalysisPanics == nil ||
		r.AnalysisOpenIncidents == nil ||
		r.AnalysisStoreErrors == nil ||
		r.AnalysisEmitErrors == nil {
		t.Fatalf("analysis metrics not all initialized: %+v", r)
	}
	r.AnalysisEmissions.WithLabelValues("diagnostic.block_missed_v1", "warning", "open").Inc()
	mfs, err := r.Registry.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	found := false
	for _, mf := range mfs {
		if mf.GetName() == "scribe_analysis_emissions_total" {
			found = true
		}
	}
	if !found {
		t.Errorf("scribe_analysis_emissions_total not found in scrape")
	}
}

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
