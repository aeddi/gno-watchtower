package logs

import (
	"bufio"
	"context"
	"encoding/json"
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

// BuildLogsOptions returns the Docker ContainerLogs options for the given lookback.
// lookback > 0 uses Since; lookback == 0 uses Tail:"0" (follow-only).
func BuildLogsOptions(lookback time.Duration, now time.Time) container.LogsOptions {
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

	logStream, err := cli.ContainerLogs(ctx, s.containerName, BuildLogsOptions(s.resumeLookback, time.Now()))
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
	for scanner.Scan() {
		// Copy scanner bytes — scanner reuses the underlying buffer on next call.
		raw := json.RawMessage(append([]byte(nil), scanner.Bytes()...))
		// Skip non-JSON lines (e.g. plain-text startup messages before the JSON logger is ready).
		if !json.Valid(raw) {
			continue
		}
		level := ParseLevel(raw)
		select {
		case out <- LogLine{Level: level, Raw: raw}:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan docker logs: %w", err)
	}
	return nil
}
