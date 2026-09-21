[CmdletBinding()]
param(
  [Parameter(Position = 0)]
  [ValidateSet('install', 'uninstall', 'reconfigure', 'start', 'stop', 'restart', 'status')]
  [string]$Action = 'status',

  [string]$WebMuxHome,

  [switch]$LocalSystem
)

$ErrorActionPreference = 'Stop'
if (-not $WebMuxHome) {
  if ($env:WEBMUX_HOME) { $WebMuxHome = $env:WEBMUX_HOME }
  else { $WebMuxHome = Join-Path $env:USERPROFILE '.config\webmux' }
}
$ServiceName = 'WebMux'
$WinSWVersion = '2.12.0'
$WinSWHash = 'B5066B7BBDFBA1293E5D15CDA3CAAEA88FBEAB35BD5B38C41C913D492AADFC4F'
$WinSWUrl = "https://github.com/winsw/winsw/releases/download/v$WinSWVersion/WinSW.NET461.exe"
$ServiceDirectory = Join-Path $env:ProgramData 'WebMux'
$WrapperPath = Join-Path $ServiceDirectory 'WebMux.exe'
$ConfigPath = Join-Path $ServiceDirectory 'WebMux.xml'
$ApplicationDirectory = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).Path
Import-Module (Join-Path $PSScriptRoot 'runtime.psm1') -Force

function Assert-WindowsAdministrator {
  if ($env:OS -ne 'Windows_NT') {
    throw 'Windows service management must be run on Windows.'
  }
  $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
  $principal = New-Object Security.Principal.WindowsPrincipal($identity)
  if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw 'Run this command from an elevated PowerShell window (Run as administrator).'
  }
}

function Escape-Xml([string]$Value) {
  return [Security.SecurityElement]::Escape($Value)
}

function Get-WebMuxService {
  return Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
}

function Write-ServiceConfig([string]$ServiceAccountXml = '') {
  $runtime = Get-WebMuxServiceRuntime -ApplicationDirectory $ApplicationDirectory
  $pathValue = [Environment]::GetEnvironmentVariable('PATH', 'Machine')
  $userPath = [Environment]::GetEnvironmentVariable('PATH', 'User')
  if ($userPath) { $pathValue = "$pathValue;$userPath" }
  $logDirectory = Join-Path $WebMuxHome 'logs'

  $xml = @"
<service>
  <id>$ServiceName</id>
  <name>WebMux</name>
  <description>Web-native persistent SSH terminal multiplexer</description>
  <executable>$(Escape-Xml $runtime.Executable)</executable>
  <arguments>$(Escape-Xml $runtime.Arguments)</arguments>
  <workingdirectory>$(Escape-Xml $ApplicationDirectory)</workingdirectory>
  <env name="WEBMUX_ROOT" value="$(Escape-Xml $ApplicationDirectory)" />
  <env name="WEBMUX_HOME" value="$(Escape-Xml $WebMuxHome)" />
  <env name="HOME" value="$(Escape-Xml $env:USERPROFILE)" />
  <env name="USERPROFILE" value="$(Escape-Xml $env:USERPROFILE)" />
  <env name="PATH" value="$(Escape-Xml $pathValue)" />
  <startmode>Automatic</startmode>
  <delayedAutoStart />
  <stoptimeout>30 sec</stoptimeout>
  <stopparentprocessfirst>true</stopparentprocessfirst>
  <onfailure action="restart" delay="10 sec" />
  <resetfailure>1 hour</resetfailure>
  <logpath>$(Escape-Xml $logDirectory)</logpath>
  <log mode="roll" />
$ServiceAccountXml</service>
"@
  Set-Content -LiteralPath $ConfigPath -Value $xml -Encoding UTF8
}

function Install-WinSW {
  if (Test-Path -LiteralPath $WrapperPath) {
    $actualHash = (Get-FileHash -LiteralPath $WrapperPath -Algorithm SHA256).Hash
    if ($actualHash -eq $WinSWHash) { return }
    throw "Unexpected checksum for $WrapperPath. Remove it before retrying the install."
  }

  $downloadPath = "$WrapperPath.download"
  try {
    Write-Host "Downloading WinSW $WinSWVersion..."
    Invoke-WebRequest -UseBasicParsing -Uri $WinSWUrl -OutFile $downloadPath
    $actualHash = (Get-FileHash -LiteralPath $downloadPath -Algorithm SHA256).Hash
    if ($actualHash -ne $WinSWHash) {
      throw "WinSW checksum verification failed: expected $WinSWHash, received $actualHash"
    }
    Move-Item -LiteralPath $downloadPath -Destination $WrapperPath
  } finally {
    if (Test-Path -LiteralPath $downloadPath) {
      Remove-Item -LiteralPath $downloadPath -Force
    }
  }
}

