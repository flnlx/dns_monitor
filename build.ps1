param([switch]$SkipTests)
$ErrorActionPreference = 'Stop'
# Keep script source ASCII for Windows PowerShell 5.1 ANSI decoding.
$readmeFileName = (-join [char[]](0x4F7F, 0x7528, 0x8BF4, 0x660E)) + '.md'
$validationFileName = (-join [char[]](0x9A8C, 0x6536, 0x8BB0, 0x5F55)) + '.md'
Set-Location -LiteralPath $PSScriptRoot
$goCmd = Get-Command go -ErrorAction SilentlyContinue
$goExe = if ($goCmd) { $goCmd.Source } elseif (Test-Path -LiteralPath (Join-Path $PSScriptRoot '.tools\go\bin\go.exe')) { Join-Path $PSScriptRoot '.tools\go\bin\go.exe' } else { Join-Path $PSScriptRoot '.tools\go\go\bin\go.exe' }
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
New-Item -ItemType Directory -Force dist\DNSMonitor\licenses | Out-Null
& $goExe build -trimpath -ldflags '-s -w' -o dist\DNSMonitor\dns-monitor.exe ./cmd/dns-monitor
if ($LASTEXITCODE -ne 0) { throw 'Build failed' }
Copy-Item -LiteralPath README.md -Destination (Join-Path 'dist\DNSMonitor' $readmeFileName) -Force
Copy-Item -LiteralPath LICENSE -Destination dist\DNSMonitor\LICENSE -Force
Copy-Item -LiteralPath start.cmd,manage-service.cmd -Destination dist\DNSMonitor -Force
if (Test-Path -LiteralPath VALIDATION.md) { Copy-Item -LiteralPath VALIDATION.md -Destination (Join-Path 'dist\DNSMonitor' $validationFileName) -Force }
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
# Stage a clean release tree so the portable ZIP never carries runtime state
# (monitor.db, access-key.txt, instance.lock, *.log). The deployed
# dist\DNSMonitor folder keeps any existing data untouched and in place.
$stageRoot = Join-Path $PSScriptRoot 'dist\.staging'
$stageDir = Join-Path $stageRoot 'DNSMonitor'
if (Test-Path -LiteralPath $stageRoot) { Remove-Item -LiteralPath $stageRoot -Recurse -Force }
New-Item -ItemType Directory -Force (Join-Path $stageDir 'licenses') | Out-Null
Copy-Item -LiteralPath 'dist\DNSMonitor\dns-monitor.exe' -Destination $stageDir -Force
Copy-Item -LiteralPath README.md -Destination (Join-Path $stageDir $readmeFileName) -Force
Copy-Item -LiteralPath LICENSE -Destination $stageDir -Force
Copy-Item -LiteralPath start.cmd,manage-service.cmd -Destination $stageDir -Force
if (Test-Path -LiteralPath VALIDATION.md) { Copy-Item -LiteralPath VALIDATION.md -Destination (Join-Path $stageDir $validationFileName) -Force }
Get-ChildItem -LiteralPath 'dist\DNSMonitor\licenses' -File | Copy-Item -Destination (Join-Path $stageDir 'licenses') -Force
Compress-Archive -Path $stageDir -DestinationPath dist\DNSMonitor-windows-amd64.zip -Force
# Regression guard: the release ZIP must never contain runtime data or logs.
Add-Type -AssemblyName System.IO.Compression.FileSystem
$reader = [IO.Compression.ZipFile]::OpenRead((Join-Path $PSScriptRoot 'dist\DNSMonitor-windows-amd64.zip'))
try {
  $bad = @($reader.Entries | ForEach-Object { $_.FullName } | Where-Object { $_ -match '(^|/)(data|logs)/' -or $_ -match '\.(db|db-wal|db-shm)$' })
} finally {
  $reader.Dispose()
}
if ($bad.Count -gt 0) { throw "Refusing release ZIP containing runtime state: $($bad -join ',')" }
Remove-Item -LiteralPath $stageRoot -Recurse -Force
Write-Host 'Portable package: dist\DNSMonitor-windows-amd64.zip'
