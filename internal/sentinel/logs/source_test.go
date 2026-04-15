package logs

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSyntheticWarnLine(t *testing.T) {
	line := syntheticWarnLine()

	if line.Level != "warn" {
		t.Errorf("Level: got %q, want %q", line.Level, "warn")
	}
	if !json.Valid(line.Raw) {
		t.Fatalf("Raw is not valid JSON: %s", line.Raw)
	}
	var parsed struct {
		Level string `json:"level"`
		Msg   string `json:"msg"`
	}
	if err := json.Unmarshal(line.Raw, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed.Level != "warn" {
		t.Errorf("JSON level: got %q, want %q", parsed.Level, "warn")
	}
	if !strings.Contains(parsed.Msg, "[WARN][sentinel]") {
		t.Errorf("msg missing [WARN][sentinel] tag: %q", parsed.Msg)
	}
	if !strings.Contains(parsed.Msg, "--log-format=json") {
		t.Errorf("msg missing --log-format=json hint: %q", parsed.Msg)
	}
}

func TestConsecutiveTransformWarnThreshold(t *testing.T) {
	if consecutiveTransformWarnThreshold != 30 {
		t.Errorf("threshold: got %d, want 30", consecutiveTransformWarnThreshold)
	}
}

// TestConsecutiveTransformLogic simulates the counter logic used in DockerSource
// and JournaldSource without requiring a real Docker/journald connection.
func TestConsecutiveTransformLogic(t *testing.T) {
	threshold := consecutiveTransformWarnThreshold

	type step struct {
		input        string
		wantSynthetic bool // whether a synthetic line should be emitted at this step
	}

	tests := []struct {
		name  string
		steps []step
	}{
		{
			name: "threshold not reached",
			steps: func() []step {
				// 29 consecutive non-JSON lines: one short of the threshold, no synthetic emitted.
				steps := make([]step, threshold-1)
				for i := range steps {
					steps[i] = step{input: "plain text", wantSynthetic: false}
				}
				return steps
			}(),
		},
		{
			name: "threshold reached on 30th line",
			steps: func() []step {
				steps := make([]step, threshold)
				for i := range steps {
					steps[i] = step{input: "plain text", wantSynthetic: i == threshold-1}
				}
				return steps
			}(),
		},
		{
			name: "repeats every 30 lines",
			steps: func() []step {
				// 90 non-JSON lines: synthetic fires at step 29 (30th), 59 (60th), 89 (90th).
				steps := make([]step, threshold*3)
				for i := range steps {
					steps[i] = step{input: "plain text", wantSynthetic: (i+1)%threshold == 0}
				}
				return steps
			}(),
		},
		{
			name: "reset on valid JSON then re-trigger",
			steps: func() []step {
				var steps []step
				// first sequence: triggers at 30th line (index threshold-1)
				for i := 0; i < threshold; i++ {
					steps = append(steps, step{input: "plain text", wantSynthetic: i == threshold-1})
				}
				// valid JSON resets the counter
				steps = append(steps, step{input: `{"level":"info","msg":"ok"}`, wantSynthetic: false})
				// second sequence: triggers again at 30th line
				for i := 0; i < threshold; i++ {
					steps = append(steps, step{input: "plain text", wantSynthetic: i == threshold-1})
				}
				return steps
			}(),
		},
		{
			name: "all valid JSON — no synthetic",
			steps: func() []step {
				steps := make([]step, threshold+10)
				for i := range steps {
					steps[i] = step{input: `{"level":"info","msg":"ok"}`, wantSynthetic: false}
				}
				return steps
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			consecutiveTransformed := 0
			for i, s := range tt.steps {
				_, transformed := NormalizeLogLine([]byte(s.input))
				var gotSynthetic bool
				if transformed {
					consecutiveTransformed++
					if consecutiveTransformed%threshold == 0 {
						gotSynthetic = true
					}
				} else {
					consecutiveTransformed = 0
				}
				if gotSynthetic != s.wantSynthetic {
					t.Errorf("step %d (input=%q): synthetic=%v, want %v", i, s.input, gotSynthetic, s.wantSynthetic)
				}
			}
		})
	}
}
