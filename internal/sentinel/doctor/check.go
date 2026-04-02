// internal/sentinel/doctor/check.go
package doctor

// Status represents the outcome of a doctor check.
type Status string

const (
	StatusGreen  Status = "GREEN"  // working correctly
	StatusRed    Status = "RED"    // enabled but failing
	StatusOrange Status = "ORANGE" // disabled in config
	StatusGrey   Status = "GREY"   // not permitted by token
)

// CheckResult holds the outcome of a single doctor check.
type CheckResult struct {
	Name   string
	Status Status
	Detail string
}

// AuthResponse is the decoded body of GET /auth/check.
type AuthResponse struct {
	Validator    string   `json:"validator"`
	Permissions  []string `json:"permissions"`
	LogsMinLevel string   `json:"logs_min_level"`
}
