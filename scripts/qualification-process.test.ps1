$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'qualification-process.ps1')
$root = Join-Path ([IO.Path]::GetTempPath()) "webmux-deadline-test-$([Guid]::NewGuid())"
[IO.Directory]::CreateDirectory($root) | Out-Null
$pwsh = (Get-Process -Id $PID).Path
try {
  $fixture = Join-Path $root 'fixture with spaces.ps1'
  [IO.File]::WriteAllText($fixture, @'
param([string]$Value, [string]$OutputFile, [int]$Code)
[IO.File]::WriteAllText($OutputFile, $Value)
exit $Code
'@)
  $output = Join-Path $root 'argument.txt'
  $value = 'private argument "with quotes" and spaces'
  Invoke-QualificationProcess -FilePath $pwsh -Arguments @('-NoProfile', '-File', $fixture, '-Value', $value, '-OutputFile', $output, '-Code', '0') `
    -TimeoutSeconds 20 -DiagnosticsDirectory $root -Phase 'success'
  if ([IO.File]::ReadAllText($output) -cne $value) { throw 'Process arguments were not preserved.' }
  $failed = $false
  try {
    Invoke-QualificationProcess -FilePath $pwsh -Arguments @('-NoProfile', '-File', $fixture, '-Value', $value, '-OutputFile', $output, '-Code', '42') `
      -TimeoutSeconds 20 -DiagnosticsDirectory $root -Phase 'failure'
  } catch { if ($_.Exception.Message -notmatch 'exited 42') { throw }; $failed = $true }
  if (-not $failed) { throw 'Nonzero exit was admitted.' }

  # A real child of the stalled process must also disappear at the deadline.
  $childPidFile = Join-Path $root 'child.pid'
  $hung = Join-Path $root 'hung.ps1'
  [IO.File]::WriteAllText($hung, @'
param([string]$PidFile)
$child = Start-Process (Get-Process -Id $PID).Path -ArgumentList @('-NoProfile', '-Command', 'Start-Sleep 90') -PassThru
[IO.File]::WriteAllText($PidFile, [string]$child.Id)
Start-Sleep 90
'@)
  $timer = [Diagnostics.Stopwatch]::StartNew()
  $timedOut = $false
  try {
    Invoke-QualificationProcess -FilePath $pwsh -Arguments @('-NoProfile', '-File', $hung, '-PidFile', $childPidFile) `
      -TimeoutSeconds 5 -DiagnosticsDirectory $root -Phase 'hung'
  } catch [TimeoutException] { $timedOut = $true }
  if (-not $timedOut -or $timer.Elapsed.TotalSeconds -gt 15) { throw 'Hung fixture was not bounded.' }
  if (-not (Test-Path -LiteralPath $childPidFile)) { throw 'Hung fixture never started its descendant.' }
  $childId = [int][IO.File]::ReadAllText($childPidFile)
  $deadline = [DateTime]::UtcNow.AddSeconds(5)
  while ((Get-Process -Id $childId -ErrorAction SilentlyContinue) -and [DateTime]::UtcNow -lt $deadline) { Start-Sleep -Milliseconds 100 }
  if (Get-Process -Id $childId -ErrorAction SilentlyContinue) { throw 'Timed-out process left a running descendant.' }
  $raw = [IO.File]::ReadAllText((Join-Path $root 'progress.jsonl'))
  if ($raw.Contains($value)) { throw 'Progress diagnostics exposed arguments.' }
  $records = @($raw.Trim().Split("`n") | ForEach-Object { $_ | ConvertFrom-Json })
  if (-not ($records | Where-Object { $_.phase -eq 'failure' -and $_.status -eq 'exited' -and $_.exitCode -eq 42 })) { throw 'Missing nonzero exit evidence.' }
  if (-not ($records | Where-Object { $_.phase -eq 'hung' -and $_.status -eq 'timeout' })) { throw 'Missing timeout evidence.' }
  if (-not ($records | Where-Object { $_.phase -eq 'hung' -and $_.status -eq 'terminated' })) { throw 'Missing termination evidence.' }
  Write-Host 'Qualification deadlines passed: argument fidelity, exit status, hung process tree and private progress diagnostics.'
} finally {
  Remove-Item -LiteralPath $root -Recurse -Force
}
