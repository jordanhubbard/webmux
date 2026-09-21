# Run only on an isolated Windows runner: exercise the actual WinSW lifecycle.
[CmdletBinding()]
param([Parameter(Mandatory = $true)][string]$BundleDirectory, [switch]$UserAccount)
$ErrorActionPreference = 'Stop'
$serviceDirectory = Join-Path $env:ProgramData 'WebMux'
if ((Get-Service -Name WebMux -ErrorAction SilentlyContinue) -or (Test-Path -LiteralPath $serviceDirectory)) {
  throw 'Service smoke requires an isolated machine without an existing WebMux service or wrapper directory.'
}
$root = (Resolve-Path -LiteralPath $BundleDirectory).Path
$installer = Join-Path $root 'service/windows-service.ps1'
# A different service account cannot inspect the runner administrator's private
# AppData ancestors when Go resolves configuration paths. Keep fixture data out
# of that profile; grant access only to this disposable directory below.
$homeDirectory = Join-Path $env:ProgramData "webmux-service-smoke-$([Guid]::NewGuid())"
$listener = [Net.Sockets.TcpListener]::new([Net.IPAddress]::Loopback, 0)
$listener.Start()
$port = $listener.LocalEndpoint.Port
$listener.Stop()
[IO.Directory]::CreateDirectory((Join-Path $homeDirectory 'config')) | Out-Null
$appFile = Join-Path $homeDirectory 'config/app.yaml'
$app = [IO.File]::ReadAllText((Join-Path $root 'config.defaults/app.yaml'))
$app = $app.Replace('name: webmux', 'name: native-service-smoke').Replace('listen_host: 0.0.0.0', 'listen_host: 127.0.0.1')
$app = $app -replace '(?m)^(\s*http_port:)\s*\d+', "`$1 $port"
$app = $app -replace '(?m)^(\s*https_port:)\s*\d+', '${1} 0'
$app = $app -replace '(?m)^(\s*session_logging:\r?\n\s*enabled:)\s*false', '${1} true'
[IO.File]::WriteAllText($appFile, $app)
[IO.File]::WriteAllText((Join-Path $homeDirectory 'config/auth.yaml'), "auth:`n  mode: none`n  users: []`n")
$url = "http://127.0.0.1:$port"
$fixtureAccount = $null
$fixtureSid = $null
$fixtureCredential = $null
function Grant-FixtureAccess([string]$Directory, [string]$Rights) {
  & icacls.exe $Directory /grant:r "*${fixtureSid}:(OI)(CI)$Rights" /T /Q | Out-Null
  if ($LASTEXITCODE -ne 0) { throw "Could not grant fixture account access to $Directory" }
}
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
function Test-ReconfigureRollback {
  $rollbackPath = Join-Path $serviceDirectory 'WebMux.xml'
  $before = [Convert]::ToBase64String([IO.File]::ReadAllBytes($rollbackPath))
  $attempt = [PSCustomObject]@{ Count = 0 }
  # Scope the injected failure to this invocation. Stop, XML replacement and
  # rollback remain real; the second start delegates to the Windows cmdlet.
  function Start-Service {
    param([string]$Name)
    if ($Name -ne 'WebMux') { throw 'Rollback fixture targeted an unexpected service.' }
    $attempt.Count++
    $current = [Convert]::ToBase64String([IO.File]::ReadAllBytes($rollbackPath))
    if ($attempt.Count -eq 1) {
      if ((Get-Service -Name $Name).Status -ne 'Stopped') { throw 'Reconfigure did not stop before replacing its definition.' }
      if ($current -ceq $before) { throw 'Failure injection did not reach the replaced definition.' }
      throw 'Injected fixture startup failure after XML replacement'
    }
    if ($attempt.Count -ne 2 -or $current -cne $before) { throw 'Rollback did not restore the original definition before restart.' }
    Microsoft.PowerShell.Management\Start-Service -Name $Name
  }
  $rejected = $false
  try { & $installer reconfigure } catch {
    if ($_.Exception.Message -ne 'Injected fixture startup failure after XML replacement') { throw }
    $rejected = $true
  }
  if (-not $rejected -or $attempt.Count -ne 2) { throw 'Reconfigure did not report the failure and attempt recovery.' }
  if ([Convert]::ToBase64String([IO.File]::ReadAllBytes($rollbackPath)) -cne $before) { throw 'Rollback changed original XML bytes.' }
  if ((Get-Service -Name WebMux).Status -ne 'Running') { throw 'Rollback did not restore the running service.' }
  if ((Get-CimInstance Win32_Service -Filter "Name='WebMux'").StartName -ne $account) { throw 'Rollback changed the service account.' }
  Wait-Healthy
  if (@(Get-ChildItem -LiteralPath $serviceDirectory -Filter 'WebMux-*.xml.tmp').Count -ne 0) { throw 'Rollback retained temporary definitions.' }
  Write-Host 'Reconfigure restored original XML bytes, account and running service after injected startup failure.'
}
try {
  if ($UserAccount) {
    $name = 'wmx' + [Guid]::NewGuid().ToString('N').Substring(0, 12)
    $password = ConvertTo-SecureString ('aA1!' + [Guid]::NewGuid().ToString('N') + [Guid]::NewGuid().ToString('N')) -AsPlainText -Force
    $createdAccount = New-LocalUser -Name $name -Password $password -Description 'Temporary WebMux CI service fixture'
    $fixtureAccount = $name
    $fixtureSid = $createdAccount.SID.Value
    $users = Get-LocalGroup -SID 'S-1-5-32-545'
    if (-not (Get-LocalGroupMember -Group $users | Where-Object { $_.SID.Value -eq $fixtureSid })) {
      Add-LocalGroupMember -Group $users -Member $createdAccount
    }
    $fixtureCredential = [PSCredential]::new("$env:COMPUTERNAME\$name", $password)
    $password = $null
    [IO.Directory]::CreateDirectory($serviceDirectory) | Out-Null
    Grant-FixtureAccess $root 'RX'
    Grant-FixtureAccess $serviceDirectory 'RX'
    Grant-FixtureAccess $homeDirectory 'M'
    # Shadow only the prompt in this fixture's scope. The installed script still
    # exercises its real credential registration and password-scrubbing path.
    function Get-Credential {
      param([string]$UserName, [string]$Message)
      return $fixtureCredential
    }
    & $installer install -WebMuxHome $homeDirectory
  } else {
    & $installer install -LocalSystem -WebMuxHome $homeDirectory
  }
  Wait-Healthy
  $definition = [xml][IO.File]::ReadAllText((Join-Path $serviceDirectory 'WebMux.xml'))
  if ($definition.service.executable -ne (Join-Path $root 'bin/webmux.exe')) { throw 'Service is not using the native executable.' }
  if ($definition.service.arguments) { throw 'Native service has unexpected launch arguments.' }
  $page = Invoke-WebRequest -UseBasicParsing -Uri $url -TimeoutSec 5
  if ($page.Content -notmatch '<div id="root">') { throw 'Service did not serve the frontend.' }
  $account = (Get-CimInstance Win32_Service -Filter "Name='WebMux'").StartName
  if ($UserAccount) {
    if ($account -notin @(".\$fixtureAccount", "$env:COMPUTERNAME\$fixtureAccount")) { throw 'Service is not running under the fixture account.' }
    if ($definition.SelectSingleNode('/service/serviceaccount')) { throw 'Service account credentials remained in the XML after registration.' }
  }
  $environment = @($definition.service.env | ForEach-Object { $_.OuterXml }) -join "`n"
  $originalConfig = [IO.File]::ReadAllText((Join-Path $serviceDirectory 'WebMux.xml'))
  $rejected = $false
  # Copy only service scripts so preflight sees a missing Go binary without
  # touching the executable used by the running service.
  $missingRoot = Join-Path $homeDirectory 'missing-runtime'
  Copy-Item -LiteralPath (Join-Path $root 'service') -Destination $missingRoot -Recurse -Force
  try { & (Join-Path $missingRoot 'windows-service.ps1') reconfigure } catch {
    if ($_.Exception.Message -notmatch 'native server is missing') { throw }
    $rejected = $true
  } finally { Remove-Item -LiteralPath $missingRoot -Recurse -Force }
  if (-not $rejected) { throw 'Reconfigure accepted a missing backend.' }
  if ([IO.File]::ReadAllText((Join-Path $serviceDirectory 'WebMux.xml')) -cne $originalConfig) { throw 'Failed preflight changed the definition.' }
  if ((Get-Service -Name WebMux).Status -ne 'Running') { throw 'Failed preflight stopped the service.' }
  Test-ReconfigureRollback
  & $installer reconfigure
  Wait-Healthy
  $reconfigured = [xml][IO.File]::ReadAllText((Join-Path $serviceDirectory 'WebMux.xml'))
  if ((@($reconfigured.service.env | ForEach-Object { $_.OuterXml }) -join "`n") -cne $environment) {
    throw 'Reconfigure changed the installed environment.'
  }
  if ((Get-CimInstance Win32_Service -Filter "Name='WebMux'").StartName -ne $account) { throw 'Reconfigure changed the service account.' }
  & node (Join-Path $PSScriptRoot 'smoke-windows-service-shutdown.mts') $homeDirectory "$port" $installer
  if ($LASTEXITCODE -ne 0) { throw 'Native service terminal/transcript shutdown checks failed.' }
  $reachable = $false
  try { $null = Invoke-WebRequest -UseBasicParsing -Uri "$url/api/health" -TimeoutSec 2; $reachable = $true } catch { }
  if ($reachable) { throw 'Service listener survived stop.' }
  # Model a stopped service whose XML still references the removed Node backend.
  $configPath = Join-Path $serviceDirectory 'WebMux.xml'
  $stale = [xml][IO.File]::ReadAllText($configPath)
  $stale.SelectSingleNode('/service/executable').InnerText = 'node.exe'
  $stale.SelectSingleNode('/service/arguments').InnerText = '"' + (Join-Path $root 'backend/dist/index.js') + '"'
  $stale.Save($configPath)
  & $installer reconfigure
  if ((Get-Service -Name WebMux).Status -ne 'Stopped') { throw 'Reconfigure started a previously stopped service.' }
  $native = [xml][IO.File]::ReadAllText($configPath)
  if ($native.service.executable -ne (Join-Path $root 'bin/webmux.exe') -or $native.service.arguments) { throw 'Reconfigure did not replace the legacy launch command.' }
  if ((Get-CimInstance Win32_Service -Filter "Name='WebMux'").StartName -ne $account) { throw 'Migration changed the service account.' }
  & $installer start
  Wait-Healthy
  if ([IO.File]::ReadAllText($appFile) -cne $app) { throw 'Service changed operator configuration.' }
  if (-not (Test-Path -LiteralPath (Join-Path $homeDirectory 'config/auth.yaml'))) { throw 'Service did not initialize authentication.' }
} catch {
  # Capture startup evidence before cleanup removes the private fixture. Never
  # print the service XML: registration temporarily writes credentials there.
  $failure = $_
  try {
    Get-CimInstance Win32_Service -Filter "Name='WebMux'" |
      Select-Object Name, State, StartName, ExitCode, ServiceSpecificExitCode |
      Format-List | Out-Host
    foreach ($directory in @($serviceDirectory, (Join-Path $homeDirectory 'logs'))) {
      if (Test-Path -LiteralPath $directory) {
        Get-ChildItem -LiteralPath $directory -Filter '*.log' -File | ForEach-Object {
          Write-Host "Service fixture log: $($_.Name)"
          Get-Content -LiteralPath $_.FullName -Tail 60 | Out-Host
        }
      }
    }
    Get-WinEvent -FilterHashtable @{
      LogName = 'System'; ProviderName = 'Service Control Manager'
      StartTime = [DateTime]::Now.AddMinutes(-5)
    } -ErrorAction SilentlyContinue |
      Where-Object { $_.Message -match 'WebMux' } |
      Select-Object -First 10 TimeCreated, Id, Message | Format-List | Out-Host
  } catch {
    Write-Warning "Could not collect service fixture diagnostics: $($_.Exception.Message)"
  }
  throw $failure
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
      if ($fixtureAccount) {
        & icacls.exe $root /remove:g "*$fixtureSid" /T /Q | Out-Null
        if ($LASTEXITCODE -ne 0) { throw 'Could not remove fixture account permissions from the installed payload.' }
        Remove-LocalUser -Name $fixtureAccount
        if (Get-LocalUser -Name $fixtureAccount -ErrorAction SilentlyContinue) { throw 'Fixture account survived cleanup.' }
      }
    }
    $fixtureCredential = $null
  }
}
if (Get-Service -Name WebMux -ErrorAction SilentlyContinue) { throw 'Service registration survived uninstall.' }
Write-Host "Native Windows service passed: install, HTTP/UI, stop, start, preserved config and uninstall (custom account: $UserAccount)."
