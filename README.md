# Ears Audio Bridge

Network audio bridge in Go. The server exposes a virtual sound output on its host; anything played to it is streamed to connected clients, which play it back like a normal media application. Optionally each client can send its microphone back, and it appears on the server as a normal input source.

## Platform / backend matrix

The whole project is pure Go — no cgo on any platform, so `GOOS=windows go build` and `GOOS=darwin go build` work from any machine with just the Go toolchain.

| Role | Linux (PipeWire) | Linux (PulseAudio) | Windows | macOS |
|---|---|---|---|---|
| Server | `pipewire` backend (preferred; native PipeWire socket + pulse protocol via pipewire-pulse) | `pulse` backend (native pulse protocol) | `native`: WASAPI loopback capture (go-wca, COM via syscall); mic return plays into VB-Cable | not available (no pure-Go CoreAudio capture) |
| Client playback | `pulse` backend (via pipewire-pulse) | `pulse` backend | `native` WASAPI (device selectable) | `native` CoreAudio via oto/purego (default device only) |
| Client mic return | yes | yes | yes (WASAPI capture, device selectable) | no |
| Virtual devices created automatically | yes | yes | no (VB-Cable required for mic return) | — |
| Volume/mute from web UI | yes | yes | no — use OS mixer | — |

`backend: auto` on Linux picks **PipeWire** when its unix socket is reachable **and** pipewire-pulse answers; otherwise **PulseAudio** on older systems. Audio never shells out to `pactl`, `pacat`, `pw-cli`, or `wpctl` — everything uses the native Pulse protocol ([`jfreymuth/pulse`](https://github.com/jfreymuth/pulse)) over a unix socket in-process.

Windows cannot create sound devices from userspace, so the mic return path there depends on pre-installed VB-Cable named in the `native:` config section. macOS has no server role and no mic capture — accepted tradeoffs of `CGO_ENABLED=0`; fixing that would need cgo/malgo or a purego CoreAudio capture layer.

## Runtime: what talks to the OS

Nothing spawns audio CLI tools. The only optional subprocess is opening the web UI in a browser from the system tray.

| OS | Audio (server + client) | Tray (`--tray`) | Open web UI (tray) |
|---|---|---|---|
| **Linux** | In-process Pulse protocol socket (pipewire-pulse or PulseAudio) | D-Bus StatusNotifier (no exec) | `xdg-open` |
| **Windows** | In-process WASAPI via COM (`go-wca`) | Win32 `Shell_NotifyIcon` (no exec) | `rundll32 url.dll,…` |
| **macOS** | In-process CoreAudio via `oto`/purego (playback only) | Not available in static builds | `open` |

| OS | Sound server required at runtime |
|---|---|
| **Linux (PipeWire)** | PipeWire daemon + **pipewire-pulse** |
| **Linux (Pulse-only)** | PulseAudio daemon |
| **Windows** | WASAPI; VB-Cable (or similar) for server mic return |
| **macOS** | CoreAudio (built-in) |

## Requirements

- **Go 1.26.4+** — all platforms build with `CGO_ENABLED=0` (no gcc, mingw, or Xcode)
- **Linux server:** PipeWire + pipewire-pulse, or PulseAudio / pipewire-pulse alone
- **Linux client:** a running Pulse-compatible server (PipeWire via pipewire-pulse, or PulseAudio)
- **Windows:** Windows 10+ with WASAPI; VB-Cable for server mic return
- **macOS client:** macOS with CoreAudio (default output only)

## Build

```
make            # all platforms into builds/
# or
go build ./cmd/earsaudiobridge-server ./cmd/earsaudiobridge-client
```

Cross-builds from Linux: `GOOS=windows go build ./cmd/...` and `GOOS=darwin GOARCH=arm64 go build ./cmd/earsaudiobridge-client` — no extra toolchain.

## Configuration

Place `server.yaml` and `client.yaml` next to the binary (or in the working directory you start it from). Override with `--config`:

```
./earsaudiobridge-server --config server.yaml
./earsaudiobridge-client --config client.yaml
```

Relative paths resolve against the current directory at startup. Both bind to all interfaces by default (`0.0.0.0:4600` stream, `0.0.0.0:4680` web UI). Set `server` in the client config to the server's **LAN IP** (not `127.0.0.1`) when the client runs on another machine.

## Run

```
./earsaudiobridge-server
./earsaudiobridge-client
```

Or from source:

```
make run-server
make run-client
```

## Running as a service

Audio bridges must reach the session sound server (PipeWire, PulseAudio, WASAPI, CoreAudio). **Per-user** integration is the default everywhere:

| OS | Default | Why not system-wide? |
|---|---|---|
| Linux | **systemd user units** (`systemctl --user`) | PipeWire/Pulse live in your login session |
| macOS | **LaunchAgents** | CoreAudio session is per user |
| Windows | **Task Scheduler at logon** | WASAPI needs the interactive desktop |

Do **not** use Linux system units (`sudo systemctl enable …`), macOS LaunchDaemons, or Windows **LocalSystem** unless you know audio will work there. See [`deploy/README.md`](deploy/README.md) for paths and scripts.

Example install layout (Linux / macOS):

| Path | Purpose |
|---|---|
| `~/.local/bin/earsaudiobridge-server` | server binary |
| `~/.local/bin/earsaudiobridge-client` | client binary |
| `~/.config/earsaudiobridge/server.yaml` | server config (Linux) |
| `~/.config/earsaudiobridge/client.yaml` | client config (Linux) |
| `~/Library/Application Support/EarsAudioBridge/` | config (macOS) |
| `%LOCALAPPDATA%\EarsAudioBridge\` | binaries + yaml (Windows, suggested) |

Unit files and helper scripts live under `deploy/`.

### Linux — systemd user units (default)

```bash
make host
chmod +x deploy/linux/install-user-units.sh
./deploy/linux/install-user-units.sh

# or install and enable in one step:
ENABLE=both ./deploy/linux/install-user-units.sh

systemctl --user enable --now earsaudiobridge-server
systemctl --user enable --now earsaudiobridge-client   # optional

systemctl --user status earsaudiobridge-server
journalctl --user -u earsaudiobridge-server -f
```

To keep services running after logout (headless server box):

```bash
loginctl enable-linger "$USER"
```

Manual install without the script:

```bash
install -m 755 earsaudiobridge-server ~/.local/bin/
install -m 755 earsaudiobridge-client ~/.local/bin/
mkdir -p ~/.config/earsaudiobridge
cp server.yaml client.yaml ~/.config/earsaudiobridge/
cp deploy/systemd/user/*.service ~/.config/systemd/user/
systemctl --user daemon-reload
systemctl --user enable --now earsaudiobridge-server
```

### macOS — launchd (LaunchAgents)

There is no macOS **server** runtime (no capture backend). Only the **client** agent is provided.

```bash
make darwin
chmod +x deploy/macos/install-client.sh
./deploy/macos/install-client.sh
```

Manual control:

```bash
launchctl kickstart -k "gui/$(id -u)/com.earentir.earsaudiobridge.client"
launchctl bootout "gui/$(id -u)" ~/Library/LaunchAgents/com.earentir.earsaudiobridge.client
```

Edit `~/Library/Application Support/EarsAudioBridge/client.yaml`, then restart the agent.

### Windows — Task Scheduler (default, per user)

Audio APIs need your logged-in desktop session. **Task Scheduler → At logon** under your account is the default (no admin, no extra tools).

Copy binaries + yaml into a folder you own (e.g. `%LOCALAPPDATA%\EarsAudioBridge`), edit the yaml, then:

```powershell
cd deploy\windows
Set-ExecutionPolicy -Scope Process Bypass
.\register-server-task.ps1 -InstallDir "$env:LOCALAPPDATA\EarsAudioBridge"
.\register-client-task.ps1 -InstallDir "$env:LOCALAPPDATA\EarsAudioBridge"
```

Manage tasks: `taskschd.msc` → **EarsAudioBridgeServer** / **EarsAudioBridgeClient**, or:

```powershell
Start-ScheduledTask -TaskName EarsAudioBridgeClient
Stop-ScheduledTask -TaskName EarsAudioBridgeClient
Unregister-ScheduledTask -TaskName EarsAudioBridgeClient -Confirm:$false
```

### Windows — Startup folder (simple alternative)

If you prefer **Settings → Apps → Startup** (or the legacy Startup folder) over Task Scheduler, use the helper batch files in `deploy\windows\`:

1. Copy `earsaudiobridge-client.exe`, `client.yaml`, and `start-client.bat` into one folder (e.g. `%LOCALAPPDATA%\EarsAudioBridge`). The batch file uses its own directory (`%~dp0`), so paths do not need editing.
2. Add a shortcut to `start-client.bat` in Startup:
   - Press **Win+R**, run `shell:startup`, and drop the shortcut there, **or**
   - **Settings → Apps → Startup → Add** and pick the batch file or shortcut.
3. For a server on the same PC, repeat with `start-server.bat`, `earsaudiobridge-server.exe`, and `server.yaml`.

Windows binaries are built as GUI apps (no console window). The batch file exits immediately; only the tray icon (client) or background process (server) remains. The client launcher passes `--tray`. For server logs when running manually, use Task Scheduler or NSSM (they can capture output to a file).

Do not register both Task Scheduler **and** Startup for the same role — pick one mechanism.

### Windows — NSSM (optional, not the default)

For a classic Windows Service (e.g. server that must start before anyone logs in), use [NSSM](https://nssm.cc/). The **client** should still run as your user, not LocalSystem:

```powershell
winget install nssm
cd deploy\windows
.\install-server.ps1 -InstallDir "C:\Program Files\EarsAudioBridge"
# Client must run as your user (set password after install):
.\install-client.ps1 -InstallDir "C:\tools\EarsAudioBridge"
nssm set EarsAudioBridgeClient ObjectName .\YourUser YourPassword
```

Logs go next to the install dir (`server.log` / `client.log`). NSSM commands: `nssm status EarsAudioBridgeServer`, `nssm restart EarsAudioBridgeServer`.

### Getting audio through (Linux server)

1. Start the server.
2. In the host sound settings (or `pavucontrol` / GNOME Settings), set the default **output** to **EarsAudioBridge_Output** (`earsaudiobridge`).
3. Play something — browser, music app, anything routed to that output.
4. Start the client on the remote machine; it should log `connected` and `playback via … backend`.
5. Optional: open the web UI at `http://<server-ip>:4680` — the **Stream source** dropdown should show `earsaudiobridge.monitor` (or `earsaudiobridge`) with a **STREAMING** tag on the matching source.

The server captures the **monitor** of the virtual sink (what is being played to it), not the machine's speakers directly. If you hear nothing on the client, check that audio is actually going to EarsAudioBridge_Output and that the web UI shows at least one connected client.

### Firewall (Fedora / firewalld)

The server listens on all interfaces, but the host firewall must allow inbound traffic on the **active zone** for your network interface (often `internal` or `public`, not necessarily the default `FedoraWorkstation` zone):

```bash
# check which zone is on your NIC
sudo firewall-cmd --get-active-zones

# open ports on that zone (example: internal)
sudo firewall-cmd --permanent --zone=internal --add-port=4600/tcp
sudo firewall-cmd --permanent --zone=internal --add-port=4680/tcp
sudo firewall-cmd --reload
```

From a client machine: `curl http://<server-ip>:4680/` should return the web UI, and TCP port 4600 should connect.

Devices the server creates on Linux (both backends use Pulse modules; pipewire-pulse exposes them on PipeWire systems):

| Device | Implementation | Purpose |
|---|---|---|
| `earsaudiobridge` | `module-null-sink` | shows in sound outputs; select it to stream what's played there |
| `earsaudiobridge_mic` | `module-null-sink` (internal) | receives mic PCM from connected clients |
| `earsaudiobridge_mic_src` | `module-remap-source` on mic sink monitor | shows in sound inputs as the remote microphone |

Web UI at `http://<server-ip>:4680`: stream stats (bandwidth, PCM vs wire), compression controls (none/Opus/FLAC), capture target selection, per-device volume/mute (Linux backends), connected client count and per-client egress bandwidth.

The client also serves a web UI on **127.0.0.1:4680** by default (`web.listen` in `client.yaml`): live playback/mic stream stats with bandwidth, connection status, codec in use, and settings (devices, latency, enable playback/mic). Open `http://localhost:4680` on the machine running the client. It binds to localhost only so Windows does not prompt for firewall access; use `0.0.0.0:4680` only if you need the UI from another machine on the LAN.

**System tray** (optional): run the client with `--tray` for a notification-area icon on **Linux** (StatusNotifierItem) and **Windows**. Left-click or **Open web UI** opens the local dashboard (`xdg-open` / `rundll32` / `open`); **Reconnect** forces a new server connection; **Quit** exits. Linux and Windows install scripts enable `--tray` by default. **macOS** static builds do not support tray yet (purego/goffi linker conflict) — use the web UI at `http://localhost:4680` instead. Omit `--tray` for headless setups.

**Multiple clients** can connect simultaneously; each receives an independent copy of the stream. Mic return from several clients at once will interleave — keep mic enabled on one client at a time.

**Compression**: the server → client audio stream can be `none` (raw PCM, ~1536 kbit/s stereo), `opus` (pure Go lossy, selectable bitrate), or `flac` (pure Go lossless, ~40–60% of PCM). Change codec from the server web UI; connected clients receive a reconfig frame and switch decoder automatically without reconnecting.

## How it works

| Path | Flow |
|---|---|
| Output | apps → virtual sink → backend capture (pulse monitor / WASAPI loopback) → optional Opus/FLAC encode → 20 ms frames over TCP → client decode → playback |
| Mic | client mic capture → TCP → server plays into virtual mic sink (pulse / native device) → any app sees it as input |

Framing is `[type:1][len:4 BE][payload]`; the server announces rate/channels in a hello frame and clients follow it. Slow clients drop frames instead of building latency.

## Verified vs untested

Compile status: linux, windows, and darwin (amd64+arm64) all pass `go build` and `go vet` with `CGO_ENABLED=0`. Linux audio uses the native Pulse protocol (`jfreymuth/pulse`); Windows uses `go-wca` over COM; macOS playback uses `oto` v3 over purego — all written against the actual package sources, not from memory.

Runtime audio was not exercised in the CI/build environment (no sound server in the build image). WASAPI loopback and PipeWire monitor capture need smoke tests on real hardware.

## Notes and limitations

- Uncompressed PCM is ~1.5 Mbit/s per client at 48 kHz stereo; Opus at 96 kbit/s is the default WAN-friendly setting. Several clients multiply egress bandwidth on the server (shown as total egress in the web UI).
- No auth on the web UI or the stream port — keep them on a trusted LAN or bind to localhost behind a reverse proxy.
