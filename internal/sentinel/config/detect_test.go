package config

import (
	"testing"
)

func TestApplyDetection_DockerFound(t *testing.T) {
	cfg := DefaultConfig()
	env := &Environment{
		Docker: &DockerResult{
			ContainerName: "gnoland-mainnet",
			RPCPort:       26657,
		},
	}

	applyDetection(cfg, env)

	if cfg.Logs.Source != LogSourceDocker {
		t.Errorf("Logs.Source: got %q, want docker", cfg.Logs.Source)
	}
	if cfg.Logs.ContainerName != "gnoland-mainnet" {
		t.Errorf("Logs.ContainerName: got %q", cfg.Logs.ContainerName)
	}
	if cfg.Resources.ContainerName != "gnoland-mainnet" {
		t.Errorf("Resources.ContainerName: got %q", cfg.Resources.ContainerName)
	}
	if cfg.Resources.Source != ResSourceBoth {
		t.Errorf("Resources.Source: got %q, want both", cfg.Resources.Source)
	}
	if cfg.RPC.RPCURL != "http://localhost:26657" {
		t.Errorf("RPC.RPCURL: got %q", cfg.RPC.RPCURL)
	}
	if cfg.Metadata.ConfigGetCmd == "" {
		t.Error("Metadata.ConfigGetCmd should be set in docker mode")
	}
	if cfg.Metadata.ConfigPath != "" {
		t.Error("Metadata.ConfigPath should be empty in docker mode")
	}
}

func TestApplyDetection_DockerWithCustomPort(t *testing.T) {
	cfg := DefaultConfig()
	env := &Environment{
		Docker: &DockerResult{
			ContainerName: "gnoland-test",
			RPCPort:       36657,
		},
	}

	applyDetection(cfg, env)

	if cfg.RPC.RPCURL != "http://localhost:36657" {
		t.Errorf("RPC.RPCURL: got %q, want http://localhost:36657", cfg.RPC.RPCURL)
	}
}

func TestApplyDetection_JournaldFound(t *testing.T) {
	cfg := DefaultConfig()
	env := &Environment{
		Journald: &JournaldResult{UnitName: "gnoland.service"},
	}

	applyDetection(cfg, env)

	if cfg.Logs.Source != LogSourceJournald {
		t.Errorf("Logs.Source: got %q, want journald", cfg.Logs.Source)
	}
	if cfg.Logs.JournaldUnit != "gnoland.service" {
		t.Errorf("Logs.JournaldUnit: got %q", cfg.Logs.JournaldUnit)
	}
	if cfg.Logs.ContainerName != "" {
		t.Errorf("Logs.ContainerName should be empty, got %q", cfg.Logs.ContainerName)
	}
	if cfg.Resources.Source != ResSourceHost {
		t.Errorf("Resources.Source: got %q, want host", cfg.Resources.Source)
	}
}

func TestApplyDetection_BinaryFound(t *testing.T) {
	cfg := DefaultConfig()
	env := &Environment{
		Binary: &BinaryResult{
			Path:       "/usr/local/bin/gnoland",
			ConfigPath: "/usr/local/bin/gnoland-data/config/config.toml",
		},
	}

	applyDetection(cfg, env)

	if cfg.Metadata.ConfigPath != "/usr/local/bin/gnoland-data/config/config.toml" {
		t.Errorf("Metadata.ConfigPath: got %q", cfg.Metadata.ConfigPath)
	}
}

func TestApplyDetection_DockerPlusJournald_DockerWins(t *testing.T) {
	cfg := DefaultConfig()
	env := &Environment{
		Docker:   &DockerResult{ContainerName: "gnoland-mainnet", RPCPort: 26657},
		Journald: &JournaldResult{UnitName: "gnoland.service"},
	}

	applyDetection(cfg, env)

	if cfg.Logs.Source != LogSourceDocker {
		t.Errorf("Docker should take precedence: Logs.Source got %q", cfg.Logs.Source)
	}
}

func TestApplyDetection_NothingFound(t *testing.T) {
	cfg := DefaultConfig()
	env := &Environment{}

	applyDetection(cfg, env)

	if cfg.Logs.Source != LogSourceDocker {
		t.Errorf("Logs.Source: got %q, want docker (default)", cfg.Logs.Source)
	}
	if cfg.Logs.ContainerName != "<gnoland-container-name>" {
		t.Errorf("Logs.ContainerName: got %q, want placeholder", cfg.Logs.ContainerName)
	}
	if cfg.Metadata.ConfigPath != "<path-to-gnoland-config>" {
		t.Errorf("Metadata.ConfigPath: got %q, want placeholder", cfg.Metadata.ConfigPath)
	}
}
