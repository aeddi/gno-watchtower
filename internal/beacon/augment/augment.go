// Package augment rewrites sentinel /rpc payloads to inject the sentry's own
// view of the network: peer count (from /net_info), chain/moniker/version
// (from /status), and configured metadata keys (from config.toml).
//
// The augmenter fails open per fetch: each of the three sentry lookups
// contributes independently, so a single sentry-side hiccup on /net_info
// does not cost us sentry_status + sentry_config in the same tick. When
// every fetch fails, the original payload is forwarded unchanged. Missing
// sentry data in the watchtower is better than missing validator data.
package augment

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/aeddi/gno-watchtower/internal/beacon/config"
	"github.com/aeddi/gno-watchtower/internal/sentinel/metadata"
	"github.com/aeddi/gno-watchtower/internal/sentinel/rpc"
)

// augmentDeadline bounds the time we'll spend augmenting a single /rpc tick
// before giving up and passing the original body through. Sized so a stuck
// sentry RPC never stalls the sentinel ingest path longer than one tick.
const augmentDeadline = 2 * time.Second

// defaultForceInterval triggers sentry_* re-emission even when no /net_info
// tick arrives for a long time. Without this, the watchtower sees sentry_config
// exactly once per sentinel lifetime — dashboards with short lookback windows
// then age out to "no data".
const defaultForceInterval = 12 * time.Hour

// Augmenter holds references to the sentry's RPC client and metadata config
// for per-request fetching.
//
// mu guards lastAugmentAt — concurrent /rpc handler goroutines may race on it.
type Augmenter struct {
	rpc           *rpc.Client
	metaCfg       config.MetadataConfig
	metaKeys      []string
	forceInterval time.Duration
	mu            sync.Mutex
	lastAugmentAt time.Time
	log           *slog.Logger
}

// New builds an Augmenter from the beacon's config.
// metadataKeys defaults to sentinel's metadata.ConfigKeys set when nil.
func New(cfg *config.Config, metadataKeys []string, log *slog.Logger) *Augmenter {
	if metadataKeys == nil {
		metadataKeys = metadata.ConfigKeys
	}
	fi := cfg.Metadata.ForceInterval.Duration
	if fi <= 0 {
		fi = defaultForceInterval
	}
	return &Augmenter{
		rpc:           rpc.NewClient(cfg.RPC.RPCURL),
		metaCfg:       cfg.Metadata,
		metaKeys:      metadataKeys,
		forceInterval: fi,
		lastAugmentAt: time.Now(),
		log:           log.With("component", "beacon_augmenter"),
	}
}

// Transform is the body-transform hook registered on the beacon server. Only
// touches POST /rpc payloads; returns nil (pass-through) for other paths.
//
// On any error during augmentation, logs a warning and returns nil so the
// server forwards the original body.
func (a *Augmenter) Transform(ctx context.Context, path string, body []byte) []byte {
	if path != "/rpc" {
		return nil
	}
	out, err := a.augmentRPC(ctx, body)
	if err != nil {
		a.log.Warn("augment /rpc failed — forwarding original", "err", err)
		return nil
	}
	return out
}

