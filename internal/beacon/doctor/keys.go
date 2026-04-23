package doctor

import (
	"fmt"

	"github.com/aeddi/gno-watchtower/internal/beacon/config"
	pkgnoise "github.com/aeddi/gno-watchtower/pkg/noise"
)

// CheckKeypair verifies that <keys_dir>/privkey and <keys_dir>/pubkey exist,
// are readable, and round-trip through noise.LoadKeypair. A broken keypair
// blocks the Noise listener at startup, so surfacing it here saves an ops
// round-trip.
func CheckKeypair(cfg config.BeaconConfig) CheckResult {
	const name = "Beacon keypair"
	if config.IsPlaceholder(cfg.KeysDir) {
		return CheckResult{Name: name, Status: StatusOrange, Detail: "beacon.keys_dir not configured"}
	}
	if _, err := pkgnoise.LoadKeypair(cfg.KeysDir); err != nil {
		return CheckResult{Name: name, Status: StatusRed, Detail: err.Error()}
	}
	return CheckResult{Name: name, Status: StatusGreen, Detail: cfg.KeysDir}
}

// CheckAuthorizedKeys reports the sentinel allowlist: Orange when empty (any
// Noise peer may connect — confidentiality without authentication), Green when
// non-empty and every entry already parsed during config.Load. Invalid entries
// never reach this check because config.Load rejects them.
func CheckAuthorizedKeys(cfg config.BeaconConfig) CheckResult {
	const name = "Authorized sentinel keys"
	if len(cfg.AuthorizedKeys) == 0 {
		return CheckResult{Name: name, Status: StatusOrange, Detail: "none configured (any sentinel may connect)"}
	}
	return CheckResult{Name: name, Status: StatusGreen, Detail: fmt.Sprintf("%d key(s) configured", len(cfg.AuthorizedKeys))}
}
