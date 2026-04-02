// internal/sentinel/config/example.go
package config

// Example is the annotated example TOML config printed by `sentinel generate-config`.
const Example = `[server]
url   = "https://monitoring.example.com/watchtower"
token = "secret-validator-token"

[rpc]
enabled                       = true
poll_interval                 = "3s"
rpc_url                       = "http://localhost:26657"
dump_consensus_state_interval = "30s"

[logs]
enabled        = true
source         = "docker"
container_name = "gnoland"
batch_size     = "1MB"
batch_timeout  = "5s"
min_level      = "info"

[otlp]
enabled     = true
listen_addr = "localhost:4317"

[resources]
enabled        = true
poll_interval  = "10s"
source         = "host"
container_name = "gnoland"

[metadata]
enabled        = true
check_interval = "10m"

binary_path          = "/usr/local/bin/gnoland"
# binary_checksum_cmd  = "docker exec gnoland sha256sum $(which gnoland)"

config_path          = "/etc/gnoland/config.toml"
# config_get_cmd       = "docker exec gnoland gnoland config get %s --raw"

genesis_path         = "/etc/gnoland/genesis.json"
# genesis_checksum_cmd = "docker exec gnoland sh -c 'sha256sum $(find . -name genesis.json)'"
`
