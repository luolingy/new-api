@echo off
rem One-click start for new-api. Starts the already-built new-api.exe (no compilation).
setlocal
cd /d "%~dp0"
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0start.ps1"
if errorlevel 1 pause
