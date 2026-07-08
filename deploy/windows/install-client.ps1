# Install EarsAudioBridge client as a Windows service via NSSM (runs as a specific user).
# Audio needs the desktop session — prefer register-client-task.ps1 for normal use.
#
# Run from an elevated PowerShell prompt, then set the password:
#   Set-ExecutionPolicy -Scope Process Bypass
#   .\install-client.ps1 -InstallDir "C:\tools\EarsAudioBridge"
#   nssm set EarsAudioBridgeClient ObjectName .\YourUser YourPassword

param(
    [string]$InstallDir = "C:\tools\EarsAudioBridge",
    [string]$ServiceName = "EarsAudioBridgeClient",
    [string]$RunAsUser = $env:USERNAME
)

$ErrorActionPreference = "Stop"
$nssm = Get-Command nssm -ErrorAction SilentlyContinue
if (-not $nssm) {
    Write-Error "nssm not found. Install with: winget install nssm"
}

$exe = Join-Path $InstallDir "earsaudiobridge-client.exe"
$cfg = Join-Path $InstallDir "client.yaml"
if (-not (Test-Path $exe)) { Write-Error "Missing $exe — copy the binary and client.yaml into $InstallDir first." }
if (-not (Test-Path $cfg)) { Write-Error "Missing $cfg" }

& nssm stop $ServiceName 2>$null
& nssm remove $ServiceName confirm 2>$null

& nssm install $ServiceName $exe "--config" $cfg
& nssm set $ServiceName AppDirectory $InstallDir
& nssm set $ServiceName DisplayName "Ears Audio Bridge Client"
& nssm set $ServiceName Description "Plays EarsAudioBridge stream and optional mic return"
& nssm set $ServiceName Start SERVICE_AUTO_START
& nssm set $ServiceName ObjectName ".\$RunAsUser"
Write-Warning "Set password: nssm set $ServiceName ObjectName .\$RunAsUser <password>"
& nssm set $ServiceName AppStdout (Join-Path $InstallDir "client.log")
& nssm set $ServiceName AppStderr (Join-Path $InstallDir "client.log")
& nssm set $ServiceName AppRotateFiles 1
& nssm set $ServiceName AppRotateBytes 1048576

& nssm start $ServiceName
Write-Host "Service $ServiceName installed. Set password if prompted, then: nssm start $ServiceName"
