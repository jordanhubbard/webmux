$ErrorActionPreference = 'Stop'
$root = Join-Path ([IO.Path]::GetTempPath()) "webmux-snapshot-test-$([Guid]::NewGuid())"
try {
  $dataRoot = Join-Path $root 'data'
  $diagnostics = Join-Path $root 'diagnostics'
  $service = Join-Path $dataRoot 'WebMux'
  $fixture = Join-Path $dataRoot 'webmux-service-smoke-owned/logs'
  $unrelated = Join-Path $dataRoot 'unrelated/logs'
  foreach ($directory in @($service, $fixture, $unrelated)) { [IO.Directory]::CreateDirectory($directory) | Out-Null }
  [IO.File]::WriteAllLines((Join-Path $service 'service.log'), [string[]](1..250 | ForEach-Object { "line-$_" }))
  [IO.File]::WriteAllText((Join-Path $fixture 'fixture.log'), 'owned fixture')
  [IO.File]::WriteAllText((Join-Path $unrelated 'private.log'), 'unrelated private data')
  [IO.File]::WriteAllText((Join-Path $service 'private.txt'), 'non-log private data')
  $linkedFixture = Join-Path $dataRoot 'webmux-service-smoke-linked'
  $nestedFixture = Join-Path $dataRoot 'webmux-service-smoke-nested'
  [IO.Directory]::CreateDirectory($nestedFixture) | Out-Null
  $linkType = if ($IsWindows) { 'Junction' } else { 'SymbolicLink' }
  New-Item -ItemType $linkType -Path $linkedFixture -Target (Split-Path $unrelated) | Out-Null
  New-Item -ItemType $linkType -Path (Join-Path $nestedFixture 'logs') -Target $unrelated | Out-Null
  # Exercise the real collector/filesystem with a controlled service query.
  # This function is confined to this test process; no real service is changed.
  function Get-CimInstance {
    param([string]$ClassName, [string]$Filter, [int]$OperationTimeoutSec)
    if ($ClassName -ne 'Win32_Service' -or $Filter -ne "Name='WebMux'" -or $OperationTimeoutSec -ne 10) {
      throw 'Service snapshot escaped its bounded fixed query.'
    }
    [pscustomobject]@{ Name = 'WebMux'; State = 'Stopped'; ExitCode = 0; ServiceSpecificExitCode = 0;
      StartName = 'private-account'; PathName = 'private-command-line' }
  }
  & (Join-Path $PSScriptRoot 'snapshot-msi-diagnostics.ps1') -DiagnosticsDirectory $diagnostics -ProgramDataRoot $dataRoot
  $output = Join-Path $diagnostics 'interrupted-service'
  $status = Get-Content -Raw -LiteralPath (Join-Path $output 'service-status.json') | ConvertFrom-Json
  if (($status.PSObject.Properties.Name | Sort-Object) -join ',' -cne 'ExitCode,Name,ServiceSpecificExitCode,State') {
    throw 'Snapshot exposed non-allowlisted service fields.'
  }
  $logs = @(Get-ChildItem -LiteralPath $output -Filter '*.log')
  if ($logs.Count -ne 2) { throw 'Snapshot did not retain exactly the known fixture logs.' }
  $tail = @(Get-Content -LiteralPath (Join-Path $output '1-service.log'))
  if ($tail.Count -ne 200 -or $tail[0] -cne 'line-51' -or $tail[-1] -cne 'line-250') { throw 'Snapshot did not bound the retained log tail.' }
  $text = (Get-ChildItem -LiteralPath $output -File | ForEach-Object { Get-Content -Raw -LiteralPath $_.FullName }) -join "`n"
  foreach ($private in @('unrelated private data', 'non-log private data', 'private-account', 'private-command-line')) {
    if ($text.Contains($private)) { throw 'Snapshot exposed unrelated/private fixture data.' }
  }
  Write-Host 'Interrupted MSI snapshot passed: fixed bounded service query, field allowlist, known logs and bounded tails.'
} finally {
  if (Test-Path -LiteralPath $root) { Remove-Item -LiteralPath $root -Recurse -Force }
}
