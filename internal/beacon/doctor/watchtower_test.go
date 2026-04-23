package doctor_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aeddi/gno-watchtower/internal/beacon/doctor"
)

func TestCheckWatchtower_HealthOK_Green(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	got := doctor.CheckWatchtower(context.Background(), srv.URL)
	if got.Status != doctor.StatusGreen {
		t.Errorf("want GREEN, got %s: %s", got.Status, got.Detail)
	}
}

func TestCheckWatchtower_HealthReturnsNon200_Red(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	got := doctor.CheckWatchtower(context.Background(), srv.URL)
	if got.Status != doctor.StatusRed {
		t.Errorf("want RED, got %s: %s", got.Status, got.Detail)
	}
}

func TestCheckWatchtower_Unreachable_Red(t *testing.T) {
	got := doctor.CheckWatchtower(context.Background(), "http://127.0.0.1:19999")
	if got.Status != doctor.StatusRed {
		t.Errorf("want RED, got %s: %s", got.Status, got.Detail)
	}
}

func TestCheckWatchtower_Placeholder_Orange(t *testing.T) {
	got := doctor.CheckWatchtower(context.Background(), "<watchtower-url>")
	if got.Status != doctor.StatusOrange {
		t.Errorf("want ORANGE for placeholder URL, got %s: %s", got.Status, got.Detail)
	}
}
