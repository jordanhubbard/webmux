[CmdletBinding()]
param([Parameter(Mandatory = $true)][string]$Installer)
$ErrorActionPreference = 'Stop'
$installerPath = (Resolve-Path -LiteralPath $Installer).Path
$installRoot = Join-Path $env:LOCALAPPDATA 'Programs\WebMux'
if ((Test-Path -LiteralPath $installRoot) -or
    (Get-Service -Name WebMux -ErrorAction SilentlyContinue) -or
    (Test-Path -LiteralPath (Join-Path $env:ProgramData 'WebMux'))) {
  throw 'MSI smoke requires an isolated runner without an existing WebMux installation or service.'
}
$temporary = Join-Path ([IO.Path]::GetTempPath()) "webmux-native-msi-smoke-$([Guid]::NewGuid())"
[IO.Directory]::CreateDirectory($temporary) | Out-Null
function Invoke-Installer([string]$Operation, [string]$Log) {
  $arguments = @($Operation, ('"' + $installerPath + '"'), '/qn', '/norestart', '/l*v', ('"' + $Log + '"'))
  $process = Start-Process msiexec.exe -ArgumentList $arguments -Wait -PassThru
  if ($process.ExitCode -ne 0) {
    if (Test-Path -LiteralPath $Log) { Get-Content -LiteralPath $Log }
    throw "MSI $Operation failed with exit code $($process.ExitCode)."
  }
}
try {
  Invoke-Installer '/i' (Join-Path $temporary 'install.log')
  if (-not (Test-Path -LiteralPath (Join-Path $installRoot 'bin\webmux.exe'))) { throw 'MSI omitted the native executable.' }
  if (Test-Path -LiteralPath (Join-Path $installRoot 'node_modules')) { throw 'Native MSI includes Node runtime dependencies.' }
  & node (Join-Path $PSScriptRoot 'smoke-native-package.mts') $installRoot
  if ($LASTEXITCODE -ne 0) { throw 'Installed native runtime failed smoke checks.' }
  & (Join-Path $PSScriptRoot 'smoke-native-service.ps1') -BundleDirectory $installRoot
} catch {
  Write-Error $_ -ErrorAction Continue
  throw
} finally {
  # Never remove an installation still referenced by a service after a failed
  # service cleanup. Retain its files and logs for diagnosis on the runner.
  if (Get-Service -Name WebMux -ErrorAction SilentlyContinue) {
    throw 'WebMux service remains registered; retaining the installation for diagnosis.'
  }
  if (Test-Path -LiteralPath $installRoot) { Invoke-Installer '/x' (Join-Path $temporary 'uninstall.log') }
}
if (Test-Path -LiteralPath (Join-Path $installRoot 'bin\webmux.exe')) { throw 'MSI uninstall left the native executable behind.' }
Remove-Item -LiteralPath $temporary -Recurse -Force
Write-Host 'Native MSI passed: installation, native runtime, service lifecycle and uninstall.'
