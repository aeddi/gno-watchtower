// internal/sentinel/doctor/remote.go
package doctor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/aeddi/gno-watchtower/internal/sentinel/config"
	"github.com/aeddi/gno-watchtower/pkg/noise"
)

// CheckRemoteTokenAndPermissions runs the remote reachable, token valid, and token permissions
// checks using a single GET /auth/check call. Returns the three CheckResults and the parsed
// AuthResponse (nil if the server is unreachable or the token is invalid).
func CheckRemoteTokenAndPermissions(ctx context.Context, cfg *config.Config) ([]CheckResult, *AuthResponse) {
	if config.IsPlaceholder(cfg.Server.URL) {
		return []CheckResult{
			{Name: "Watchtower", Status: StatusOrange, Detail: "server.url not configured"},
			{Name: "Token valid", Status: StatusOrange, Detail: "server.url not configured"},
			{Name: "Token permissions", Status: StatusOrange, Detail: "server.url not configured"},
		}, nil
	}
	if config.IsPlaceholder(cfg.Server.Token) {
		return []CheckResult{
			{Name: "Watchtower", Status: StatusOrange, Detail: "server.token not configured"},
			{Name: "Token valid", Status: StatusOrange, Detail: "server.token not configured"},
			{Name: "Token permissions", Status: StatusOrange, Detail: "server.token not configured"},
		}, nil
	}

	client, reqURL, err := buildAuthCheckClient(cfg)
	if err != nil {
		return unreachable(err.Error()), nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return unreachable(fmt.Sprintf("build request: %v", err)), nil
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Server.Token)

	resp, err := client.Do(req)
	if err != nil {
		return unreachable(err.Error()), nil
	}
	defer resp.Body.Close()

	// We got an HTTP response — server is reachable regardless of status code.
	remote := CheckResult{Name: "Watchtower", Status: StatusGreen, Detail: cfg.Server.URL}

	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
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
func unreachable(reason string) []CheckResult {
	return []CheckResult{
		{Name: "Watchtower", Status: StatusRed, Detail: fmt.Sprintf("unreachable: %s", reason)},
		{Name: "Token valid", Status: StatusRed, Detail: "cannot check: watchtower unreachable"},
		{Name: "Token permissions", Status: StatusRed, Detail: "cannot check: watchtower unreachable"},
	}
}

// buildAuthCheckClient returns an http.Client and the URL to GET /auth/check
// against. When cfg.Server.URL is noise://, the client's transport routes
// through a Noise-wrapped net.Conn and the URL is rewritten to http:// (the
// scheme is a routing hint — transport encryption is Noise, not TLS). For
// any other scheme the plain default-transport client is returned.
func buildAuthCheckClient(cfg *config.Config) (*http.Client, string, error) {
	if !strings.HasPrefix(cfg.Server.URL, "noise://") {
		return &http.Client{Timeout: 10 * time.Second}, cfg.Server.URL + "/auth/check", nil
	}
	noiseCfg, err := cfg.NoiseConfig()
	if err != nil {
		return nil, "", fmt.Errorf("noise config: %w", err)
	}
	if noiseCfg == nil {
		return nil, "", fmt.Errorf("server.url is noise:// but no beacon keypair was configured")
	}
	// Clone so later mutations of the caller's AuthorizedKeys can't race with
	// Dial goroutines.
	nc := noiseCfg.Clone()
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return noise.Dial(ctx, network, addr, nc)
		},
	}
	rewritten := "http://" + strings.TrimPrefix(cfg.Server.URL, "noise://") + "/auth/check"
	return &http.Client{Transport: transport, Timeout: 10 * time.Second}, rewritten, nil
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
