package analysis

import (
	"testing"
	"time"
)

func TestRuleConfigFallsBackToDefaultsFromMeta(t *testing.T) {
	meta := Meta{
		Code: "x", Version: 1,
		Params: map[string]ParamSpec{
			"threshold": {Default: int64(60)},
			"factor":    {Default: 1.5},
			"window":    {Default: 30 * time.Second},
			"label":     {Default: "default-label"},
			"enabled":   {Default: true},
		},
	}
	cfg, err := NewRuleConfig(meta, nil)
	if err != nil {
		t.Fatalf("NewRuleConfig: %v", err)
	}
	if got := cfg.Int("threshold"); got != 60 {
		t.Errorf("Int(threshold) = %d, want 60", got)
	}
	if got := cfg.Float64("factor"); got != 1.5 {
		t.Errorf("Float64(factor) = %f, want 1.5", got)
	}
	if got := cfg.Duration("window"); got != 30*time.Second {
		t.Errorf("Duration(window) = %v", got)
	}
	if got := cfg.String("label"); got != "default-label" {
		t.Errorf("String(label) = %q", got)
	}
	if got := cfg.Bool("enabled"); !got {
		t.Errorf("Bool(enabled) = false, want true")
	}
}

func TestRuleConfigOverlayBeatsDefaults(t *testing.T) {
	meta := Meta{
		Code: "x", Version: 1,
		Params: map[string]ParamSpec{
			"threshold": {Default: int64(60)},
			"factor":    {Default: 1.5},
			"window":    {Default: 30 * time.Second},
		},
	}
	cfg, err := NewRuleConfig(meta, map[string]any{
		"threshold": int64(90),
		"factor":    2.0,
		"window":    "2m",
	})
	if err != nil {
		t.Fatalf("NewRuleConfig: %v", err)
	}
	if got := cfg.Int("threshold"); got != 90 {
		t.Errorf("Int = %d, want 90", got)
	}
	if got := cfg.Float64("factor"); got != 2.0 {
		t.Errorf("Float64 = %f, want 2.0", got)
	}
	if got := cfg.Duration("window"); got != 2*time.Minute {
		t.Errorf("Duration = %v, want 2m", got)
	}
}

func TestRuleConfigUnknownKeyIsRejected(t *testing.T) {
	meta := Meta{Code: "x", Version: 1, Params: map[string]ParamSpec{
		"threshold": {Default: int64(60)},
	}}
	_, err := NewRuleConfig(meta, map[string]any{"bogus": 1})
	if err == nil {
		t.Errorf("expected error for unknown key, got nil")
	}
}

func TestRuleConfigBoundsCheck(t *testing.T) {
	meta := Meta{Code: "x", Version: 1, Params: map[string]ParamSpec{
		"factor": {Default: 1.0, Min: 0.0, Max: 100.0},
	}}
	if _, err := NewRuleConfig(meta, map[string]any{"factor": 200.0}); err == nil {
		t.Errorf("expected out-of-bounds error, got nil")
	}
}
