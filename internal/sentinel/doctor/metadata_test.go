package doctor_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gnolang/val-companion/internal/sentinel/config"
	"github.com/gnolang/val-companion/internal/sentinel/doctor"
)

func TestCheckMetadataBinary_Path_Green(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "gnoland")
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("fake binary")
	f.Close()

	cfg := config.MetadataConfig{BinaryPath: f.Name()}
	r := doctor.CheckMetadataBinary(cfg)
	if r.Status != doctor.StatusGreen {
		t.Errorf("want GREEN, got %s: %s", r.Status, r.Detail)
	}
	if r.Detail == "" {
		t.Error("detail must be non-empty")
	}
}

func TestCheckMetadataBinary_MissingFile_Red(t *testing.T) {
	cfg := config.MetadataConfig{BinaryPath: "/nonexistent/gnoland"}
	r := doctor.CheckMetadataBinary(cfg)
	if r.Status != doctor.StatusRed {
		t.Errorf("want RED, got %s", r.Status)
	}
}

func TestCheckMetadataBinary_NotConfigured_Orange(t *testing.T) {
	cfg := config.MetadataConfig{}
	r := doctor.CheckMetadataBinary(cfg)
	if r.Status != doctor.StatusOrange {
		t.Errorf("want ORANGE, got %s", r.Status)
	}
}

func TestCheckMetadataGenesis_Path_Green(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "genesis.json")
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(`{"chain_id":"test"}`)
	f.Close()

	cfg := config.MetadataConfig{GenesisPath: f.Name()}
	r := doctor.CheckMetadataGenesis(cfg)
	if r.Status != doctor.StatusGreen {
		t.Errorf("want GREEN, got %s: %s", r.Status, r.Detail)
	}
}

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

func TestCheckMetadataConflicts_PathAndCmd_Red(t *testing.T) {
	cfg := config.MetadataConfig{
		BinaryPath:        "/usr/local/bin/gnoland",
		BinaryChecksumCmd: "sha256sum /usr/local/bin/gnoland",
	}
	r := doctor.CheckMetadataConflicts(cfg)
	if r.Status != doctor.StatusRed {
		t.Errorf("want RED on conflict, got %s: %s", r.Status, r.Detail)
	}
}

func TestCheckMetadataConflicts_NoConflict_Green(t *testing.T) {
	cfg := config.MetadataConfig{
		BinaryPath:  "/usr/local/bin/gnoland",
		GenesisPath: "/etc/gnoland/genesis.json",
	}
	r := doctor.CheckMetadataConflicts(cfg)
	if r.Status != doctor.StatusGreen {
		t.Errorf("want GREEN, got %s: %s", r.Status, r.Detail)
	}
}
