# PowerShell 7 qualification helpers. Records deliberately omit arguments and
# environment: service registration can carry a disposable account password.
function Write-QualificationPhase {
  param([string]$DiagnosticsDirectory, [string]$Phase, [string]$Status,
        [int]$ProcessId = 0, $ExitCode = $null)
  [IO.Directory]::CreateDirectory($DiagnosticsDirectory) | Out-Null
  $record = [ordered]@{ time = [DateTime]::UtcNow.ToString('o'); phase = $Phase;
    status = $Status; pid = $ProcessId; exitCode = $ExitCode }
  $line = $record | ConvertTo-Json -Compress
  [IO.File]::AppendAllText((Join-Path $DiagnosticsDirectory 'progress.jsonl'), $line + "`n")
  Write-Host $line
}

function Invoke-QualificationProcess {
  param([Parameter(Mandatory)][string]$FilePath,
        [string[]]$Arguments = @(),
        [Parameter(Mandatory)][ValidateRange(1, 1800)][int]$TimeoutSeconds,
        [Parameter(Mandatory)][string]$DiagnosticsDirectory,
        [Parameter(Mandatory)][string]$Phase)
  Write-QualificationPhase $DiagnosticsDirectory $Phase 'starting'
  $info = [Diagnostics.ProcessStartInfo]::new()
  $info.FileName = $FilePath
  $info.UseShellExecute = $false
  foreach ($argument in $Arguments) { $info.ArgumentList.Add($argument) }
  $process = [Diagnostics.Process]::new()
  $process.StartInfo = $info
  try {
    if (-not $process.Start()) { throw 'Qualification process did not start.' }
    Write-QualificationPhase $DiagnosticsDirectory $Phase 'running' $process.Id
    if (-not $process.WaitForExit($TimeoutSeconds * 1000)) {
      Write-QualificationPhase $DiagnosticsDirectory $Phase 'timeout' $process.Id
      # Kill only this invocation and its descendants, never all msiexec/node
      # instances. Windows Installer's service is outside this owned tree.
      try {
        $process.Kill($true)
        if (-not $process.WaitForExit(5000)) { throw 'Timed-out process did not exit.' }
        Write-QualificationPhase $DiagnosticsDirectory $Phase 'terminated' $process.Id $process.ExitCode
      } catch {
        Write-QualificationPhase $DiagnosticsDirectory $Phase 'termination-failed' $process.Id
      }
      throw [TimeoutException]::new("Qualification phase '$Phase' exceeded ${TimeoutSeconds}s. Diagnostics: $DiagnosticsDirectory")
    }
    Write-QualificationPhase $DiagnosticsDirectory $Phase 'exited' $process.Id $process.ExitCode
    if ($process.ExitCode -ne 0) { throw "Qualification phase '$Phase' exited $($process.ExitCode). Diagnostics: $DiagnosticsDirectory" }
  } finally { $process.Dispose() }
}
