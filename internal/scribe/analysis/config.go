package analysis

import (
	"fmt"
	"time"
)

type ruleConfig struct {
	defaults map[string]any
}

// NewRuleConfig builds a typed config view for one rule by merging declared
// defaults from meta.Params with values supplied from
// [analysis.rules.<code>_v<version>]. Unknown keys and out-of-bounds values
// are reported as errors at startup — never silently accepted.
func NewRuleConfig(meta Meta, overlay map[string]any) (RuleConfig, error) {
	merged := map[string]any{}
	for k, spec := range meta.Params {
		merged[k] = spec.Default
	}
	for k, v := range overlay {
		spec, ok := meta.Params[k]
		if !ok {
			return nil, fmt.Errorf("rule %s: unknown config key %q", meta.Kind(), k)
		}
		coerced, err := coerce(spec.Default, v)
		if err != nil {
			return nil, fmt.Errorf("rule %s: param %q: %w", meta.Kind(), k, err)
		}
		if err := checkBounds(coerced, spec); err != nil {
			return nil, fmt.Errorf("rule %s: param %q: %w", meta.Kind(), k, err)
		}
		merged[k] = coerced
	}
	return &ruleConfig{defaults: merged}, nil
}

// coerce normalizes a TOML-decoded value to match the declared default's
// runtime type. We accept the few combinations the TOML library actually
// produces (int64 from integers, float64 from floats, string from strings,
// bool from bools), plus a string-to-Duration upgrade for time.Duration
// defaults.
func coerce(def, v any) (any, error) {
	switch def.(type) {
	case int64:
		switch x := v.(type) {
		case int64:
			return x, nil
		case int:
			return int64(x), nil
		}
	case float64:
		switch x := v.(type) {
		case float64:
			return x, nil
		case int64:
			return float64(x), nil
		}
	case string:
		if s, ok := v.(string); ok {
			return s, nil
		}
	case bool:
		if b, ok := v.(bool); ok {
			return b, nil
		}
	case time.Duration:
		switch x := v.(type) {
		case string:
			d, err := time.ParseDuration(x)
			if err != nil {
				return nil, fmt.Errorf("invalid duration %q: %w", x, err)
			}
			return d, nil
		case time.Duration:
			return x, nil
		}
	}
	return nil, fmt.Errorf("type mismatch: default %T vs supplied %T", def, v)
}

func checkBounds(v any, spec ParamSpec) error {
	switch x := v.(type) {
	case int64:
		if min, ok := spec.Min.(int64); ok && x < min {
			return fmt.Errorf("value %d below min %d", x, min)
		}
		if max, ok := spec.Max.(int64); ok && x > max {
			return fmt.Errorf("value %d above max %d", x, max)
		}
	case float64:
		if min, ok := spec.Min.(float64); ok && x < min {
			return fmt.Errorf("value %g below min %g", x, min)
		}
		if max, ok := spec.Max.(float64); ok && x > max {
			return fmt.Errorf("value %g above max %g", x, max)
		}
	}
	return nil
}

func (c *ruleConfig) Float64(k string) float64 {
	if v, ok := c.defaults[k].(float64); ok {
		return v
	}
	return 0
}

func (c *ruleConfig) Int(k string) int64 {
	if v, ok := c.defaults[k].(int64); ok {
		return v
	}
	return 0
}

func (c *ruleConfig) Duration(k string) time.Duration {
	if v, ok := c.defaults[k].(time.Duration); ok {
		return v
	}
	return 0
}

func (c *ruleConfig) String(k string) string {
	if v, ok := c.defaults[k].(string); ok {
		return v
	}
	return ""
}

func (c *ruleConfig) Bool(k string) bool {
	if v, ok := c.defaults[k].(bool); ok {
		return v
	}
	return false
}
