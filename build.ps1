param([switch]$SkipTests)
$ErrorActionPreference = 'Stop'
Set-Location -LiteralPath $PSScriptRoot
$goCmd = Get-Command go -ErrorAction SilentlyContinue
$goExe = if ($goCmd) { $goCmd.Source } else { Join-Path $PSScriptRoot '.tools\go\bin\go.exe' }
if (-not (Test-Path -LiteralPath $goExe)) { throw 'Install Go 1.25 or later, or unpack it at .tools\go.' }
$env:GOCACHE = Join-Path $PSScriptRoot '.tools\gocache'
$env:GOMODCACHE = Join-Path $PSScriptRoot '.tools\gomodcache'
$env:CGO_ENABLED = '0'
if (-not $SkipTests) {
  & $goExe test ./...
  if ($LASTEXITCODE -ne 0) { throw 'Tests failed' }
  & $goExe vet ./...
  if ($LASTEXITCODE -ne 0) { throw 'Vet failed' }
}
New-Item -ItemType Directory -Force dist\DNSMonitor\doggo, dist\DNSMonitor\licenses | Out-Null
& $goExe build -trimpath -ldflags '-s -w' -o dist\DNSMonitor\dns-monitor.exe ./cmd/dns-monitor
if ($LASTEXITCODE -ne 0) { throw 'Build failed' }
Copy-Item -LiteralPath doggo\doggo.exe,doggo\LICENSE,doggo\README.md,doggo\DOGGOHELP.TXT -Destination dist\DNSMonitor\doggo -Force
Copy-Item -LiteralPath README.md -Destination dist\DNSMonitor\使用说明.md -Force
Copy-Item -LiteralPath start.cmd,manage-service.cmd -Destination dist\DNSMonitor -Force
if (Test-Path -LiteralPath VALIDATION.md) { Copy-Item -LiteralPath VALIDATION.md -Destination dist\DNSMonitor\验收记录.md -Force }
$moduleList = (& $goExe version -m dist\DNSMonitor\dns-monitor.exe) -join "`n"
if ($LASTEXITCODE -ne 0) { throw 'Cannot collect dependency licenses' }
$moduleDirs = & $goExe list -deps -f '{{with .Module}}{{.Path}}|{{.Dir}}{{end}}' ./cmd/dns-monitor | Sort-Object -Unique
foreach ($line in $moduleDirs) {
  if (-not $line.Trim() -or $line.StartsWith('dnsmonitor|')) { continue }
  $parts = $line.Split('|', 2)
  $label = $parts[0] -replace '[^a-zA-Z0-9.-]', '_'
  Get-ChildItem -LiteralPath $parts[1] -File | Where-Object { $_.Name -match '^(LICENSE|COPYING|NOTICE)' } | ForEach-Object {
    Copy-Item -LiteralPath $_.FullName -Destination (Join-Path 'dist\DNSMonitor\licenses' ($label + '-' + $_.Name)) -Force
  }
}
[IO.File]::WriteAllText((Join-Path $PSScriptRoot 'dist\DNSMonitor\licenses\go-modules.txt'), $moduleList, [Text.UTF8Encoding]::new($false))
Copy-Item -LiteralPath (Join-Path (& $goExe env GOROOT) 'LICENSE') -Destination dist\DNSMonitor\licenses\Go-LICENSE -Force
Compress-Archive -Path dist\DNSMonitor -DestinationPath dist\DNSMonitor-windows-amd64.zip -Force
Write-Host 'Portable package: dist\DNSMonitor-windows-amd64.zip'