function Install-WebMuxService {
  if (Get-WebMuxService) {
    throw "The $ServiceName service is already installed. Run the uninstall command first."
  }
  $null = Get-WebMuxServiceRuntime -ApplicationDirectory $ApplicationDirectory
  if (-not (Get-Command ssh.exe -CommandType Application -ErrorAction SilentlyContinue)) {
    throw 'OpenSSH Client is required: ssh.exe was not found on PATH.'
  }

  New-Item -ItemType Directory -Path $ServiceDirectory -Force | Out-Null
  New-Item -ItemType Directory -Path (Join-Path $WebMuxHome 'logs') -Force | Out-Null
  Install-WinSW

  $accountXml = ''
  $plainPassword = $null
  if ($LocalSystem) {
    $accountXml = "  <serviceaccount>`r`n    <user>LocalSystem</user>`r`n  </serviceaccount>`r`n"
    Write-Warning 'Installing as LocalSystem. User SSH keys and known-hosts files will not be available.'
  } else {
    $currentAccount = [Security.Principal.WindowsIdentity]::GetCurrent().Name
    $credential = Get-Credential -UserName $currentAccount -Message 'Enter the password for the Windows account that will run WebMux'
    if (-not $credential) { throw 'Service account credentials are required.' }
    $accountParts = $credential.UserName.Split('\', 2)
    if ($accountParts.Count -ne 2) {
      throw 'The service account must be in DOMAIN\user or COMPUTER\user form.'
    }
    $plainPassword = $credential.GetNetworkCredential().Password
    $accountXml = "  <serviceaccount>`r`n    <domain>$(Escape-Xml $accountParts[0])</domain>`r`n    <user>$(Escape-Xml $accountParts[1])</user>`r`n    <password>$(Escape-Xml $plainPassword)</password>`r`n    <allowservicelogon>true</allowservicelogon>`r`n  </serviceaccount>`r`n"
  }

  try {
    Write-ServiceConfig $accountXml
    & $WrapperPath install
    if ($LASTEXITCODE -ne 0) { throw "WinSW service installation failed with exit code $LASTEXITCODE." }
  } finally {
    # The password is only needed while registering the service. Never retain it
    # in the WinSW configuration that remains on disk.
    $plainPassword = $null
    Write-ServiceConfig
  }

  Start-Service -Name $ServiceName
  (Get-Service -Name $ServiceName).WaitForStatus('Running', [TimeSpan]::FromSeconds(30))
  Write-Host "WebMux service installed and running."
  Write-Host "Configuration: $WebMuxHome"
  Write-Host "Service logs: $(Join-Path $WebMuxHome 'logs')"
}

function Uninstall-WebMuxService {
  $service = Get-WebMuxService
  if (-not $service) {
    Write-Host 'WebMux service is not installed.'
    return
  }
  if ($service.Status -ne 'Stopped') {
    Stop-Service -Name $ServiceName
    $service.WaitForStatus('Stopped', [TimeSpan]::FromSeconds(30))
  }
  if (-not (Test-Path -LiteralPath $WrapperPath)) {
    throw "The service wrapper is missing at $WrapperPath; remove the service manually with sc.exe delete $ServiceName."
  }
  & $WrapperPath uninstall
  if ($LASTEXITCODE -ne 0) { throw "WinSW service removal failed with exit code $LASTEXITCODE." }
  Remove-Item -LiteralPath $ConfigPath -Force -ErrorAction SilentlyContinue
  Remove-Item -LiteralPath $WrapperPath -Force -ErrorAction SilentlyContinue
  if ((Test-Path -LiteralPath $ServiceDirectory) -and
      -not (Get-ChildItem -LiteralPath $ServiceDirectory -Force | Select-Object -First 1)) {
    Remove-Item -LiteralPath $ServiceDirectory -Force
  }
  Write-Host 'WebMux service uninstalled. Runtime data and logs were preserved.'
}

function Reconfigure-WebMuxService {
  $service = Get-WebMuxService
  if (-not $service) { throw 'Install the WebMux service before reconfiguring it.' }
  $runtime = Get-WebMuxServiceRuntime -ApplicationDirectory $ApplicationDirectory
  $original = [IO.File]::ReadAllBytes($ConfigPath)
  $definition = New-Object System.Xml.XmlDocument
  $definition.XmlResolver = $null
  $definition.Load($ConfigPath)
  if ($definition.SelectSingleNode('/service/id').InnerText -ne $ServiceName) { throw 'Unexpected WinSW service identity.' }
  foreach ($name in @('executable', 'arguments', 'workingdirectory')) {
    if ($definition.SelectNodes("/service/$name").Count -ne 1) { throw "Expected one WinSW $name element." }
  }
  $roots = $definition.SelectNodes('/service/env[@name="WEBMUX_ROOT"]')
  if ($roots.Count -ne 1) { throw 'Expected one WEBMUX_ROOT environment entry.' }
  $definition.SelectSingleNode('/service/executable').InnerText = $runtime.Executable
  $definition.SelectSingleNode('/service/arguments').InnerText = $runtime.Arguments
  $definition.SelectSingleNode('/service/workingdirectory').InnerText = $ApplicationDirectory
  $roots[0].SetAttribute('value', $ApplicationDirectory)
  $wasRunning = $service.Status -eq 'Running'
  if ($service.Status -ne 'Stopped' -and -not $wasRunning) { throw "Wait for the service's pending transition before reconfiguring: $($service.Status)" }
  $temporary = Join-Path $ServiceDirectory "WebMux-$([Guid]::NewGuid()).xml.tmp"
  $replaced = $false
  try {
    # Serialize and validate the new runtime before stopping the current service.
    $definition.Save($temporary)
    if ($wasRunning) {
      Stop-Service -Name $ServiceName
      $service.WaitForStatus('Stopped', [TimeSpan]::FromSeconds(30))
    }
    # PowerShell coerces $null to an empty string for this .NET string parameter.
    [IO.File]::Replace($temporary, $ConfigPath, [System.Management.Automation.Language.NullString]::Value)
    $replaced = $true
    if ($wasRunning) {
      Start-Service -Name $ServiceName
      (Get-Service -Name $ServiceName).WaitForStatus('Running', [TimeSpan]::FromSeconds(30))
    }
  } catch {
    $failure = $_
    if ($replaced) {
      $current = Get-WebMuxService
      if ($current.Status -ne 'Stopped') {
        Stop-Service -Name $ServiceName
        $current.WaitForStatus('Stopped', [TimeSpan]::FromSeconds(30))
      }
      [IO.File]::WriteAllBytes($temporary, $original)
      [IO.File]::Replace($temporary, $ConfigPath, [System.Management.Automation.Language.NullString]::Value)
    }
    if ($wasRunning -and (Get-WebMuxService).Status -eq 'Stopped') {
      Start-Service -Name $ServiceName
      (Get-Service -Name $ServiceName).WaitForStatus('Running', [TimeSpan]::FromSeconds(30))
    }
    throw $failure
  } finally {
    if (Test-Path -LiteralPath $temporary) { Remove-Item -LiteralPath $temporary -Force }
  }
  Write-Host "WebMux service runtime reconfigured to $($runtime.Backend). Existing account and environment were preserved."
}

Assert-WindowsAdministrator

switch ($Action) {
  'install' { Install-WebMuxService }
  'uninstall' { Uninstall-WebMuxService }
  'reconfigure' {
    if ($PSBoundParameters.ContainsKey('WebMuxHome') -or $LocalSystem) {
      throw 'Reconfigure preserves the installed home and service account; omit WebMuxHome and LocalSystem.'
    }
    Reconfigure-WebMuxService
  }
  'start' {
    Start-Service -Name $ServiceName
    (Get-Service -Name $ServiceName).WaitForStatus('Running', [TimeSpan]::FromSeconds(30))
    Write-Host 'WebMux service is running.'
  }
  'stop' {
    Stop-Service -Name $ServiceName
    (Get-Service -Name $ServiceName).WaitForStatus('Stopped', [TimeSpan]::FromSeconds(30))
    Write-Host 'WebMux service is stopped.'
  }
  'restart' {
    Restart-Service -Name $ServiceName
    (Get-Service -Name $ServiceName).WaitForStatus('Running', [TimeSpan]::FromSeconds(30))
    Write-Host 'WebMux service is running.'
  }
  'status' {
    $service = Get-WebMuxService
    if ($service) { $service | Format-Table -AutoSize Name, Status, StartType }
    else { Write-Host 'WebMux service is not installed.' }
  }
}
