package doctor_test

import (
	"strings"
	"testing"

	"github.com/gnolang/val-companion/internal/sentinel/doctor"
	"github.com/gnolang/val-companion/pkg/termstyle"
)

func TestPrintReport_ContainsAllResults(t *testing.T) {
	results := []doctor.CheckResult{
		{Name: "Watchtower", Status: doctor.StatusGreen, Detail: "https://example.com"},
		{Name: "Token valid", Status: doctor.StatusRed, Detail: "auth failed: 401"},
		{Name: "Metadata binary", Status: doctor.StatusOrange, Detail: "disabled in config"},
		{Name: "Resources", Status: doctor.StatusGrey, Detail: "metrics permission not granted"},
	}

	var buf strings.Builder
	doctor.PrintReport(&buf, "/etc/sentinel.toml", results)
	out := buf.String()

	checks := []string{
		"Validating sentinel config:",
		"/etc/sentinel.toml",
		"Watchtower",
		termstyle.SymOK,
		"Token valid",
		termstyle.SymFail,
		termstyle.SymOff,
		termstyle.SymSkip,
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("report missing %q\nfull output:\n%s", want, out)
		}
	}
}
