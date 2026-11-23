# Run only in an owned, deadline-bounded process on a disposable CI runner.
# Retain known fixture logs, never credentials, command lines or service accounts.
param([Parameter(Mandatory)][string]$DiagnosticsDirectory,
      [string]$ProgramDataRoot = $env:ProgramData)
$ErrorActionPreference = 'Stop'
$destination = Join-Path $DiagnosticsDirectory 'interrupted-service'
New-Item -ItemType Directory -Path $destination -Force | Out-Null
Get-CimInstance Win32_Service -Filter "Name='WebMux'" -OperationTimeoutSec 10 |
  Select-Object Name, State, ExitCode, ServiceSpecificExitCode |
  ConvertTo-Json | Set-Content (Join-Path $destination 'service-status.json')
$directories = @((Join-Path $ProgramDataRoot 'WebMux'))
$directories += @(Get-ChildItem -LiteralPath $ProgramDataRoot -Directory -Filter 'webmux-service-smoke-*' |
  Where-Object { -not ($_.Attributes -band [IO.FileAttributes]::ReparsePoint) } |
  ForEach-Object { Join-Path $_.FullName 'logs' })
$index = 0
foreach ($directory in $directories) {
  $index++
  if (Test-Path -LiteralPath $directory) {
    $entry = Get-Item -LiteralPath $directory -Force
    if ($entry.Attributes -band [IO.FileAttributes]::ReparsePoint) { continue }
    Get-ChildItem -LiteralPath $directory -File -Filter '*.log' |
      Where-Object { -not ($_.Attributes -band [IO.FileAttributes]::ReparsePoint) } |
      ForEach-Object { Get-Content -LiteralPath $_.FullName -Tail 200 | Set-Content (Join-Path $destination "$index-$($_.Name)") }
  }
}
