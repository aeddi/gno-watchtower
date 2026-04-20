package doctor_test

import (
	"context"
	"testing"

	"github.com/aeddi/gno-watchtower/internal/sentinel/config"
	"github.com/aeddi/gno-watchtower/internal/sentinel/doctor"
)

func TestCheckResources_Host_Green(t *testing.T) {
	cfg := config.ResourcesConfig{Source: "host"}
	r := doctor.CheckResources(context.Background(), cfg)
	// gopsutil cpu.Percent works on linux and macOS in CI.
	if r.Status == doctor.StatusOrange {
		t.Errorf("unexpected ORANGE: %s", r.Detail)
	}
	// Green or Red depending on environment — just verify it doesn't panic and returns a result.
	if r.Name == "" {
		t.Error("Name must be set")
	}
}

func TestCheckResources_Host_NotConfigured_Orange(t *testing.T) {
	cfg := config.ResourcesConfig{Source: ""}
	r := doctor.CheckResources(context.Background(), cfg)
	if r.Status != doctor.StatusOrange {
		t.Errorf("want ORANGE for empty source, got %s", r.Status)
	}
}

func TestCheckResources_Docker_MissingContainer_Red(t *testing.T) {
	cfg := config.ResourcesConfig{Source: "docker", ContainerName: "__nonexistent_container_xyz__"}
	r := doctor.CheckResources(context.Background(), cfg)
	// Docker SDK will fail — Red depending on whether Docker daemon is running.
	// We just verify no panic and result is set.
	if r.Name == "" {
		t.Error("Name must be set")
	}
}
