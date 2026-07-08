@echo off
rem Start EarsAudioBridge server (place next to earsaudiobridge-server.exe and server.yaml).
cd /d "%~dp0"
start "" "%~dp0earsaudiobridge-server.exe" --config "%~dp0server.yaml"
