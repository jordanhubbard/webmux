# Resolve the Go service executable without a Node runtime dependency.
function Get-WebMuxServiceRuntime {
  [CmdletBinding()]
  param([Parameter(Mandatory = $true)][string]$ApplicationDirectory)
  $native = Join-Path $ApplicationDirectory 'bin\webmux.exe'
  if (-not (Test-Path -LiteralPath $native -PathType Leaf)) {
    throw "The native server is missing at $native. Build or extract the native Windows bundle first."
  }
  return [PSCustomObject]@{ Backend = 'go'; Executable = $native; Arguments = '' }
}
Export-ModuleMember -Function Get-WebMuxServiceRuntime
