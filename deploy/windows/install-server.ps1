# Install EarsAudioBridge server as a Windows service via NSSM.
# WASAPI usually needs the logged-in desktop — prefer register-server-task.ps1 for most setups.
# This script is for machines where a true Windows Service is required (e.g. headless with care).
#
# Run from an elevated PowerShell prompt:
#   Set-ExecutionPolicy -Scope Process Bypass
#   .\install-server.ps1 -InstallDir "C:\Program Files\EarsAudioBridge"

param(
    [string]$InstallDir = "C:\Program Files\EarsAudioBridge",
    [string]$ServiceName = "EarsAudioBridgeServer"
)

$ErrorActionPreference = "Stop"
$nssm = Get-Command nssm -ErrorAction SilentlyContinue
if (-not $nssm) {
    Write-Error "nssm not found. Install with: winget install nssm"
}

$exe = Join-Path $InstallDir "earsaudiobridge-server.exe"
$cfg = Join-Path $InstallDir "server.yaml"
if (-not (Test-Path $exe)) { Write-Error "Missing $exe — copy the binary and server.yaml into $InstallDir first." }
if (-not (Test-Path $cfg)) { Write-Error "Missing $cfg" }

& nssm stop $ServiceName 2>$null
& nssm remove $ServiceName confirm 2>$null

& nssm install $ServiceName $exe "--config" $cfg
& nssm set $ServiceName AppDirectory $InstallDir
& nssm set $ServiceName DisplayName "Ears Audio Bridge Server"
& nssm set $ServiceName Description "Streams host audio to EarsAudioBridge clients"
& nssm set $ServiceName Start SERVICE_AUTO_START
& nssm set $ServiceName AppStdout (Join-Path $InstallDir "server.log")
& nssm set $ServiceName AppStderr (Join-Path $InstallDir "server.log")
& nssm set $ServiceName AppRotateFiles 1
& nssm set $ServiceName AppRotateBytes 1048576

& nssm start $ServiceName
Write-Host "Service $ServiceName installed and started. Logs: $(Join-Path $InstallDir 'server.log')"
