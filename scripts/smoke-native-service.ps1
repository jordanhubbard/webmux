# Run only on an isolated Windows runner: exercise the actual WinSW lifecycle.
[CmdletBinding()]
param([Parameter(Mandatory = $true)][string]$BundleDirectory)
$ErrorActionPreference = 'Stop'
$serviceDirectory = Join-Path $env:ProgramData 'WebMux'
if ((Get-Service -Name WebMux -ErrorAction SilentlyContinue) -or (Test-Path -LiteralPath $serviceDirectory)) {
  throw 'Service smoke requires an isolated machine without an existing WebMux service or wrapper directory.'
}
$root = (Resolve-Path -LiteralPath $BundleDirectory).Path
$installer = Join-Path $root 'service/windows-service.ps1'
$homeDirectory = Join-Path ([IO.Path]::GetTempPath()) "webmux-service-smoke-$([Guid]::NewGuid())"
$listener = [Net.Sockets.TcpListener]::new([Net.IPAddress]::Loopback, 0)
$listener.Start()
$port = $listener.LocalEndpoint.Port
$listener.Stop()
[IO.Directory]::CreateDirectory((Join-Path $homeDirectory 'config')) | Out-Null
$appFile = Join-Path $homeDirectory 'config/app.yaml'
$app = [IO.File]::ReadAllText((Join-Path $root 'config.defaults/app.yaml'))
$app = $app.Replace('name: webmux', 'name: native-service-smoke').Replace('listen_host: 0.0.0.0', 'listen_host: 127.0.0.1')
$app = $app -replace '(?m)^(\s*http_port:)\s*\d+', "`$1 $port"
[IO.File]::WriteAllText($appFile, $app)
$url = "http://127.0.0.1:$port"
function Wait-Healthy {
  $deadline = [DateTime]::UtcNow.AddSeconds(30)
  do {
    try {
      $health = Invoke-RestMethod -Uri "$url/api/health" -TimeoutSec 2
      if ($health.status -eq 'ok' -and $health.name -eq 'native-service-smoke') { return }
    } catch { }
    Start-Sleep -Milliseconds 200
  } while ([DateTime]::UtcNow -lt $deadline)
  throw 'Native Windows service did not become healthy.'
}
try {
  & $installer install -Backend go -LocalSystem -WebMuxHome $homeDirectory
  Wait-Healthy
  $definition = [xml][IO.File]::ReadAllText((Join-Path $serviceDirectory 'WebMux.xml'))
  if ($definition.service.executable -ne (Join-Path $root 'bin/webmux.exe')) { throw 'Service is not using the native executable.' }
  if ($definition.service.arguments) { throw 'Native service has unexpected launch arguments.' }
  $page = Invoke-WebRequest -UseBasicParsing -Uri $url -TimeoutSec 5
  if ($page.Content -notmatch '<div id="root">') { throw 'Service did not serve the frontend.' }
  & $installer stop
  $reachable = $false
  try { $null = Invoke-WebRequest -UseBasicParsing -Uri "$url/api/health" -TimeoutSec 2; $reachable = $true } catch { }
  if ($reachable) { throw 'Service listener survived stop.' }
  & $installer start
  Wait-Healthy
  if ([IO.File]::ReadAllText($appFile) -cne $app) { throw 'Service changed operator configuration.' }
  if (-not (Test-Path -LiteralPath (Join-Path $homeDirectory 'config/auth.yaml'))) { throw 'Service did not initialize authentication.' }
} finally {
  try {
    if (Get-Service -Name WebMux -ErrorAction SilentlyContinue) {
      & $installer uninstall
      if (-not (Test-Path -LiteralPath $appFile) -or [IO.File]::ReadAllText($appFile) -cne $app) {
        throw 'Service uninstall removed or changed runtime configuration.'
      }
    }
    if (-not (Get-Service -Name WebMux -ErrorAction SilentlyContinue)) {
      if (Test-Path -LiteralPath $serviceDirectory) { Remove-Item -LiteralPath $serviceDirectory -Recurse -Force }
    }
  } finally {
    if (-not (Get-Service -Name WebMux -ErrorAction SilentlyContinue)) {
      if (Test-Path -LiteralPath $homeDirectory) { Remove-Item -LiteralPath $homeDirectory -Recurse -Force }
    }
  }
}
if (Get-Service -Name WebMux -ErrorAction SilentlyContinue) { throw 'Service registration survived uninstall.' }
Write-Host 'Native Windows service passed: install, HTTP/UI, stop, start, preserved config and uninstall.'
