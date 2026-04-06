package app_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/gnolang/val-companion/internal/sentinel/app"
	"github.com/gnolang/val-companion/internal/sentinel/config"
	pkglogger "github.com/gnolang/val-companion/pkg/logger"
)

func TestRun_HealthEndpoint_Responds(t *testing.T) {
	cfg := &config.Config{
		Health: config.HealthConfig{Enabled: true, ListenAddr: "127.0.0.1:19876"},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go app.Run(ctx, cfg, pkglogger.Noop())
	time.Sleep(100 * time.Millisecond) // wait for server to start

	resp, err := http.Get("http://127.0.0.1:19876/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET /health: got %d, want 200", resp.StatusCode)
	}
}
