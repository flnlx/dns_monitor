@echo off
cd /d "%~dp0"
echo DNS Monitor - Windows Service
echo 1. Install (automatic delayed start)
echo 2. Start
echo 3. Stop
echo 4. Restart
echo 5. Uninstall
echo 6. Status
choice /c 123456 /m "Select action"
if errorlevel 6 goto status
if errorlevel 5 set "DNS_ACTION=uninstall"
if errorlevel 5 goto run
if errorlevel 4 set "DNS_ACTION=restart"
if errorlevel 4 goto run
if errorlevel 3 set "DNS_ACTION=stop"
if errorlevel 3 goto run
if errorlevel 2 set "DNS_ACTION=start"
if errorlevel 2 goto run
set "DNS_ACTION=install"
:run
powershell.exe -NoProfile -Command "$p = Join-Path (Get-Location) 'dns-monitor.exe'; Start-Process -FilePath $p -ArgumentList @('service', $env:DNS_ACTION) -Verb RunAs -WindowStyle Hidden -Wait"
echo See logs\service-action.log for the result.
pause
exit /b
:status
dns-monitor.exe service status
pause
