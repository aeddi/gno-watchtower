package doctor_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aeddi/gno-watchtower/internal/sentinel/config"
	"github.com/aeddi/gno-watchtower/internal/sentinel/doctor"
)

func TestCheckMetadataConfig_Path_Green(t *testing.T) {
	toml := "[p2p]\nladdr = \"tcp://0.0.0.0:26656\"\n[rpc]\nladdr = \"tcp://0.0.0.0:26657\"\nmoniker = \"mynode\"\n"
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.toml")
	if err := os.WriteFile(cfgPath, []byte(toml), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := config.MetadataConfig{ConfigPath: cfgPath}
	r := doctor.CheckMetadataConfig(cfg)
	if r.Status != doctor.StatusGreen {
		t.Errorf("want GREEN, got %s: %s", r.Status, r.Detail)
	}
}

func TestCheckMetadataConfig_NotConfigured_Orange(t *testing.T) {
	cfg := config.MetadataConfig{}
	r := doctor.CheckMetadataConfig(cfg)
	if r.Status != doctor.StatusOrange {
		t.Errorf("want ORANGE, got %s", r.Status)
	}
}

func TestCheckMetadataConflicts_PathAndCmd_Red(t *testing.T) {
	cfg := config.MetadataConfig{
		ConfigPath:   "/etc/gnoland/config.toml",
		ConfigGetCmd: "gnoland config get %s",
	}
	r := doctor.CheckMetadataConflicts(cfg)
	if r.Status != doctor.StatusRed {
		t.Errorf("want RED on conflict, got %s: %s", r.Status, r.Detail)
	}
}

func TestCheckMetadataConflicts_NoConflict_Green(t *testing.T) {
	cfg := config.MetadataConfig{
		ConfigPath: "/etc/gnoland/config.toml",
	}
	r := doctor.CheckMetadataConflicts(cfg)
	if r.Status != doctor.StatusGreen {
		t.Errorf("want GREEN, got %s: %s", r.Status, r.Detail)
	}
}
