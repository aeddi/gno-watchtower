// internal/sentinel/doctor/report.go
package doctor

import (
	"fmt"
	"io"

	"github.com/gnolang/val-companion/pkg/termstyle"
)

// PrintReport writes a human-readable doctor report to w.
func PrintReport(w io.Writer, configPath string, results []CheckResult) {
	fmt.Fprintf(w, "Validating sentinel config: %s\n\n", configPath)
	for _, r := range results {
		fmt.Fprintln(w, formatResult(r))
	}
	fmt.Fprintln(w)
}

func formatResult(r CheckResult) string {
	switch r.Status {
	case StatusGreen:
		return termstyle.OK(r.Name, r.Detail)
	case StatusRed:
		return termstyle.Fail(r.Name, r.Detail)
	case StatusOrange:
		return termstyle.Off(r.Name, r.Detail)
	case StatusGrey:
		return termstyle.Skip(r.Name, r.Detail)
	default:
		return fmt.Sprintf("  ? %-20s %s", r.Name, r.Detail)
	}
}
