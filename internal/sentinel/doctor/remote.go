// internal/sentinel/doctor/remote.go
package doctor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/gnolang/val-companion/internal/sentinel/config"
)

// CheckRemoteTokenAndPermissions runs the remote reachable, token valid, and token permissions
// checks using a single GET /auth/check call. Returns the three CheckResults and the parsed
// AuthResponse (nil if the server is unreachable or the token is invalid).
func CheckRemoteTokenAndPermissions(ctx context.Context, cfg *config.Config) ([]CheckResult, *AuthResponse) {
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.Server.URL+"/auth/check", nil)
	if err != nil {
		return unreachable(cfg.Server.URL, fmt.Sprintf("build request: %v", err)), nil
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Server.Token)

	resp, err := client.Do(req)
	if err != nil {
		return unreachable(cfg.Server.URL, err.Error()), nil
	}
	defer resp.Body.Close()

	// We got an HTTP response — server is reachable regardless of status code.
	remote := CheckResult{Name: "Remote reachable", Status: StatusGreen, Detail: cfg.Server.URL}

	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, resp.Body)
		token := CheckResult{
			Name:   "Token valid",
			Status: StatusRed,
			Detail: fmt.Sprintf("auth/check returned HTTP %d", resp.StatusCode),
		}
		perms := CheckResult{
			Name:   "Token permissions",
			Status: StatusRed,
			Detail: "cannot check: token invalid",
		}
		return []CheckResult{remote, token, perms}, nil
	}

	var ar AuthResponse
	if err := json.NewDecoder(resp.Body).Decode(&ar); err != nil {
		token := CheckResult{
			Name:   "Token valid",
			Status: StatusRed,
			Detail: fmt.Sprintf("decode auth response: %v", err),
		}
		perms := CheckResult{
			Name:   "Token permissions",
			Status: StatusRed,
			Detail: "cannot check: auth response invalid",
		}
		return []CheckResult{remote, token, perms}, nil
	}

	token := CheckResult{
		Name:   "Token valid",
		Status: StatusGreen,
		Detail: fmt.Sprintf("validator: %s", ar.Validator),
	}
	perms := checkPermissions(cfg, &ar)
	return []CheckResult{remote, token, perms}, &ar
}

// unreachable returns three Red results when the server cannot be reached.
func unreachable(serverURL, reason string) []CheckResult {
	return []CheckResult{
		{Name: "Remote reachable", Status: StatusRed, Detail: fmt.Sprintf("%s unreachable: %s", serverURL, reason)},
		{Name: "Token valid", Status: StatusRed, Detail: "cannot check: remote unreachable"},
		{Name: "Token permissions", Status: StatusRed, Detail: "cannot check: remote unreachable"},
	}
}

// checkPermissions compares enabled features in cfg against the token's permissions.
func checkPermissions(cfg *config.Config, ar *AuthResponse) CheckResult {
	var missing []string

	if cfg.RPC.Enabled && !slices.Contains(ar.Permissions, "rpc") {
		missing = append(missing, "rpc")
	}
	if (cfg.Resources.Enabled || cfg.Metadata.Enabled) && !slices.Contains(ar.Permissions, "metrics") {
		missing = append(missing, "metrics")
	}
	if cfg.Logs.Enabled && !slices.Contains(ar.Permissions, "logs") {
		missing = append(missing, "logs")
	}
	if cfg.OTLP.Enabled && !slices.Contains(ar.Permissions, "otlp") {
		missing = append(missing, "otlp")
	}

	if len(missing) > 0 {
		return CheckResult{
			Name:   "Token permissions",
			Status: StatusRed,
			Detail: fmt.Sprintf("missing permissions for enabled features: %s", strings.Join(missing, ", ")),
		}
	}
	return CheckResult{
		Name:   "Token permissions",
		Status: StatusGreen,
		Detail: fmt.Sprintf("granted: %s", strings.Join(ar.Permissions, ", ")),
	}
}
