package logs

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/docker/docker/api/types/container"
	dockerclient "github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

// DockerSource tails logs from a Docker container.
// The Docker daemon is reached via DOCKER_HOST or the default Unix socket.
type DockerSource struct {
	containerName string
}

// NewDockerSource creates a DockerSource for the named container.
func NewDockerSource(containerName string) *DockerSource {
	return &DockerSource{containerName: containerName}
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

	logStream, err := cli.ContainerLogs(ctx, s.containerName, container.LogsOptions{
		Follow:     true,
		ShowStdout: true,
		ShowStderr: true,
		Tail:       "0", // don't replay historical logs, only follow new entries
	})
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
