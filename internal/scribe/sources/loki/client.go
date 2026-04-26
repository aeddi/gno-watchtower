package loki

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type Client struct {
	base string
	http *http.Client
}

func New(base string) *Client {
	return &Client{base: base, http: &http.Client{Timeout: 30 * time.Second}}
}

// Stream is a labeled set of log entries.
type Stream struct {
	Labels  map[string]string
	Entries []Entry
}

type Entry struct {
	Time time.Time
	Line string
}

// QueryRange fetches log entries for `q` between [from, to]; capped at `limit` per stream.
func (c *Client) QueryRange(ctx context.Context, q string, from, to time.Time, limit int) ([]Stream, error) {
	u, _ := url.Parse(c.base + "/loki/api/v1/query_range")
	v := url.Values{}
	v.Set("query", q)
	v.Set("start", strconv.FormatInt(from.UnixNano(), 10))
	v.Set("end", strconv.FormatInt(to.UnixNano(), 10))
	v.Set("limit", strconv.Itoa(limit))
	v.Set("direction", "FORWARD")
	u.RawQuery = v.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("loki http %d: %s", resp.StatusCode, string(body))
	}
	var raw struct {
		Status string `json:"status"`
		Data   struct {
			Result []struct {
				Stream map[string]string `json:"stream"`
				Values [][]string        `json:"values"` // [[ts_ns, line], ...]
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	out := make([]Stream, 0, len(raw.Data.Result))
	for _, r := range raw.Data.Result {
		s := Stream{Labels: r.Stream}
		for _, v := range r.Values {
			if len(v) != 2 {
				continue
			}
			tsNs, _ := strconv.ParseInt(v[0], 10, 64)
			s.Entries = append(s.Entries, Entry{Time: time.Unix(0, tsNs), Line: v[1]})
		}
		out = append(out, s)
	}
	return out, nil
}
