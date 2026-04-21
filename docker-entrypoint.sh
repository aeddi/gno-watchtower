#!/bin/sh
# docker-entrypoint.sh — first-run bootstrap for gno-watchtower images.
#
# If the container is started with the default CMD (`run /etc/<bin>/config.toml`)
# or the common shorthand (`run` with no args) and the expected files don't
# exist yet, generate them before exec'ing the binary:
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

if [ "$#" -eq 0 ]; then
  printf 'entrypoint: missing BIN argument (expected sentinel|beacon|watchtower)\n' >&2
  exit 2
fi

BIN="$1"
shift

# Guard against entrypoint overrides passing an unexpected BIN — the rest of
# this script assumes the binary exists at /usr/local/bin/$BIN.
case "$BIN" in
sentinel | beacon | watchtower) ;;
*)
  printf 'entrypoint: unknown bin %s (expected sentinel|beacon|watchtower)\n' "$BIN" >&2
  exit 2
  ;;
esac

CONFIG_PATH="/etc/${BIN}/config.toml"
KEYS_DIR="/etc/${BIN}/keys"

# Capture the last positional arg (where `run` expects the config path)
# portably — POSIX sh has no `${@: -1}`.
last_arg=""
for arg in "$@"; do
  last_arg=$arg
done

# Treat these as "default bootstrap path":
#   - `run` with no further args (Dockerfile CMD short form)
#   - `run <CONFIG_PATH>` where the last arg is the default config path
# Anything else (custom config path, subcommand like `version`) bypasses
# bootstrap entirely.
if [ "${1:-}" = "run" ] && { [ "$#" -eq 1 ] || [ "$last_arg" = "$CONFIG_PATH" ]; }; then
  if [ ! -f "$CONFIG_PATH" ]; then
    mkdir -p "$(dirname "$CONFIG_PATH")"
    # Atomic write: generate into a sibling tempfile and rename on success. A
    # crash mid-generation leaves the old file (or nothing) in place rather
    # than a half-written config that would trap the daemon in a parse-error
    # crash loop even after restart.
    tmp="${CONFIG_PATH}.tmp.$$"
    if /usr/local/bin/"$BIN" generate-config "$tmp"; then
      mv -f "$tmp" "$CONFIG_PATH"
      printf 'entrypoint: wrote default config to %s — edit <placeholders> (url, token, ...) so the daemon can start\n' "$CONFIG_PATH" >&2
    else
      rm -f "$tmp"
      printf 'entrypoint: generate-config failed; not exec-ing daemon\n' >&2
      exit 1
    fi
  fi
  case "$BIN" in
  sentinel | beacon)
    if [ ! -f "$KEYS_DIR/privkey" ]; then
      mkdir -p "$KEYS_DIR"
      /usr/local/bin/"$BIN" keygen "$KEYS_DIR"
      printf 'entrypoint: generated Noise keypair in %s\n' "$KEYS_DIR" >&2
    fi
    ;;
  esac
fi

# If the caller didn't supply args (i.e. `docker run <img> run` shorthand),
# fall back to the default `run <CONFIG_PATH>` invocation.
if [ "${1:-}" = "run" ] && [ "$#" -eq 1 ]; then
  set -- run "$CONFIG_PATH"
fi

exec /usr/local/bin/"$BIN" "$@"
