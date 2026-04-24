package doctor_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/aeddi/gno-watchtower/internal/beacon/config"
	"github.com/aeddi/gno-watchtower/internal/beacon/doctor"
)

func TestCheckMetadataConfig_Placeholder_Orange(t *testing.T) {
	got := doctor.CheckMetadataConfig(context.Background(), config.MetadataConfig{
		ConfigPath: "<path-to-gnoland-config>",
	})
	if got.Status != doctor.StatusOrange {
		t.Errorf("want ORANGE for placeholder, got %s: %s", got.Status, got.Detail)
	}
}

func TestCheckMetadataConfig_Unconfigured_Orange(t *testing.T) {
	got := doctor.CheckMetadataConfig(context.Background(), config.MetadataConfig{})
	if got.Status != doctor.StatusOrange {
		t.Errorf("want ORANGE for empty metadata, got %s: %s", got.Status, got.Detail)
	}
}

func TestCheckMetadataConfig_FileMissing_Red(t *testing.T) {
	got := doctor.CheckMetadataConfig(context.Background(), config.MetadataConfig{
		ConfigPath: "/nonexistent/gnoland.toml",
	})
	if got.Status != doctor.StatusRed {
		t.Errorf("want RED, got %s: %s", got.Status, got.Detail)
	}
}

func TestCheckMetadataConfig_FileReadable_Green(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gnoland.toml")
	// A minimal config with one of the ConfigKeys present satisfies the
	// key-count reporter without mocking the whole gnoland shape.
	if err := os.WriteFile(path, []byte("moniker = \"test-val\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := doctor.CheckMetadataConfig(context.Background(), config.MetadataConfig{
		ConfigPath: path,
	})
	if got.Status != doctor.StatusGreen {
		t.Errorf("want GREEN, got %s: %s", got.Status, got.Detail)
	}
}

func TestCheckMetadataConfig_CmdFailing_Red(t *testing.T) {
	got := doctor.CheckMetadataConfig(context.Background(), config.MetadataConfig{
		ConfigGetCmd: "/bin/false %s",
	})
	if got.Status != doctor.StatusRed {
		t.Errorf("want RED, got %s: %s", got.Status, got.Detail)
	}
}

func TestCheckMetadataConfig_CmdSucceeding_Green(t *testing.T) {
	got := doctor.CheckMetadataConfig(context.Background(), config.MetadataConfig{
		ConfigGetCmd: "echo stub-%s",
	})
	if got.Status != doctor.StatusGreen {
		t.Errorf("want GREEN, got %s: %s", got.Status, got.Detail)
	}
}
