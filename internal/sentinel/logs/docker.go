package logs

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/docker/docker/api/types/container"
	dockerclient "github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

const dockerRetryDelay = 5 * time.Second

// DockerSource tails logs from a Docker container.
// The Docker daemon is reached via DOCKER_HOST or the default Unix socket.
type DockerSource struct {
	containerName string
	log           *slog.Logger
}

// NewDockerSource creates a DockerSource for the named container.
func NewDockerSource(containerName string, log *slog.Logger) *DockerSource {
	return &DockerSource{
		containerName: containerName,
		log:           log.With("component", "docker_log_source", "container", containerName),
	}
}

// Tail streams log lines from the container until ctx is cancelled.
// Each line is expected to be a JSON object from gnoland.
// If the container restarts or the connection drops, Tail automatically reconnects
// after a short delay instead of returning an error.
func (s *DockerSource) Tail(ctx context.Context, out chan<- LogLine) error {
	cli, err := dockerclient.NewClientWithOpts(
		dockerclient.FromEnv,
		dockerclient.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return fmt.Errorf("docker client: %w", err)
	}
	defer cli.Close()

	for {
		if err := s.stream(ctx, cli, out); err != nil {
			return err // only a cancelled context reaches here
		}
		// stream returned nil: container went away, wait and reconnect.
		s.log.Warn("container log stream ended, reconnecting", "delay", dockerRetryDelay)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(dockerRetryDelay):
		}
	}
}

// stream opens a single ContainerLogs session and drains it into out.
// Returns nil when the stream ends for a transient reason (container restart).
// Returns a non-nil error only when ctx is cancelled.
func (s *DockerSource) stream(ctx context.Context, cli *dockerclient.Client, out chan<- LogLine) error {
	logStream, err := cli.ContainerLogs(ctx, s.containerName, container.LogsOptions{
		Follow:     true,
		ShowStdout: true,
		ShowStderr: true,
		Tail:       "0", // don't replay historical logs, only follow new entries
	})
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		s.log.Warn("container logs unavailable", "err", err)
		return nil // transient — caller will retry
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
	consecutiveTransformed := 0
	for scanner.Scan() {
		normalized, transformed := NormalizeLogLine(scanner.Bytes())
		if transformed {
			consecutiveTransformed++
			if consecutiveTransformed%consecutiveTransformWarnThreshold == 0 {
				s.log.Warn("more than 30 consecutive non-JSON log lines were auto-transformed; add --log-format=json to gnoland")
				select {
				case out <- syntheticWarnLine():
				case <-ctx.Done():
					return ctx.Err()
				}
			}
		} else {
			consecutiveTransformed = 0
		}
		level := ParseLevel(normalized)
		select {
		case out <- LogLine{Level: level, Raw: normalized}:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if err := scanner.Err(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		s.log.Warn("docker log scan error", "err", err)
	}
	return nil // stream ended cleanly or with a transient error — caller will retry
}
