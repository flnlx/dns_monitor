@echo off
cd /d "%~dp0"
dns-monitor.exe --open
if errorlevel 1 pause
