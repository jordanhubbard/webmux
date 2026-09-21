[CmdletBinding()]
param([string]$RuntimeModule = (Join-Path $PSScriptRoot '..\webmux\service\runtime.psm1'))
$ErrorActionPreference = 'Stop'
Import-Module $RuntimeModule -Force
$fixture = Join-Path ([IO.Path]::GetTempPath()) "WebMux runtime & spaces [$([Guid]::NewGuid())]"
function Assert-Equal($Actual, $Expected) {
  if ($Actual -cne $Expected) { throw "Expected '$Expected', received '$Actual'" }
}
function Assert-Fails([scriptblock]$Operation) {
  $failed = $false
  try { & $Operation | Out-Null } catch { $failed = $true }
  if (-not $failed) { throw 'Expected runtime selection to fail.' }
}
try {
  [IO.Directory]::CreateDirectory((Join-Path $fixture 'bin')) | Out-Null
  [IO.Directory]::CreateDirectory((Join-Path $fixture 'backend\dist')) | Out-Null
  $native = Join-Path $fixture 'bin\webmux.exe'
  $legacy = Join-Path $fixture 'backend\dist\index.js'
  Assert-Fails { Get-WebMuxServiceRuntime -ApplicationDirectory $fixture }
  [IO.File]::WriteAllText($legacy, '')
  Assert-Fails { Get-WebMuxServiceRuntime -ApplicationDirectory $fixture }
  [IO.File]::WriteAllText($native, '')
  $previousPath = $env:PATH
  try {
    # Native selection must work even when no Node executable is discoverable.
    $env:PATH = ''
    $runtime = Get-WebMuxServiceRuntime -ApplicationDirectory $fixture
    Assert-Equal $runtime.Backend 'go'
    Assert-Equal $runtime.Executable $native
    Assert-Equal $runtime.Arguments ''
  } finally { $env:PATH = $previousPath }
  Remove-Item -LiteralPath $native -Force
  [IO.Directory]::CreateDirectory($native) | Out-Null
  Assert-Fails { Get-WebMuxServiceRuntime -ApplicationDirectory $fixture }
  Write-Host 'Go service resolution passed: no Node dependency, no legacy fallback, missing files and spaced paths.'
} finally { Remove-Item -LiteralPath $fixture -Recurse -Force }
