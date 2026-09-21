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
powershell.exe -NoProfile -ExecutionPolicy Bypass -Command ^
  "$ErrorActionPreference='Stop';" ^
  "$e=(Join-Path (Get-Location) 'dns-monitor.exe');" ^
  "$act=$env:DNS_ACTION;" ^
  "$admin=(New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator);" ^
  "if($admin){" ^
  "  & $e service $act; $code=$LASTEXITCODE" ^
  "}else{" ^
  "  try{ $p=Start-Process -FilePath $e -ArgumentList @('service',$act) -Verb RunAs -WindowStyle Hidden -Wait -PassThru -ErrorAction Stop; $code=$p.ExitCode }" ^
  "  catch{ Write-Host '[FAIL] Elevation was not completed (UAC cancelled or the account lacks administrator rights).' -ForegroundColor Red; $code=-1 }" ^
  "};" ^
  "if($code -eq 0){ Write-Host ('[OK] Service action ''{0}'' completed.' -f $act) -ForegroundColor Green }" ^
  "elseif($code -ne -1){ Write-Host ('[FAIL] Service action ''{0}'' failed with exit code {1}. See logs\service-action.log.' -f $act,$code) -ForegroundColor Red };" ^
  "$log=Join-Path (Get-Location) 'logs\service-action.log';" ^
  "if(Test-Path $log){ Write-Host '--- Recent result (service-action.log) ---'; Get-Content $log -Tail 3 };" ^
  "Write-Host '--- Current service status ---';" ^
  "& $e service status"
pause
exit /b
:status
dns-monitor.exe service status
pause