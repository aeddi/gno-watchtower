package doctor_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aeddi/gno-watchtower/internal/beacon/doctor"
)

func TestCheckRPC_StatusOK_Green(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/status" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	got := doctor.CheckRPC(context.Background(), srv.URL)
	if got.Status != doctor.StatusGreen {
		t.Errorf("want GREEN, got %s: %s", got.Status, got.Detail)
	}
}

func TestCheckRPC_Unreachable_Red(t *testing.T) {
	got := doctor.CheckRPC(context.Background(), "http://127.0.0.1:19999")
	if got.Status != doctor.StatusRed {
		t.Errorf("want RED, got %s: %s", got.Status, got.Detail)
	}
}

func TestCheckRPC_Empty_Red(t *testing.T) {
	got := doctor.CheckRPC(context.Background(), "")
	if got.Status != doctor.StatusRed {
		t.Errorf("want RED for empty URL, got %s: %s", got.Status, got.Detail)
	}
}
