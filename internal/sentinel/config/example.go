package config

// Example is the annotated example TOML config printed by `sentinel generate-config`.
const Example = `[server]
url   = "https://monitoring.example.com"
token = "secret-validator-token"

[rpc]
enabled                       = true
poll_interval                 = "3s"
rpc_url                       = "http://localhost:26657"
dump_consensus_state_interval = "30s"
`
