package doctor

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// CheckRPC probes the sentry's local gnoland RPC endpoint by requesting
// /status — the cheapest RPC route that requires the node be alive. Unlike
// the sentinel doctor (which only validates rpc_url shape), the beacon
// actively talks to RPC on every /rpc forward to augment payloads, so a
// broken RPC is a hot bug rather than a startup-only one. A live probe
// surfaces that before the first sentinel connects.
func CheckRPC(ctx context.Context, rpcURL string) CheckResult {
	const name = "RPC"
	if rpcURL == "" {
		return CheckResult{Name: name, Status: StatusRed, Detail: "rpc.rpc_url not set"}
	}
	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rpcURL+"/status", nil)
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
			Detail: fmt.Sprintf("/status returned HTTP %d", resp.StatusCode),
		}
	}
	return CheckResult{Name: name, Status: StatusGreen, Detail: rpcURL}
}
