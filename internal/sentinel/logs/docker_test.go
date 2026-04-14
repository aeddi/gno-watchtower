package logs_test

import (
	"testing"

	"github.com/gnolang/val-companion/internal/sentinel/logs"
	"github.com/gnolang/val-companion/pkg/logger"
)

func TestNewDockerSource_ReturnsNonNil(t *testing.T) {
	s := logs.NewDockerSource("gnoland", logger.Noop())
	if s == nil {
		t.Fatal("expected non-nil DockerSource")
	}
}
