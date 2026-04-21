// Package augment rewrites sentinel /rpc payloads to inject the sentry's own
// view of the network: peer count (from /net_info), chain/moniker/version
// (from /status), and configured metadata keys (from config.toml).
//
// The augmenter fails open: if any of the three fetches fails, the original
// payload is forwarded unchanged with a warning log. Missing sentry data in
// the watchtower is better than missing validator data.
package augment

import (
	"context"
	"encoding/json"
	"errors"
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

// Augmenter holds references to the sentry's RPC client and metadata config
// for per-request fetching.
type Augmenter struct {
	rpc      *rpc.Client
	metaCfg  config.MetadataConfig
	metaKeys []string
	log      *slog.Logger
}

// New builds an Augmenter from the beacon's config.
// metadataKeys defaults to sentinel's metadata.ConfigKeys set when nil.
func New(cfg *config.Config, metadataKeys []string, log *slog.Logger) *Augmenter {
	if metadataKeys == nil {
		metadataKeys = metadata.ConfigKeys
	}
	return &Augmenter{
		rpc:      rpc.NewClient(cfg.RPC.RPCURL),
		metaCfg:  cfg.Metadata,
		metaKeys: metadataKeys,
		log:      log.With("component", "beacon_augmenter"),
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
	if _, hasNetInfo := data["net_info"]; !hasNetInfo {
		// Only augment on ticks that carry /net_info — no value in re-
		// fetching the sentry for every block/validators/etc push.
		return nil, nil
	}

	// Bound the whole augment step; a stalled sentry RPC must not hold up
	// the sentinel's ingest path indefinitely.
	ctx, cancel := context.WithTimeout(ctx, augmentDeadline)
	defer cancel()

	// Run the three fetches in parallel; any failure aborts and the caller
	// falls back to passing through. 1-2 ms latency added per augmented tick.
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

	if err := errors.Join(statusErr, netErr, configErr); err != nil {
		return nil, err
	}

	// Inject the three augmentation keys. Re-marshal the modified data map
	// back into payload.data without touching other top-level fields of the
	// payload (like collected_at etc.).
	data["sentry_status"] = statusRaw
	data["sentry_net_info"] = netRaw
	if configRaw != nil {
		data["sentry_config"] = configRaw
	}
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
