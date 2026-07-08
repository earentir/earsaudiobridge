#!/usr/bin/env bash
# Install EarsAudioBridge launchd agents (macOS). Client only — no macOS server runtime.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
APP_SUPPORT="$HOME/Library/Application Support/EarsAudioBridge"
LAUNCH_AGENTS="$HOME/Library/LaunchAgents"
LOG_DIR="$HOME/Library/Logs/EarsAudioBridge"
BIN_DIR="${BIN_DIR:-$HOME/.local/bin}"

install -d "$APP_SUPPORT" "$LOG_DIR" "$BIN_DIR"

arch="$(uname -m)"
case "$arch" in
  arm64) suffix="darwin_arm64" ;;
  x86_64) suffix="darwin_amd64" ;;
  *) echo "unsupported arch: $arch" >&2; exit 1 ;;
esac

client_src="$REPO_ROOT/builds/earsaudiobridge-client_${suffix}"
[[ -f "$client_src" ]] || client_src="$(command -v earsaudiobridge-client 2>/dev/null || true)"
[[ -n "$client_src" && -f "$client_src" ]] || { echo "build client first: make darwin" >&2; exit 1; }

install -m 755 "$client_src" "$BIN_DIR/earsaudiobridge-client"
[[ -f "$APP_SUPPORT/client.yaml" ]] || install -m 644 "$REPO_ROOT/client.yaml" "$APP_SUPPORT/client.yaml"

plist="$LAUNCH_AGENTS/com.earentir.earsaudiobridge.client.plist"
sed \
  -e "s|/Users/REPLACE_ME|$HOME|g" \
  -e "s|/usr/local/bin/earsaudiobridge-client|$BIN_DIR/earsaudiobridge-client|g" \
  "$REPO_ROOT/deploy/macos/earsaudiobridge-client.plist" > "$plist"

launchctl bootout "gui/$(id -u)" "$plist" 2>/dev/null || true
launchctl bootstrap "gui/$(id -u)" "$plist"
launchctl enable "gui/$(id -u)/com.earentir.earsaudiobridge.client"
launchctl kickstart -k "gui/$(id -u)/com.earentir.earsaudiobridge.client"

echo "Client agent installed: $plist"
echo "Logs: $LOG_DIR/client.log"
