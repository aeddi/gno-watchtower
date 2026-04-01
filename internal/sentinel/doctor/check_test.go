package doctor_test

import (
	"testing"

	"github.com/gnolang/val-companion/internal/sentinel/doctor"
	"github.com/gnolang/val-companion/internal/sentinel/metadata"
	"github.com/gnolang/val-companion/internal/sentinel/resources"
)

func TestCheckResult_Fields(t *testing.T) {
	r := doctor.CheckResult{Name: "foo", Status: doctor.StatusGreen, Detail: "ok"}
	if r.Status != doctor.StatusGreen {
		t.Fatalf("expected GREEN, got %s", r.Status)
	}
}

func TestExports_MetadataAndResources(t *testing.T) {
	// Verify exported symbols compile (compile error = RED).
	_ = metadata.ConfigKeys
	_ = metadata.SHA256File
	_ = metadata.RunCmd
	_ = metadata.ReadConfigKey
	_ = resources.ContainerStats
}
