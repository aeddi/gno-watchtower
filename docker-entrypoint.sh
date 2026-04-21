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
  config_dir="$(dirname "$CONFIG_PATH")"
  if [ ! -f "$CONFIG_PATH" ]; then
    mkdir -p "$config_dir"
    # Pre-flight writability check — catches volume-mounted-as-root-but-image-
    # runs-as-UID-10001 footguns with a clear error instead of a cryptic
    # mid-bootstrap EACCES.
    if [ ! -w "$config_dir" ]; then
      printf 'entrypoint: %s not writable by UID %s — chown the mount or drop the --user override\n' \
        "$config_dir" "$(id -u)" >&2
      exit 1
    fi
    # Atomic write: unique tempfile via `mktemp` (survives concurrent first
    # runs; predictable $$ names would race), cleanup trap so a SIGTERM
    # between generate and `mv` does not leak the tempfile, rename on success.
    # A crash mid-generation leaves the old file (or nothing) in place rather
    # than a half-written config that would trap the daemon in a parse-error
    # crash loop even after restart.
    tmp="$(mktemp "${CONFIG_PATH}.tmp.XXXXXX")" || {
      printf 'entrypoint: mktemp failed in %s\n' "$config_dir" >&2
      exit 1
    }
    trap 'rm -f "$tmp"' EXIT INT TERM
    if /usr/local/bin/"$BIN" generate-config "$tmp"; then
      mv -f "$tmp" "$CONFIG_PATH"
      trap - EXIT INT TERM
      printf 'entrypoint: wrote default config to %s — edit <placeholders> (url, token, ...) so the daemon can start\n' "$CONFIG_PATH" >&2
    else
      printf 'entrypoint: generate-config failed; not exec-ing daemon\n' >&2
      exit 1
    fi
  fi
  case "$BIN" in
  sentinel | beacon)
    if [ ! -f "$KEYS_DIR/privkey" ]; then
      # `mkdir` (without -p) is our concurrency lock: if two containers race
      # on the same empty volume, only one creates the dir; the other sees
      # EEXIST and skips keygen. Parent must exist, so ensure it first.
      mkdir -p "$(dirname "$KEYS_DIR")"
      if ! mkdir "$KEYS_DIR" 2>/dev/null; then
        # Another container won the race or the dir exists but is empty of
        # keys (operator-created empty dir). Fall through to keygen; `keygen`
        # below writes privkey with O_EXCL-equivalent semantics via os.WriteFile
        # atop an existing empty dir.
        :
      fi
      if ! /usr/local/bin/"$BIN" keygen "$KEYS_DIR"; then
        printf 'entrypoint: keygen failed; not exec-ing daemon\n' >&2
        exit 1
      fi
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
