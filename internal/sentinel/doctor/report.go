// internal/sentinel/doctor/report.go
package doctor

import (
	"github.com/aeddi/gno-watchtower/pkg/termstyle"
)

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
		return termstyle.Fail(r.Name, r.Detail)
	}
}
