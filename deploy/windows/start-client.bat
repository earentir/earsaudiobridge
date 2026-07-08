@echo off
rem Start EarsAudioBridge client (place next to earsaudiobridge-client.exe and client.yaml).
cd /d "%~dp0"
start "" "%~dp0earsaudiobridge-client.exe" --tray --config "%~dp0client.yaml"
