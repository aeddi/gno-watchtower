// internal/sentinel/logs/docker.go (temporary stub — will be replaced in Task 4)
package logs

import "context"

type DockerSource struct{ containerName string }

func NewDockerSource(containerName string) *DockerSource { return &DockerSource{containerName: containerName} }
func (s *DockerSource) Tail(ctx context.Context, out chan<- LogLine) error {
	<-ctx.Done()
	return ctx.Err()
}
