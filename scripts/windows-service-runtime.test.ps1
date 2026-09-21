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
  Assert-Fails { Get-WebMuxServiceRuntime -ApplicationDirectory $fixture -Backend go }
  [IO.File]::WriteAllText($legacy, '')
  $runtime = Get-WebMuxServiceRuntime -ApplicationDirectory $fixture
  Assert-Equal $runtime.Backend 'node'
  Assert-Equal $runtime.Arguments ('"' + $legacy + '"')
  if ($runtime.Executable -isnot [string]) { throw 'Service executable must be a single path.' }
  Assert-Equal $runtime.Executable (Get-Command node.exe -CommandType Application | Select-Object -First 1).Source
  [IO.File]::WriteAllText($native, '')
  $previousPath = $env:PATH
  try {
    # Native selection must work even when no Node executable is discoverable.
    $env:PATH = ''
    foreach ($backend in @('auto', 'go')) {
      $runtime = Get-WebMuxServiceRuntime -ApplicationDirectory $fixture -Backend $backend
      Assert-Equal $runtime.Backend 'go'
      Assert-Equal $runtime.Executable $native
      Assert-Equal $runtime.Arguments ''
    }
  } finally { $env:PATH = $previousPath }
  Assert-Equal (Get-WebMuxServiceRuntime -ApplicationDirectory $fixture -Backend node).Backend 'node'
  Remove-Item -LiteralPath $native -Force
  [IO.Directory]::CreateDirectory($native) | Out-Null
  Assert-Fails { Get-WebMuxServiceRuntime -ApplicationDirectory $fixture -Backend go }
  Assert-Equal (Get-WebMuxServiceRuntime -ApplicationDirectory $fixture).Backend 'node'
  Write-Host 'Service runtime selection passed: native without Node, legacy fallback, explicit selection, missing files and spaced paths.'
} finally { Remove-Item -LiteralPath $fixture -Recurse -Force }
