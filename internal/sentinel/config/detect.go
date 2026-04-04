package config

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

// Environment holds the results of environment detection probes.
type Environment struct {
	Docker   *DockerResult
	Journald *JournaldResult
	Binary   *BinaryResult
}

// DockerResult holds information about a detected gnoland Docker container.
type DockerResult struct {
	ContainerName string
	RPCPort       uint16 // 0 if port 26657 is not exposed
}

// JournaldResult holds information about a detected gnoland systemd service.
type JournaldResult struct {
	UnitName string
}

// BinaryResult holds information about a detected gnoland binary on PATH.
type BinaryResult struct {
	Path        string
	GenesisPath string // empty if not found near the binary
	ConfigPath  string // empty if not found near the binary
}

// Detect probes the environment for a gnoland setup and prints progress to w.
func Detect(ctx context.Context, w io.Writer) *Environment {
	fmt.Fprintln(w, "Detecting environment...")
	env := &Environment{}

	env.Docker = probeDocker(ctx, w)
	env.Journald = probeJournald(w)
	env.Binary = probeBinary(w)

	return env
}

func probeDocker(ctx context.Context, w io.Writer) *DockerResult {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		fmt.Fprintf(w, "  Docker container: not available (%v)\n", err)
		return nil
	}
	defer cli.Close()

	containers, err := cli.ContainerList(ctx, container.ListOptions{})
	if err != nil {
		fmt.Fprintf(w, "  Docker container: not available (%v)\n", err)
		return nil
	}

	for _, c := range containers {
		for _, name := range c.Names {
			trimmed := strings.TrimPrefix(name, "/")
			if !strings.Contains(trimmed, "gnoland") {
				continue
			}
			result := &DockerResult{ContainerName: trimmed}
			for _, p := range c.Ports {
				if p.PrivatePort == 26657 && p.PublicPort != 0 {
					result.RPCPort = p.PublicPort
					break
				}
			}
			fmt.Fprintf(w, "  Docker container: found (%s)\n", trimmed)
			return result
		}
	}

	fmt.Fprintln(w, "  Docker container: not found")
	return nil
}

func probeJournald(w io.Writer) *JournaldResult {
	out, err := exec.Command("systemctl", "list-units", "--type=service", "--plain", "--no-legend").Output()
	if err != nil {
		fmt.Fprintln(w, "  Journald service: not available")
		return nil
	}

	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 && strings.Contains(fields[0], "gnoland") {
			fmt.Fprintf(w, "  Journald service: found (%s)\n", fields[0])
			return &JournaldResult{UnitName: fields[0]}
		}
	}

	fmt.Fprintln(w, "  Journald service: not found")
	return nil
}

func probeBinary(w io.Writer) *BinaryResult {
	binPath, err := exec.LookPath("gnoland")
	if err != nil {
		fmt.Fprintln(w, "  Gnoland binary:   not found")
		return nil
	}

	// Resolve symlinks to get the real binary directory.
	resolved, err := filepath.EvalSymlinks(binPath)
	if err != nil {
		resolved = binPath
	}
	fmt.Fprintf(w, "  Gnoland binary:   found (%s)\n", resolved)

	result := &BinaryResult{Path: resolved}
	dir := filepath.Dir(resolved)

	genesisPath := filepath.Join(dir, "genesis.json")
	if _, err := os.Stat(genesisPath); err == nil {
		fmt.Fprintf(w, "    genesis.json:   found (%s)\n", genesisPath)
		result.GenesisPath = genesisPath
	} else {
		fmt.Fprintln(w, "    genesis.json:   not found")
	}

	configPath := filepath.Join(dir, "gnoland-data", "config", "config.toml")
	if _, err := os.Stat(configPath); err == nil {
		fmt.Fprintf(w, "    config.toml:    found (%s)\n", configPath)
		result.ConfigPath = configPath
	} else {
		fmt.Fprintln(w, "    config.toml:    not found")
	}

	return result
}

// applyDetection overrides default config values with detection results.
func applyDetection(cfg *Config, env *Environment) {
	if env.Docker != nil {
		cfg.Logs.Source = "docker"
		cfg.Logs.ContainerName = env.Docker.ContainerName
		cfg.Resources.ContainerName = env.Docker.ContainerName
		cfg.Resources.Source = "both"
		if env.Docker.RPCPort != 0 {
			cfg.RPC.RPCURL = fmt.Sprintf("http://localhost:%d", env.Docker.RPCPort)
		}
		// Docker mode: use cmd fields, clear path fields.
		cfg.Metadata.BinaryVersionCmd = fmt.Sprintf("docker exec %s gnoland version", env.Docker.ContainerName)
		cfg.Metadata.BinaryPath = ""
		cfg.Metadata.ConfigGetCmd = fmt.Sprintf("docker exec %s gnoland config get %%s --raw", env.Docker.ContainerName)
		cfg.Metadata.ConfigPath = ""
	} else if env.Journald != nil {
		cfg.Logs.Source = "journald"
		cfg.Logs.JournaldUnit = env.Journald.UnitName
		cfg.Logs.ContainerName = ""
		cfg.Resources.ContainerName = ""
		cfg.Resources.Source = "host"
	}

	// Binary probe results apply to path-mode metadata (when not in docker mode).
	if env.Binary != nil && env.Docker == nil {
		cfg.Metadata.BinaryPath = env.Binary.Path
		if env.Binary.GenesisPath != "" {
			cfg.Metadata.GenesisPath = env.Binary.GenesisPath
		}
		if env.Binary.ConfigPath != "" {
			cfg.Metadata.ConfigPath = env.Binary.ConfigPath
		}
	}
}
