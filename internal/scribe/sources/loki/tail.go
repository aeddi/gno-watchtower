package loki

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/coder/websocket"
)

// TailEntry is one log line received via the live tail.
type TailEntry struct {
	Stream Stream // Labels populated; the embedded Entries slice is not used here.
	Time   time.Time
	Line   string
}

// Tail opens a /loki/api/v1/tail websocket and sends entries to ch until ctx is done
// or the connection drops. On error or disconnect, returns; the caller is
// responsible for retry with a new `start` timestamp (typically last_seen - overlap).
func Tail(ctx context.Context, base string, query string, start time.Time, ch chan<- TailEntry) error {
	u, err := url.Parse(base + "/loki/api/v1/tail")
	if err != nil {
		return err
	}
	v := url.Values{}
	v.Set("query", query)
	v.Set("start", strconv.FormatInt(start.UnixNano(), 10))
	u.RawQuery = v.Encode()

	// coder/websocket dials over the http(s) transport; convert ws/wss → http/https
	// so test servers built with httptest.NewServer (http://) work as websocket peers.
	dialURL := strings.Replace(u.String(), "wss://", "https://", 1)
	dialURL = strings.Replace(dialURL, "ws://", "http://", 1)

	c, _, err := websocket.Dial(ctx, dialURL, nil)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer c.Close(websocket.StatusNormalClosure, "")

	for {
		_, data, err := c.Read(ctx)
		if err != nil {
			return err
		}
		var msg struct {
			Streams []struct {
				Stream map[string]string `json:"stream"`
				Values [][]string        `json:"values"`
			} `json:"streams"`
		}
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}
		for _, s := range msg.Streams {
			for _, vv := range s.Values {
				if len(vv) != 2 {
					continue
				}
				tsNs, _ := strconv.ParseInt(vv[0], 10, 64)
				select {
				case ch <- TailEntry{Stream: Stream{Labels: s.Stream}, Time: time.Unix(0, tsNs), Line: vv[1]}:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
		}
	}
}
