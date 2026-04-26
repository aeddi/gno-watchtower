package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

func backfillCmd(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("backfill", flag.ContinueOnError)
	fs.SetOutput(out)
	server := fs.String("server", "http://localhost:8090", "scribe server URL")
	from := fs.String("from", "", "RFC3339 start of range (required)")
	to := fs.String("to", "", "RFC3339 end of range (required)")
	chunkSize := fs.String("chunk-size", "", "chunk size, e.g. 1h (optional)")
	pollInterval := fs.Duration("poll-interval", time.Second, "status poll interval")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *from == "" || *to == "" {
		return errors.New("--from and --to are required")
	}

	body, _ := json.Marshal(map[string]any{
		"from":       *from,
		"to":         *to,
		"chunk_size": *chunkSize,
	})
	resp, err := http.Post(strings.TrimRight(*server, "/")+"/api/backfill", "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("post: %w", err)
	}
	var created struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	dec := json.NewDecoder(resp.Body)
	_ = dec.Decode(&created)
	resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("scheduler refused job: http %d", resp.StatusCode)
	}
	if created.ID == "" {
		return errors.New("response missing id")
	}
	fmt.Fprintf(out, "scheduled %s (status=%s)\n", created.ID, created.Status)

	for {
		time.Sleep(*pollInterval)
		r2, err := http.Get(strings.TrimRight(*server, "/") + "/api/backfill/" + created.ID)
		if err != nil {
			return fmt.Errorf("get status: %w", err)
		}
		var got struct {
			ID         string `json:"id"`
			Status     string `json:"status"`
			LastError  string `json:"last_error"`
			ErrorCount int    `json:"error_count"`
		}
		_ = json.NewDecoder(r2.Body).Decode(&got)
		r2.Body.Close()
		fmt.Fprintf(out, "status=%s\n", got.Status)
		switch got.Status {
		case "completed":
			fmt.Fprintf(out, "%s completed\n", got.ID)
			return nil
		case "failed":
			return fmt.Errorf("%s failed: %s", got.ID, got.LastError)
		case "cancelled":
			return fmt.Errorf("%s cancelled", got.ID)
		}
	}
}
