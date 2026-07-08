# Deploy helpers

EarsAudioBridge must run in the **logged-in user's session** so it can reach PipeWire, PulseAudio, WASAPI, or CoreAudio. **Per-user** service integration is the default on every platform. Do not install Linux **system** units (`/etc/systemd/system`) or Windows **LocalSystem** services unless you have a very specific reason and understand that audio will usually not work.

## Defaults by platform

| OS | Default mechanism | Unit / task location | Config |
|---|---|---|---|
| **Linux** | **systemd user units** | `~/.config/systemd/user/` | `~/.config/earsaudiobridge/` |
| **macOS** | **LaunchAgent** (per user) | `~/Library/LaunchAgents/` | `~/Library/Application Support/EarsAudioBridge/` |
| **Windows** | **Task Scheduler at logon** (per user) | Task Scheduler → your account | install dir you choose (e.g. `%LOCALAPPDATA%\EarsAudioBridge`) |

Binaries default to `~/.local/bin/` on Linux and macOS (user install, no `sudo`).

## Linux — systemd user (default)

```bash
make host
./deploy/linux/install-user-units.sh

# enable what you need (both are installed; neither is auto-enabled):
systemctl --user enable --now earsaudiobridge-server
systemctl --user enable --now earsaudiobridge-client

# optional: keep running after logout (headless server box)
loginctl enable-linger "$USER"
```

Or enable during install:

```bash
ENABLE=both ./deploy/linux/install-user-units.sh
# ENABLE=server | client | both
```

There is **no** `deploy/systemd/system/` tree — system-wide units are intentionally not supported.

## macOS — LaunchAgent (client only)

No macOS server runtime. Client agent only:

```bash
make darwin
./deploy/macos/install-client.sh
```

The client agent runs without `--tray` on macOS (static builds use purego for audio, which conflicts with the systray library). Use the local web UI instead. Linux and Windows support `--tray`.

## Windows — Task Scheduler (default)

```powershell
.\deploy\windows\register-server-task.ps1 -InstallDir "$env:LOCALAPPDATA\EarsAudioBridge"
.\deploy\windows\register-client-task.ps1 -InstallDir "$env:LOCALAPPDATA\EarsAudioBridge"
```

**Startup folder alternative:** copy `start-client.bat` (and `start-server.bat` if needed) next to your binaries and yaml, then add a shortcut via `shell:startup` or **Settings → Apps → Startup**. See the main [README](../README.md#windows--startup-folder-simple-alternative).

NSSM scripts (`install-*.ps1`) are **optional** fallbacks for a classic Windows Service; the client should still run as your user account, not LocalSystem.
