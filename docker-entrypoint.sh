#!/bin/sh
# docker-entrypoint.sh — first-run bootstrap for gno-watchtower images.
#
# If the container is started with the default CMD (`run /etc/<bin>/config.toml`)
# and the expected files don't exist yet, generate them before exec'ing the
# binary:
#   - Config: missing → `<bin> generate-config /etc/<bin>/config.toml`.
#   - Keys   (sentinel + beacon only): missing privkey → `<bin> keygen /etc/<bin>/keys`.
#
# Generated configs ship with <angle-bracket> placeholders that validation
# rejects; the daemon will crash-loop with a clear error until the operator
# edits url/token/etc. in the mounted volume.
#
# Ad-hoc subcommands (`version`, `generate-config`, `keygen`, ...) are passed
# through untouched. Custom config paths (`run /custom/path.toml`) also bypass
# bootstrap — callers owning a non-default path manage its lifecycle.
set -eu

BIN="$1"
shift

CONFIG_PATH="/etc/${BIN}/config.toml"
KEYS_DIR="/etc/${BIN}/keys"

# The config path is always the last positional arg to `run`; capture it
# portably (no bashisms).
last_arg=""
for arg in "$@"; do
    last_arg=$arg
done

if [ "${1:-}" = "run" ] && [ "$last_arg" = "$CONFIG_PATH" ]; then
    if [ ! -f "$CONFIG_PATH" ]; then
        mkdir -p "$(dirname "$CONFIG_PATH")"
        /usr/local/bin/"$BIN" generate-config "$CONFIG_PATH"
        echo "entrypoint: wrote default config to $CONFIG_PATH — edit <placeholders> (url, token, ...) so the daemon can start" >&2
    fi
    case "$BIN" in
        sentinel|beacon)
            if [ ! -f "$KEYS_DIR/privkey" ]; then
                mkdir -p "$KEYS_DIR"
                /usr/local/bin/"$BIN" keygen "$KEYS_DIR"
                echo "entrypoint: generated Noise keypair in $KEYS_DIR" >&2
            fi
            ;;
    esac
fi

exec /usr/local/bin/"$BIN" "$@"
