#!/usr/bin/env bash
# Link an Anthropic (Claude) account to the onWatch fork container.
#
# Run on the Docker host:
#   bash link-anthropic-docker.sh [path-to-onwatch-data] [container-name]
# Without a path, the folder mounted at /data in the container is used.
# The container name defaults to onwatch.
#
# The fork image sets HOME=/data/antigravity, so onWatch reads
# /data/antigravity/.claude/.credentials.json. This script logs Claude Code in
# with its config folder pointed there. That login belongs to onWatch alone:
# onWatch renews its tokens without logging out Claude Code anywhere else.
set -euo pipefail

DATA_DIR="${1:-}"
CONTAINER="${2:-onwatch}"
SUDO=""
if [ "$(id -u)" -ne 0 ]; then SUDO="sudo"; fi
# Use sudo for Docker when this user is not in the docker group.
DOCKER="docker"
if ! docker info >/dev/null 2>&1; then DOCKER="$SUDO docker"; fi

# Without a path, ask Docker which host folder the container mounts at /data.
if [ -z "$DATA_DIR" ] || [ ! -d "$DATA_DIR" ]; then
  found="$($DOCKER inspect "$CONTAINER" --format '{{range .Mounts}}{{if eq .Destination "/data"}}{{.Source}}{{end}}{{end}}' 2>/dev/null || true)"
  if [ -n "$found" ]; then
    echo "Using the data folder mounted at /data in $CONTAINER: $found"
    DATA_DIR="$found"
  fi
fi
if [ -z "$DATA_DIR" ] || ! $SUDO test -d "$DATA_DIR"; then
  echo "Could not find onWatch's data folder. Pass it as the first argument," >&2
  echo "or the container name as the second if it is not \"$CONTAINER\" (see: docker ps)." >&2
  exit 1
fi
# A Docker-managed volume sits under a root-only folder; run as root there.
if ! [ -x "$DATA_DIR" ] || ! [ -r "$DATA_DIR" ]; then
  echo "$DATA_DIR is only readable by root. Re-run with: sudo bash $0 \"$DATA_DIR\" $CONTAINER" >&2
  exit 1
fi
CLAUDE_DIR="$DATA_DIR/antigravity/.claude"

if ! command -v claude >/dev/null 2>&1; then
  echo "Installing Claude Code..."
  curl -fsSL https://claude.ai/install.sh | bash
  export PATH="$HOME/.local/bin:$PATH"
fi

# The container's folders are private to its user (65532), so log in through
# a temporary folder this user owns, then copy only the login file into place.
CRED="$CLAUDE_DIR/.credentials.json"
if ! $SUDO test -s "$CRED"; then
  LOGIN_DIR="$(mktemp -d)"
  trap 'rm -rf "$LOGIN_DIR"' EXIT
  echo
  echo "Claude Code will open. Type /login, sign in with the account to track,"
  echo "paste the code back if asked, then type /exit."
  echo
  CLAUDE_CONFIG_DIR="$LOGIN_DIR" CLAUDE_SECURESTORAGE_CONFIG_DIR="$LOGIN_DIR" claude
  if [ ! -s "$LOGIN_DIR/.credentials.json" ]; then
    echo "No login was saved. Run the script again and finish /login before /exit." >&2
    exit 1
  fi
  $SUDO mkdir -p "$CLAUDE_DIR"
  $SUDO install -m 600 -o 65532 -g 65532 "$LOGIN_DIR/.credentials.json" "$CRED"
fi
$SUDO chown 65532:65532 "$DATA_DIR/antigravity" "$CLAUDE_DIR"
$SUDO chmod 700 "$CLAUDE_DIR"
echo "Credentials saved. Restarting $CONTAINER so onWatch detects them..."
$DOCKER restart "$CONTAINER" >/dev/null
sleep 5
if $DOCKER logs --since 1m "$CONTAINER" 2>&1 | grep -qi "anthropic"; then
  echo "Anthropic is linked. Open the dashboard and pick the Anthropic tab."
else
  echo "Restarted. If the Anthropic tab does not appear, run: docker logs $CONTAINER | grep -i anthropic"
fi
