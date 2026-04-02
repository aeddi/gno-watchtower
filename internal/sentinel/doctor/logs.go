// internal/sentinel/doctor/logs.go
package doctor

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gnolang/val-companion/internal/sentinel/config"
	"github.com/gnolang/val-companion/internal/sentinel/logs"
	"github.com/gnolang/val-companion/pkg/levels"
)

// checkDuration is how long CheckLogs listens for log lines.
const checkDuration = 3 * time.Second

// CheckLogs tails the provided Source for up to 3 seconds and checks:
// - at least one line received
// - all received lines are valid JSON
// - at least one line is at minLevel or above
//
// src must already be constructed; CheckLogs drives it directly.
func CheckLogs(ctx context.Context, src logs.Source, cfg config.LogsConfig, minLevel string) CheckResult {
	const name = "Logs"

	ctx, cancel := context.WithTimeout(ctx, checkDuration)
	defer cancel()

	lineCh := make(chan logs.LogLine, 256)
	go func() {
		src.Tail(ctx, lineCh) //nolint:errcheck
	}()

	var total, invalidJSON, atLevel int

	for {
		select {
		case line := <-lineCh:
			total++
			if !json.Valid(line.Raw) {
				invalidJSON++
			}
			if levels.Rank(line.Level) >= levels.Rank(minLevel) {
				atLevel++
			}
		case <-ctx.Done():
			if total == 0 {
				return CheckResult{Name: name, Status: StatusRed, Detail: fmt.Sprintf("no lines received in %s", checkDuration)}
			}
			if invalidJSON > 0 {
				return CheckResult{Name: name, Status: StatusRed, Detail: fmt.Sprintf("%d/%d lines are not valid JSON", invalidJSON, total)}
			}
			if atLevel == 0 {
				return CheckResult{
					Name:   name,
					Status: StatusRed,
					Detail: fmt.Sprintf("%d lines received but none at %s or above", total, minLevel),
				}
			}
			return CheckResult{
				Name:   name,
				Status: StatusGreen,
				Detail: fmt.Sprintf("%d valid JSON lines, %d at %s+ in %s", total, atLevel, minLevel, checkDuration),
			}
		}
	}
}
