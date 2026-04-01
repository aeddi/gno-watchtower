// internal/sentinel/doctor/report.go
package doctor

import (
	"fmt"
	"io"
)

// PrintReport writes a human-readable doctor report to w.
func PrintReport(w io.Writer, configPath string, results []CheckResult) {
	fmt.Fprintf(w, "sentinel doctor — %s\n\n", configPath)
	for _, r := range results {
		fmt.Fprintf(w, "  %-24s [%s]  %s\n", r.Name, r.Status, r.Detail)
	}
	fmt.Fprintln(w)
}
