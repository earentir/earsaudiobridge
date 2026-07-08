# Register EarsAudioBridge server to start at user logon (recommended on Windows).
# WASAPI loopback needs the interactive session — prefer this over LocalSystem.
#
# Run in PowerShell (no admin required):
#   Set-ExecutionPolicy -Scope Process Bypass
#   .\register-server-task.ps1 -InstallDir "C:\Program Files\EarsAudioBridge"

param(
    [string]$InstallDir = "C:\Program Files\EarsAudioBridge",
    [string]$TaskName = "EarsAudioBridgeServer"
)

$ErrorActionPreference = "Stop"
$exe = Join-Path $InstallDir "earsaudiobridge-server.exe"
$cfg = Join-Path $InstallDir "server.yaml"
if (-not (Test-Path $exe)) { Write-Error "Missing $exe" }
if (-not (Test-Path $cfg)) { Write-Error "Missing $cfg" }

$action = New-ScheduledTaskAction -Execute $exe -Argument "--config `"$cfg`"" -WorkingDirectory $InstallDir
$trigger = New-ScheduledTaskTrigger -AtLogOn -User $env:USERNAME
$settings = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -RestartCount 3 -RestartInterval (New-TimeSpan -Minutes 1)
Register-ScheduledTask -TaskName $TaskName -Action $action -Trigger $trigger -Settings $settings -Force | Out-Null
Start-ScheduledTask -TaskName $TaskName
Write-Host "Task $TaskName registered (runs at logon). Start now with: Start-ScheduledTask -TaskName $TaskName"
