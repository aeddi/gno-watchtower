package config

import (
	"strings"
	"testing"

	toml "github.com/pelletier/go-toml/v2"
)

func TestInjectAlternatives_InsertsAfterKey(t *testing.T) {
	input := strings.Join([]string{
		"[metadata]",
		"config_get_cmd = 'docker exec gnoland gnoland config get %s --raw'",
	}, "\n")

	alts := []alternative{
		{
			afterKey: "config_get_cmd",
			comment:  "Alternative: read config file directly",
			key:      "config_path",
			value:    "/etc/gnoland/config.toml",
		},
	}

	result := injectAlternatives(input, alts)

	if !strings.Contains(result, "# Alternative: read config file directly") {
		t.Error("missing config_path alternative comment")
	}
	if !strings.Contains(result, "# config_path = '/etc/gnoland/config.toml'") {
		t.Error("missing commented-out config_path")
	}

	cmdIdx := strings.Index(result, "config_get_cmd")
	altIdx := strings.Index(result, "# config_path")
	if altIdx < cmdIdx {
		t.Error("config_path alternative should appear after config_get_cmd")
	}
}

func TestInjectAlternatives_NoMatch_Passthrough(t *testing.T) {
	input := "[server]\nurl = 'http://example.com'\n"
	result := injectAlternatives(input, []alternative{
		{afterKey: "nonexistent_key", comment: "test", key: "foo", value: "bar"},
	})
	if result != input {
		t.Errorf("expected passthrough, got:\n%s", result)
	}
}

func TestBuildAlternatives_DockerMode(t *testing.T) {
	cfg := DefaultConfig()
	env := &Environment{Docker: &DockerResult{ContainerName: "gnoland"}}
	applyDetection(cfg, env)
	alts := buildAlternatives(cfg, env)

	if len(alts) != 2 {
		t.Fatalf("expected 2 alternatives, got %d", len(alts))
	}
	// Logs: journald_unit as alternative to container_name
	if alts[0].section != "logs" || alts[0].key != "journald_unit" {
		t.Errorf("alt[0]: section=%q key=%q", alts[0].section, alts[0].key)
	}
	// Metadata: config_path as alternative to config_get_cmd
	if alts[1].afterKey != "config_get_cmd" || alts[1].key != "config_path" {
		t.Errorf("alt[1]: afterKey=%q key=%q", alts[1].afterKey, alts[1].key)
	}
}

func TestBuildAlternatives_NativeMode(t *testing.T) {
	cfg := DefaultConfig()
	env := &Environment{}
	applyDetection(cfg, env)
	alts := buildAlternatives(cfg, env)

	if len(alts) != 2 {
		t.Fatalf("expected 2 alternatives, got %d", len(alts))
	}
	// Logs: journald_unit as alternative (default source is docker)
	if alts[0].section != "logs" || alts[0].key != "journald_unit" {
		t.Errorf("alt[0]: section=%q key=%q", alts[0].section, alts[0].key)
	}
	// Metadata: config_get_cmd as alternative to config_path
	if alts[1].afterKey != "config_path" || alts[1].key != "config_get_cmd" {
		t.Errorf("alt[1]: afterKey=%q key=%q", alts[1].afterKey, alts[1].key)
	}
}

func TestMarshalDefaultConfig_ValidTOML(t *testing.T) {
	cfg := DefaultConfig()
	data, err := toml.Marshal(cfg)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	// Verify it can be unmarshaled back (valid TOML).
	var roundTrip Config
	if err := toml.Unmarshal(data, &roundTrip); err != nil {
		t.Fatalf("round-trip Unmarshal: %v", err)
	}
	if roundTrip.Server.URL != "<watchtower-server-url>" {
		t.Errorf("round-trip Server.URL: got %q", roundTrip.Server.URL)
	}
	// Verify section and field comment tags are present in output.
	output := string(data)
	if !strings.Contains(output, "# Connection to the watchtower server") {
		t.Error("missing section comment for [server]")
	}
	if !strings.Contains(output, "# docker or journald") {
		t.Error("missing comment for logs.source")
	}
}
