package doctor

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/aeddi/gno-watchtower/internal/beacon/config"
)

// CheckWatchtower probes the upstream watchtower's public /health endpoint.
// No token is sent: the watchtower exposes /health unauthenticated so sentinels
// and beacons can sanity-check reachability without provisioning credentials
// for the doctor. A 200 means both DNS/TCP/TLS and the watchtower process are
// healthy; any non-200 or transport error returns Red.
func CheckWatchtower(ctx context.Context, serverURL string) CheckResult {
	const name = "Watchtower"
	if config.IsPlaceholder(serverURL) {
		return CheckResult{Name: name, Status: StatusOrange, Detail: "server.url not configured"}
	}

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, serverURL+"/health", nil)
	if err != nil {
		return CheckResult{Name: name, Status: StatusRed, Detail: fmt.Sprintf("build request: %v", err)}
	}
	resp, err := client.Do(req)
	if err != nil {
		return CheckResult{Name: name, Status: StatusRed, Detail: fmt.Sprintf("unreachable: %v", err)}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return CheckResult{
			Name:   name,
			Status: StatusRed,
			Detail: fmt.Sprintf("/health returned HTTP %d", resp.StatusCode),
		}
	}
	return CheckResult{Name: name, Status: StatusGreen, Detail: serverURL}
}
