package vm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestInstantQueryParsesVector(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/query" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
            "status":"success",
            "data":{"resultType":"vector","result":[
                {"metric":{"__name__":"sentinel_validator_online","validator":"node-1"},
                 "value":[1714039200,"1"]}]}
        }`))
	}))
	defer srv.Close()

	c := New(srv.URL)
	res, err := c.Instant(context.Background(), "sentinel_validator_online", time.Now())
	if err != nil {
		t.Fatalf("Instant: %v", err)
	}
	if len(res) != 1 || res[0].Labels["validator"] != "node-1" || res[0].Value != 1.0 {
		t.Errorf("got %+v", res)
	}
}

func TestRangeQueryParsesMatrix(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[
            {"metric":{"validator":"node-1"},"values":[[1714039200,"100"],[1714039203,"101"]]}]}}`))
	}))
	defer srv.Close()
	c := New(srv.URL)
	res, err := c.Range(context.Background(), "x", time.Now().Add(-time.Hour), time.Now(), 3*time.Second)
	if err != nil {
		t.Fatalf("Range: %v", err)
	}
	if len(res) != 1 || len(res[0].Values) != 2 {
		t.Fatalf("got %+v", res)
	}
}

func TestErrorOnNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()
	c := New(srv.URL)
	if _, err := c.Instant(context.Background(), "x", time.Now()); err == nil {
		t.Error("expected error on 500")
	}
}
