package logs_test

import (
	"testing"

	"github.com/gnolang/val-companion/internal/sentinel/logs"
)

func TestNewDockerSource_ReturnsNonNil(t *testing.T) {
	s := logs.NewDockerSource("gnoland")
	if s == nil {
		t.Fatal("expected non-nil DockerSource")
	}
}
