package kinds_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aeddi/gno-watchtower/internal/scribe/normalizer"
	"github.com/aeddi/gno-watchtower/internal/scribe/sources/loki"
	"github.com/aeddi/gno-watchtower/internal/scribe/types"
)

func runLogHandler(t *testing.T, h normalizer.Handler, file string) []types.Op {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", file))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	o := normalizer.Observation{
		Lane: normalizer.LaneLogs, IngestTime: time.Now().UTC(),
		LogQuery: `{validator="node-1"}`,
		LogEntry: &loki.TailEntry{
			Stream: loki.Stream{Labels: map[string]string{"validator": "node-1", "module": "consensus"}},
			Time:   time.Now().UTC(),
			Line:   string(body),
		},
	}
	return h.Handle(context.Background(), o)
}
