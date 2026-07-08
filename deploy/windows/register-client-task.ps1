# Register EarsAudioBridge client to start at user logon (recommended on Windows).
# Audio APIs need the interactive desktop session — prefer this over a SYSTEM service.
#
# Run in PowerShell (no admin required):
#   Set-ExecutionPolicy -Scope Process Bypass
#   .\register-client-task.ps1 -InstallDir "C:\tools\EarsAudioBridge"

param(
    [string]$InstallDir = "C:\tools\EarsAudioBridge",
    [string]$TaskName = "EarsAudioBridgeClient"
)

$ErrorActionPreference = "Stop"
$exe = Join-Path $InstallDir "earsaudiobridge-client.exe"
$cfg = Join-Path $InstallDir "client.yaml"
if (-not (Test-Path $exe)) { Write-Error "Missing $exe" }
if (-not (Test-Path $cfg)) { Write-Error "Missing $cfg" }

$action = New-ScheduledTaskAction -Execute $exe -Argument "--tray --config `"$cfg`"" -WorkingDirectory $InstallDir
$trigger = New-ScheduledTaskTrigger -AtLogOn -User $env:USERNAME
$settings = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -RestartCount 3 -RestartInterval (New-TimeSpan -Minutes 1)
Register-ScheduledTask -TaskName $TaskName -Action $action -Trigger $trigger -Settings $settings -Force | Out-Null
Start-ScheduledTask -TaskName $TaskName
Write-Host "Task $TaskName registered (runs at logon). Start now with: Start-ScheduledTask -TaskName $TaskName"
