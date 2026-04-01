package doctor_test

import (
	"strings"
	"testing"

	"github.com/gnolang/val-companion/internal/sentinel/doctor"
)

func TestPrintReport_ContainsAllResults(t *testing.T) {
	results := []doctor.CheckResult{
		{Name: "Remote reachable", Status: doctor.StatusGreen, Detail: "https://example.com"},
		{Name: "Token valid", Status: doctor.StatusRed, Detail: "auth failed: 401"},
		{Name: "Metadata binary", Status: doctor.StatusOrange, Detail: "disabled in config"},
		{Name: "Resources", Status: doctor.StatusGrey, Detail: "metrics permission not granted"},
	}

	var buf strings.Builder
	doctor.PrintReport(&buf, "/etc/sentinel.toml", results)
	out := buf.String()

	checks := []string{
		"sentinel doctor",
		"/etc/sentinel.toml",
		"Remote reachable",
		"[GREEN]",
		"Token valid",
		"[RED]",
		"[ORANGE]",
		"[GREY]",
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("report missing %q\nfull output:\n%s", want, out)
		}
	}
}
