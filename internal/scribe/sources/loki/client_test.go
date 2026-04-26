package loki

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestQueryRangeParsesStreams(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"streams","result":[
            {"stream":{"validator":"node-1","module":"consensus"},
             "values":[["1714039200000000000","enterPropose 100/0"],["1714039201000000000","Commit"]]}]}}`))
	}))
	defer srv.Close()

	c := New(srv.URL)
	streams, err := c.QueryRange(context.Background(),
		`{validator="node-1"}`, time.Now().Add(-time.Hour), time.Now(), 100)
	if err != nil {
		t.Fatalf("QueryRange: %v", err)
	}
	if len(streams) != 1 || len(streams[0].Entries) != 2 {
		t.Fatalf("got %+v", streams)
	}
	if streams[0].Labels["validator"] != "node-1" {
		t.Errorf("label not parsed: %+v", streams[0].Labels)
	}
}

func TestQueryRangePropagatesError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	defer srv.Close()
	c := New(srv.URL)
	if _, err := c.QueryRange(context.Background(), `{x="y"}`, time.Now().Add(-time.Minute), time.Now(), 10); err == nil {
		t.Error("expected error")
	}
}
