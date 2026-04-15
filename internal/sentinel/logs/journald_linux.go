// internal/sentinel/logs/journald_linux.go
package logs

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/coreos/go-systemd/v22/sdjournal"
)

// JournaldSource tails logs from a systemd journal unit.
type JournaldSource struct {
	unit string
	log  *slog.Logger
}

// NewJournaldSource creates a JournaldSource for the named systemd unit.
// The .service suffix is accepted but stripped: "gnoland" and "gnoland.service" are equivalent.
func NewJournaldSource(unit string, log *slog.Logger) *JournaldSource {
	return &JournaldSource{
		unit: strings.TrimSuffix(unit, ".service"),
		log:  log.With("component", "journald_log_source", "unit", unit),
	}
}

// Tail streams log entries from the journal until ctx is cancelled.
// Only entries added after Tail is called are emitted (SeekTail is used at startup).
// Each entry's MESSAGE field is expected to be a JSON object from gnoland.
func (s *JournaldSource) Tail(ctx context.Context, out chan<- LogLine) error {
	j, err := sdjournal.NewJournal()
	if err != nil {
		return fmt.Errorf("open journal: %w", err)
	}
	defer j.Close()

	if err := j.AddMatch("_SYSTEMD_UNIT=" + s.unit + ".service"); err != nil {
		return fmt.Errorf("add journal match: %w", err)
	}
	if err := j.SeekTail(); err != nil {
		return fmt.Errorf("seek journal tail: %w", err)
	}

	consecutiveTransformed := 0
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Wait up to 1s for new entries to arrive.
		// j.Wait returns a JournalWaitResult (int), not (result, error).
		if r := j.Wait(time.Second); r < 0 {
			return fmt.Errorf("journal wait: error code %d", r)
		}

		// Drain all newly available entries.
		for {
			n, err := j.Next()
			if err != nil {
				return fmt.Errorf("journal next: %w", err)
			}
			if n == 0 {
				break // no more entries in this batch
			}

			entry, err := j.GetEntry()
			if err != nil {
				return fmt.Errorf("journal entry: %w", err)
			}

			msg, ok := entry.Fields[sdjournal.SD_JOURNAL_FIELD_MESSAGE]
			if !ok {
				continue
			}

			normalized, transformed := NormalizeLogLine([]byte(msg))
			if transformed {
				consecutiveTransformed++
				if consecutiveTransformed == consecutiveTransformWarnThreshold+1 {
					s.log.Warn("more than 30 consecutive non-JSON log lines were auto-transformed; add --log-format=json to gnoland")
					select {
					case out <- syntheticWarnLine():
					case <-ctx.Done():
						return ctx.Err()
					}
				}
			} else {
				consecutiveTransformed = 0
			}
			level := ParseLevel(normalized)
			select {
			case out <- LogLine{Level: level, Raw: normalized}:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
}
