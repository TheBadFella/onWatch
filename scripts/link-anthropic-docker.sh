#!/usr/bin/env bash
# Link an Anthropic (Claude) account to the onWatch fork container.
#
# Run on the Docker host, from the folder that holds your compose file:
#   ./link-anthropic-docker.sh [path-to-onwatch-data] [container-name]
# Defaults: ./onwatch-data and onwatch.
#
# The fork image sets HOME=/data/antigravity, so onWatch reads
# /data/antigravity/.claude/.credentials.json. This script logs Claude Code in
# with its config folder pointed there. That login belongs to onWatch alone:
# onWatch renews its tokens without logging out Claude Code anywhere else.
set -euo pipefail

DATA_DIR="${1:-./onwatch-data}"
CONTAINER="${2:-onwatch}"
CLAUDE_DIR="$DATA_DIR/antigravity/.claude"
SUDO=""
if [ "$(id -u)" -ne 0 ]; then SUDO="sudo"; fi

if [ ! -d "$DATA_DIR" ]; then
  echo "Data folder $DATA_DIR not found. Pass the folder mounted at /data." >&2
  exit 1
fi

if ! command -v claude >/dev/null 2>&1; then
  echo "Installing Claude Code..."
  curl -fsSL https://claude.ai/install.sh | bash
  export PATH="$HOME/.local/bin:$PATH"
fi

# Log in as the current user, then hand the folder to the container user.
$SUDO mkdir -p "$CLAUDE_DIR"
$SUDO chown -R "$(id -u):$(id -g)" "$CLAUDE_DIR"
CLAUDE_DIR_ABS="$(cd "$CLAUDE_DIR" && pwd)"

if [ ! -s "$CLAUDE_DIR_ABS/.credentials.json" ]; then
  echo
  echo "Claude Code will open. Type /login, sign in with the account to track,"
  echo "paste the code back if asked, then type /exit."
  echo
  CLAUDE_CONFIG_DIR="$CLAUDE_DIR_ABS" CLAUDE_SECURESTORAGE_CONFIG_DIR="$CLAUDE_DIR_ABS" claude
fi

if [ ! -s "$CLAUDE_DIR_ABS/.credentials.json" ]; then
  echo "No credentials were saved in $CLAUDE_DIR_ABS. Run the script again and finish /login." >&2
  exit 1
fi

chmod 700 "$CLAUDE_DIR_ABS"
chmod 600 "$CLAUDE_DIR_ABS/.credentials.json"
$SUDO chown -R 65532:65532 "$CLAUDE_DIR_ABS"
echo "Credentials saved. Restarting $CONTAINER so onWatch detects them..."
docker restart "$CONTAINER" >/dev/null
sleep 5
if docker logs --since 1m "$CONTAINER" 2>&1 | grep -qi "anthropic"; then
  echo "Anthropic is linked. Open the dashboard and pick the Anthropic tab."
else
  echo "Restarted. If the Anthropic tab does not appear, run: docker logs $CONTAINER | grep -i anthropic"
fi
