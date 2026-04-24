package doctor_test

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/aeddi/gno-watchtower/internal/beacon/config"
	"github.com/aeddi/gno-watchtower/internal/beacon/doctor"
	pkgnoise "github.com/aeddi/gno-watchtower/pkg/noise"
)

func TestCheckKeypair_KeysPresent_Green(t *testing.T) {
	dir := t.TempDir()
	kp, err := pkgnoise.GenerateKeypair()
	if err != nil {
		t.Fatal(err)
	}
	if err := pkgnoise.WriteKeypair(dir, kp); err != nil {
		t.Fatal(err)
	}
	got := doctor.CheckKeypair(config.BeaconConfig{KeysDir: dir})
	if got.Status != doctor.StatusGreen {
		t.Errorf("want GREEN, got %s: %s", got.Status, got.Detail)
	}
}

func TestCheckKeypair_Placeholder_Orange(t *testing.T) {
	got := doctor.CheckKeypair(config.BeaconConfig{KeysDir: "<path-to-beacon-keys-dir>"})
	if got.Status != doctor.StatusOrange {
		t.Errorf("want ORANGE, got %s: %s", got.Status, got.Detail)
	}
}

func TestCheckKeypair_MissingDir_Red(t *testing.T) {
	got := doctor.CheckKeypair(config.BeaconConfig{KeysDir: "/nonexistent/beacon-keys"})
	if got.Status != doctor.StatusRed {
		t.Errorf("want RED, got %s: %s", got.Status, got.Detail)
	}
}

func TestCheckKeypair_PartialKeys_Red(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "privkey"), []byte("garbage"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := doctor.CheckKeypair(config.BeaconConfig{KeysDir: dir})
	if got.Status != doctor.StatusRed {
		t.Errorf("want RED for missing pubkey, got %s: %s", got.Status, got.Detail)
	}
}

func TestCheckAuthorizedKeys_Empty_Orange(t *testing.T) {
	got := doctor.CheckAuthorizedKeys(config.BeaconConfig{})
	if got.Status != doctor.StatusOrange {
		t.Errorf("want ORANGE for empty allowlist, got %s: %s", got.Status, got.Detail)
	}
}

func TestCheckAuthorizedKeys_AllParseable_Green(t *testing.T) {
	kp1, _ := pkgnoise.GenerateKeypair()
	kp2, _ := pkgnoise.GenerateKeypair()
	got := doctor.CheckAuthorizedKeys(config.BeaconConfig{
		AuthorizedKeys: []string{hex.EncodeToString(kp1.Public), hex.EncodeToString(kp2.Public)},
	})
	if got.Status != doctor.StatusGreen {
		t.Errorf("want GREEN, got %s: %s", got.Status, got.Detail)
	}
}
