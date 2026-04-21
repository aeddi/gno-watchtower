package logs

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/docker/docker/api/types/container"
	dockerclient "github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

// DockerSource tails logs from a Docker container.
// The Docker daemon is reached via DOCKER_HOST or the default Unix socket.
type DockerSource struct {
	containerName  string
	resumeLookback time.Duration
}

// NewDockerSource creates a DockerSource for the named container.
// resumeLookback is how far back to read on each connect; 0 means only follow new entries.
// A non-zero value is used on sentinel restart to catch up logs written during downtime
// (the Docker API has no persistent cursor, so we rewind by a time window).
func NewDockerSource(containerName string, resumeLookback time.Duration) *DockerSource {
	return &DockerSource{containerName: containerName, resumeLookback: resumeLookback}
}

// buildLogsOptions returns the Docker ContainerLogs options for the given lookback.
// lookback > 0 uses Since; lookback == 0 uses Tail:"0" (follow-only).
// Package-private — used by Tail below and exercised directly in docker_test.go.
func buildLogsOptions(lookback time.Duration, now time.Time) container.LogsOptions {
	opts := container.LogsOptions{Follow: true, ShowStdout: true, ShowStderr: true}
	if lookback > 0 {
		opts.Since = strconv.FormatInt(now.Add(-lookback).Unix(), 10)
	} else {
		opts.Tail = "0"
	}
	return opts
}

// Tail streams log lines from the container until ctx is cancelled.
// Each line is expected to be a JSON object from gnoland.
func (s *DockerSource) Tail(ctx context.Context, out chan<- LogLine) error {
	cli, err := dockerclient.NewClientWithOpts(
		dockerclient.FromEnv,
		dockerclient.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return fmt.Errorf("docker client: %w", err)
	}
	defer cli.Close()

	logStream, err := cli.ContainerLogs(ctx, s.containerName, buildLogsOptions(s.resumeLookback, time.Now()))
	if err != nil {
		return fmt.Errorf("container logs %q: %w", s.containerName, err)
	}
	defer logStream.Close()

	// Docker multiplexes stdout/stderr with an 8-byte header per frame.
	// stdcopy.StdCopy demultiplexes both streams into a single writer.
	pr, pw := io.Pipe()
	defer pr.Close()
	go func() {
		_, err := stdcopy.StdCopy(pw, pw, logStream)
		pw.CloseWithError(err)
	}()

	scanner := bufio.NewScanner(pr)
	// Default Scanner buffer caps lines at 64KB; gnoland can emit much larger
	// single lines (e.g. dumped consensus state, or operator/test injection).
	// Use a 1MB ceiling — large enough for real-world lines while bounding
	// memory. Lines above the ceiling still error with ErrTooLong, which we
	// handle below by skipping to the next newline instead of crashing Tail.
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	for scanner.Scan() {
		// Copy scanner bytes — scanner reuses the underlying buffer on next call.
		raw := append([]byte(nil), scanner.Bytes()...)
		// Non-JSON gnoland output (startup banners, stack traces) is wrapped as
		// a synthetic JSON line so it still reaches Loki under module=sentinel-raw.
		// Empty lines return nil and are skipped.
		line := EnsureJSON(raw, time.Now())
		if line == nil {
			continue
		}
		level := ParseLevel(line)
		select {
		case out <- LogLine{Level: level, Raw: line}:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	// bufio.ErrTooLong here means a single line exceeded our 1MB ceiling. We
	// surface it but return nil so the caller reconnects instead of treating it
	// as fatal. The oversized line is lost, but the rest of the stream survives.
	if err := scanner.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			return nil
		}
		return fmt.Errorf("scan docker logs: %w", err)
	}
	return nil
}
