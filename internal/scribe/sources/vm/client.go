package vm

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

// Client is an HTTP wrapper around VictoriaMetrics PromQL endpoints.
type Client struct {
	base string
	http *http.Client
}

func New(base string) *Client {
	return &Client{base: base, http: &http.Client{Timeout: 30 * time.Second}}
}

// Sample is one (labels, value) point at time t.
type Sample struct {
	Labels map[string]string
	Time   time.Time
	Value  float64
}

// Series is a labeled series of points (range query).
type Series struct {
	Labels map[string]string
	Values []Sample // ordered by time
}

// Instant runs an instant PromQL query.
func (c *Client) Instant(ctx context.Context, q string, at time.Time) ([]Sample, error) {
	u, _ := url.Parse(c.base + "/api/v1/query")
	v := url.Values{}
	v.Set("query", q)
	v.Set("time", strconv.FormatFloat(float64(at.UnixNano())/1e9, 'f', 3, 64))
	u.RawQuery = v.Encode()

	body, err := c.do(ctx, u.String())
	if err != nil {
		return nil, err
	}
	var resp struct {
		Status string `json:"status"`
		Data   struct {
			ResultType string `json:"resultType"`
			Result     []struct {
				Metric map[string]string `json:"metric"`
				Value  []json.RawMessage `json:"value"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	if resp.Status != "success" {
		return nil, fmt.Errorf("vm status = %q", resp.Status)
	}
	out := make([]Sample, 0, len(resp.Data.Result))
	for _, r := range resp.Data.Result {
		if len(r.Value) != 2 {
			continue
		}
		var ts float64
		var val string
		_ = json.Unmarshal(r.Value[0], &ts)
		_ = json.Unmarshal(r.Value[1], &val)
		f, _ := strconv.ParseFloat(val, 64)
		out = append(out, Sample{
			Labels: r.Metric,
			Time:   time.Unix(int64(ts), int64((ts-float64(int64(ts)))*1e9)),
			Value:  f,
		})
	}
	return out, nil
}

// Range runs a range PromQL query.
func (c *Client) Range(ctx context.Context, q string, from, to time.Time, step time.Duration) ([]Series, error) {
	u, _ := url.Parse(c.base + "/api/v1/query_range")
	v := url.Values{}
	v.Set("query", q)
	v.Set("start", strconv.FormatFloat(float64(from.UnixNano())/1e9, 'f', 3, 64))
	v.Set("end", strconv.FormatFloat(float64(to.UnixNano())/1e9, 'f', 3, 64))
	v.Set("step", step.String())
	u.RawQuery = v.Encode()

	body, err := c.do(ctx, u.String())
	if err != nil {
		return nil, err
	}
	var resp struct {
		Status string `json:"status"`
		Data   struct {
			Result []struct {
				Metric map[string]string   `json:"metric"`
				Values [][]json.RawMessage `json:"values"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	if resp.Status != "success" {
		return nil, fmt.Errorf("vm status = %q", resp.Status)
	}
	out := make([]Series, 0, len(resp.Data.Result))
	for _, r := range resp.Data.Result {
		s := Series{Labels: r.Metric}
		for _, vv := range r.Values {
			if len(vv) != 2 {
				continue
			}
			var ts float64
			var val string
			_ = json.Unmarshal(vv[0], &ts)
			_ = json.Unmarshal(vv[1], &val)
			f, _ := strconv.ParseFloat(val, 64)
			s.Values = append(s.Values, Sample{
				Labels: r.Metric,
				Time:   time.Unix(int64(ts), int64((ts-float64(int64(ts)))*1e9)),
				Value:  f,
			})
		}
		out = append(out, s)
	}
	return out, nil
}

func (c *Client) do(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("vm http %d: %s", resp.StatusCode, string(body))
	}
	return io.ReadAll(resp.Body)
}
