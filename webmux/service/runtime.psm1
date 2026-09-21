# Resolve the installed backend without requiring Node for native bundles.
function Get-WebMuxServiceRuntime {
  [CmdletBinding()]
  param(
    [Parameter(Mandatory = $true)][string]$ApplicationDirectory,
    [ValidateSet('auto', 'go', 'node')][string]$Backend = 'auto'
  )
  $native = Join-Path $ApplicationDirectory 'bin\webmux.exe'
  $legacy = Join-Path $ApplicationDirectory 'backend\dist\index.js'
  if ($Backend -eq 'go' -or ($Backend -eq 'auto' -and (Test-Path -LiteralPath $native -PathType Leaf))) {
    if (-not (Test-Path -LiteralPath $native -PathType Leaf)) {
      throw "The native server is missing at $native. Build or extract the native Windows bundle first."
    }
    return [PSCustomObject]@{ Backend = 'go'; Executable = $native; Arguments = '' }
  }
  if (-not (Test-Path -LiteralPath $legacy -PathType Leaf)) {
    throw "The Node production build is missing at $legacy. Run npm run build or select a native bundle."
  }
  $node = (Get-Command node.exe -CommandType Application -ErrorAction Stop | Select-Object -First 1).Source
  return [PSCustomObject]@{ Backend = 'node'; Executable = $node; Arguments = '"' + $legacy + '"' }
}
Export-ModuleMember -Function Get-WebMuxServiceRuntime
