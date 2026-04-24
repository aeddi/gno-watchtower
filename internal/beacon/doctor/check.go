// Package doctor provides the `beacon doctor` subcommand: a set of
// read-only checks that surface misconfiguration before a live run.
//
// Unlike the sentinel doctor, the beacon does not hold a bearer token — it
// forwards the sentinel's token unchanged to the upstream watchtower. So the
// checks here are scoped to what the beacon alone controls: upstream
// reachability, Noise keypair presence, sentinel allowlist parseability, local
// gnoland RPC reachability, and gnoland config access for metadata
// augmentation.
package doctor

// Status represents the outcome of a doctor check.
type Status string

const (
	StatusGreen  Status = "GREEN"  // working correctly
	StatusRed    Status = "RED"    // enabled but failing
	StatusOrange Status = "ORANGE" // disabled in config
)

// CheckResult holds the outcome of a single doctor check.
type CheckResult struct {
	Name   string
	Status Status
	Detail string
}
