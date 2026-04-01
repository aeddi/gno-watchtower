// internal/sentinel/resources/docker.go
package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	dockerclient "github.com/docker/docker/client"
)

// ContainerStats returns the one-shot Docker stats snapshot for the named container as raw JSON bytes.
// It wraps dockerContainerStats for use by external packages.
func ContainerStats(ctx context.Context, name string) ([]byte, error) {
	return dockerContainerStats(ctx, name)
}

// dockerContainerStats returns the one-shot Docker stats snapshot for the named container as raw JSON bytes.
func dockerContainerStats(ctx context.Context, containerName string) ([]byte, error) {
	cli, err := dockerclient.NewClientWithOpts(
		dockerclient.FromEnv,
		dockerclient.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return nil, fmt.Errorf("docker client: %w", err)
	}
	defer cli.Close()

	resp, err := cli.ContainerStatsOneShot(ctx, containerName)
	if err != nil {
		return nil, fmt.Errorf("container stats %q: %w", containerName, err)
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read stats body: %w", err)
	}

	// Validate it's JSON before returning.
	if !json.Valid(b) {
		return nil, fmt.Errorf("container stats: invalid JSON response")
	}
	return b, nil
}
