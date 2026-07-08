#!/usr/bin/env bash
# Install EarsAudioBridge user systemd units (Linux).
# PipeWire/PulseAudio require a user session — do not install as a system unit.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
BIN_DIR="${BIN_DIR:-$HOME/.local/bin}"
CFG_DIR="${CFG_DIR:-$HOME/.config/earsaudiobridge}"
UNIT_DIR="$HOME/.config/systemd/user"

install -d "$BIN_DIR" "$CFG_DIR" "$UNIT_DIR"

for cmd in earsaudiobridge-server earsaudiobridge-client; do
  arch="$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')"
  if [[ "$arch" == "amd64" && -x "$REPO_ROOT/builds/$cmd" ]]; then
    src="$REPO_ROOT/builds/$cmd"
  elif [[ -x "$REPO_ROOT/builds/${cmd}_linux_${arch}" ]]; then
    src="$REPO_ROOT/builds/${cmd}_linux_${arch}"
  elif command -v "$cmd" &>/dev/null; then
    src="$(command -v "$cmd")"
  else
    echo "Build or install $cmd first (make host && cp builds/* $BIN_DIR/)" >&2
    exit 1
  fi
  install -m 755 "$src" "$BIN_DIR/$cmd"
done

[[ -f "$CFG_DIR/server.yaml" ]] || install -m 644 "$REPO_ROOT/server.yaml" "$CFG_DIR/server.yaml"
[[ -f "$CFG_DIR/client.yaml" ]] || install -m 644 "$REPO_ROOT/client.yaml" "$CFG_DIR/client.yaml"

install -m 644 "$REPO_ROOT/deploy/systemd/user/"*.service "$UNIT_DIR/"

ENABLE="${ENABLE:-}"

systemctl --user daemon-reload
echo "Installed binaries to $BIN_DIR and units to $UNIT_DIR"
echo ""
case "$ENABLE" in
  server)
    systemctl --user enable --now earsaudiobridge-server
    echo "Enabled earsaudiobridge-server"
    ;;
  client)
    systemctl --user enable --now earsaudiobridge-client
    echo "Enabled earsaudiobridge-client"
    ;;
  both)
    systemctl --user enable --now earsaudiobridge-server earsaudiobridge-client
    echo "Enabled earsaudiobridge-server and earsaudiobridge-client"
    ;;
  *)
    echo "Units installed but not enabled. Pick one or both:"
    echo "  systemctl --user enable --now earsaudiobridge-server"
    echo "  systemctl --user enable --now earsaudiobridge-client"
    echo ""
    echo "Or re-run with: ENABLE=server|client|both $0"
    ;;
esac
echo ""
echo "For headless / without staying logged in:"
echo "  loginctl enable-linger \"\$USER\""