// augmentRPC decodes the protocol.RPCPayload as a tolerant map, checks
// whether it contains a /net_info key (the main signal we enrich), and if
// so injects three sibling keys before re-encoding.
//
// Tolerant decode: we treat the payload as map[string]json.RawMessage so
// unknown/new keys from future sentinels pass through untouched. We only read
// `data` to check for "net_info" presence.
func (a *Augmenter) augmentRPC(ctx context.Context, body []byte) ([]byte, error) {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	dataRaw, ok := payload["data"]
	if !ok {
		return nil, nil // no data field, can't augment, pass through
	}
	var data map[string]json.RawMessage
	if err := json.Unmarshal(dataRaw, &data); err != nil {
		return nil, err
	}
	_, hasNetInfo := data["net_info"]
	if !hasNetInfo {
		// Normally we only augment on /net_info ticks — no value in re-fetching
		// the sentry for every block/validators/etc push. But sentry_config +
		// sentry_status are static enough that dashboards expect them to always
		// be present; if /net_info stops flowing (stable peer count) the series
		// would age out. Force-augment when we haven't seen a /net_info tick
		// within forceInterval.
		a.mu.Lock()
		stale := time.Since(a.lastAugmentAt) >= a.forceInterval
		a.mu.Unlock()
		if !stale {
			return nil, nil
		}
	}

	// Bound the whole augment step; a stalled sentry RPC must not hold up
	// the sentinel's ingest path indefinitely.
	ctx, cancel := context.WithTimeout(ctx, augmentDeadline)
	defer cancel()

	// Run the three fetches in parallel. Per-fetch fail-open: each key is
	// injected only if its own RPC succeeded; one sentry hiccup on /net_info
	// must not cost us sentry_status + sentry_config in the same tick. When
	// all three fail we return (nil, nil) so the caller forwards the original
	// body unchanged. 1-2 ms latency added per augmented tick.
	var (
		wg                sync.WaitGroup
		statusRaw, netRaw json.RawMessage
		configRaw         json.RawMessage
		statusErr, netErr error
		configErr         error
	)
	wg.Go(func() { statusRaw, statusErr = a.rpc.Status(ctx) })
	wg.Go(func() { netRaw, netErr = a.rpc.NetInfo(ctx) })
	if a.metaCfg.ConfigPath != "" || a.metaCfg.ConfigGetCmd != "" {
		wg.Go(func() { configRaw, configErr = a.fetchConfig(ctx) })
	}
	wg.Wait()

	// Inject what we got; skip what failed. Re-marshal the modified data map
	// back into payload.data without touching other top-level fields of the
	// payload (like collected_at etc.).
	injected := 0
	if statusErr == nil && len(statusRaw) > 0 {
		data["sentry_status"] = statusRaw
		injected++
	} else if statusErr != nil {
		a.log.Debug("augment: sentry /status fetch failed — skipping sentry_status", "err", statusErr)
	}
	if netErr == nil && len(netRaw) > 0 {
		data["sentry_net_info"] = netRaw
		injected++
	} else if netErr != nil {
		a.log.Debug("augment: sentry /net_info fetch failed — skipping sentry_net_info", "err", netErr)
	}
	if configErr == nil && configRaw != nil {
		data["sentry_config"] = configRaw
		injected++
	} else if configErr != nil {
		a.log.Debug("augment: sentry config fetch failed — skipping sentry_config", "err", configErr)
	}
	if injected == 0 {
		// All fetches failed — pass through the original body unchanged.
		return nil, nil
	}
	a.mu.Lock()
	a.lastAugmentAt = time.Now()
	a.mu.Unlock()
	reencoded, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	payload["data"] = reencoded
	return json.Marshal(payload)
}

// fetchConfig reads the configured metadata keys from the sentry's
// gnoland config (file or exec command) and marshals them as a JSON object.
// Failure on any single key is non-fatal — that key is omitted and we proceed
// with whatever was readable.
func (a *Augmenter) fetchConfig(ctx context.Context) (json.RawMessage, error) {
	values := make(map[string]string, len(a.metaKeys))
	for _, key := range a.metaKeys {
		var (
			val string
			err error
		)
		switch {
		case a.metaCfg.ConfigPath != "":
			val, err = metadata.ReadConfigKey(a.metaCfg.ConfigPath, key)
		case a.metaCfg.ConfigGetCmd != "":
			cmd := strings.ReplaceAll(a.metaCfg.ConfigGetCmd, "%s", key)
			val, err = metadata.RunCmd(ctx, cmd)
		}
		if err != nil {
			a.log.Debug("sentry config key unreachable", "key", key, "err", err)
			continue
		}
		values[key] = val
	}
	if len(values) == 0 {
		return nil, nil
	}
	return json.Marshal(values)
}
