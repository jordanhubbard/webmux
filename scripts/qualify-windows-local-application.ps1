param([Parameter(Mandatory = $true)][string]$OutputDirectory)

# Like collect-native-screen.ps1, this is a synthetic fixture on a disposable
# Actions VM. It must never install/run a task on the user's qualification host.
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
if ($env:GITHUB_ACTIONS -ne 'true' -or $env:RUNNER_ENVIRONMENT -ne 'github-hosted' -or -not $env:RUNNER_TEMP -or -not $env:GITHUB_WORKSPACE) {
    throw 'This synthetic task requires a disposable GitHub Actions runner'
}
$repo = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
if (-not $repo.Equals([IO.Path]::GetFullPath($env:GITHUB_WORKSPACE), [StringComparison]::OrdinalIgnoreCase)) {
    throw 'Qualification source must be the Actions checkout'
}
$OutputDirectory = [IO.Path]::GetFullPath($OutputDirectory)
$tempPrefix = [IO.Path]::GetFullPath($env:RUNNER_TEMP).TrimEnd('\') + '\'
if (-not $OutputDirectory.StartsWith($tempPrefix, [StringComparison]::OrdinalIgnoreCase)) { throw 'Fixture output must stay below RUNNER_TEMP' }
if (Test-Path -LiteralPath $OutputDirectory) { throw 'Application qualification directory must be new' }
New-Item -ItemType Directory -Path $OutputDirectory | Out-Null
$public = Join-Path $OutputDirectory 'public'
New-Item -ItemType Directory -Path $public | Out-Null
$private = Join-Path $OutputDirectory 'private'
New-Item -ItemType Directory -Path $private | Out-Null
$node = (Get-Command node -ErrorAction Stop).Source
$go = (Get-Command go -ErrorAction Stop).Source
$sourceHead = (& git -C $repo rev-parse HEAD).Trim()
if ($LASTEXITCODE -ne 0) { throw 'Cannot bind qualification source' }
$dirty = & git -C $repo status --porcelain --untracked-files=normal
if ($LASTEXITCODE -ne 0 -or $dirty) { throw 'Qualification requires a clean source checkout' }
$image = Join-Path $public 'webmux-app.exe'
Push-Location (Join-Path $repo 'webmux/server')
try {
    & $go run ./cmd/build -o $image
    if ($LASTEXITCODE -ne 0) { throw 'Native application fixture build failed' }
} finally { Pop-Location }
$imageSHA = (Get-FileHash -Algorithm SHA256 -LiteralPath $image).Hash.ToLowerInvariant()
$test = Join-Path $repo 'tests/e2e/windows-local-provider.native.spec.ts'
$testSHA = (Get-FileHash -Algorithm SHA256 -LiteralPath $test).Hash.ToLowerInvariant()
$childTest = Join-Path $repo 'tests/e2e/windows-local-child.mts'
$childTestSHA = (Get-FileHash -Algorithm SHA256 -LiteralPath $childTest).Hash.ToLowerInvariant()
$browser = (& $node -e "console.log(require('./webmux/node_modules/playwright-core').chromium.executablePath())").Trim()
if ($LASTEXITCODE -ne 0 -or -not (Test-Path -LiteralPath $browser)) { throw 'Installed fixture browser is required' }
$listener = [Net.Sockets.TcpListener]::new([Net.IPAddress]::Loopback, 0)
$listener.Start()
$port = $listener.LocalEndpoint.Port
$listener.Stop()
$result = Join-Path $public 'exit-code.txt'
$report = Join-Path $public 'tests.json'
$workerError = Join-Path $public 'worker-stderr.log'
$worker = Join-Path $private 'worker.ps1'
function Quote-Literal([string]$Value) { return "'" + $Value.Replace("'", "''") + "'" }
$source = @'
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
function Finish([string]$Code) {
    [IO.File]::WriteAllText(__RESULT__ + '.tmp', $Code)
    [IO.File]::Move(__RESULT__ + '.tmp', __RESULT__)
}
try {
    # No user profile, authentication state or runner secrets are copied.
    if (-not ([Security.Principal.WindowsIdentity]::GetCurrent()).IsSystem) { throw 'Expected the disposable SYSTEM fixture account' }
    [IO.File]::WriteAllText(__ACCOUNT__, '{"isSystem":true}')
    $env:GITHUB_ACTIONS = 'true'
    $env:RUNNER_ENVIRONMENT = 'github-hosted'
    $env:PATH = __PATH__
    $env:LOCALAPPDATA = __CACHE__
    $env:APPDATA = __APPDATA__
    $env:WEBMUX_RECOVERY_CACHE_HOME = __CACHE__
    $env:WEBMUX_E2E_PREBUILT_BINARY = __IMAGE__
    $env:WEBMUX_E2E_PREBUILT_SHA256 = __SHA__
    $env:WEBMUX_E2E_PORT = __PORT__
    $env:PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH = __BROWSER__
    foreach ($p in @($env:LOCALAPPDATA, $env:APPDATA)) { New-Item -ItemType Directory -Path $p -Force | Out-Null }
    Set-Location -LiteralPath __CWD__
    # Preserve native stderr as evidence; warnings must not become PS exceptions.
    $ErrorActionPreference = 'Continue'
    & __NODE__ 'scripts/run-playwright.mts' '--config=playwright.native-windows.config.ts' 'windows-local-provider.native.spec.ts' '--trace=retain-on-failure' '--reporter=json' __OUTPUT__ 2> __ERROR__ | Set-Content -Encoding UTF8 -LiteralPath __REPORT__
    $code = $LASTEXITCODE
    $ErrorActionPreference = 'Stop'
    Finish ([string]$code)
} catch {
    $_ | Out-String | Add-Content -Encoding UTF8 -LiteralPath __ERROR__
    Finish '1'
}
'@
$workerPath = (Split-Path $node) + ';' + (Split-Path $go) + ';' + $env:SystemRoot + '\System32;' + $env:SystemRoot
foreach ($entry in @{
    '__RESULT__' = $result; '__PATH__' = $workerPath;
    '__CACHE__' = (Join-Path $private 'cache'); '__APPDATA__' = (Join-Path $private 'appdata');
    '__IMAGE__' = $image; '__SHA__' = $imageSHA; '__PORT__' = ([string]$port);
    '__BROWSER__' = $browser; '__CWD__' = (Join-Path $repo 'webmux'); '__NODE__' = $node;
    '__OUTPUT__' = ('--output=' + (Join-Path $public 'test-results')); '__REPORT__' = $report; '__ERROR__' = $workerError
    '__ACCOUNT__' = (Join-Path $public 'account-scope.json')
}.GetEnumerator()) { $source = $source.Replace($entry.Key, (Quote-Literal $entry.Value)) }
Set-Content -LiteralPath $worker -Value $source -Encoding UTF8
$taskName = 'WebMux-Local-App-' + [guid]::NewGuid().ToString('N')
$action = New-ScheduledTaskAction -Execute "$env:SystemRoot\System32\WindowsPowerShell\v1.0\powershell.exe" -Argument "-NoProfile -NonInteractive -ExecutionPolicy Bypass -File `"$worker`""
$principal = New-ScheduledTaskPrincipal -UserId 'SYSTEM' -LogonType ServiceAccount -RunLevel Highest
$settings = New-ScheduledTaskSettingsSet -ExecutionTimeLimit (New-TimeSpan -Minutes 5) -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries
$receipt = @{ source = $sourceHead; imageSHA256 = $imageSHA; testSHA256 = $testSHA; childTestSHA256 = $childTestSHA;
    scope = 'Synthetic hosted SYSTEM application fixture; no authenticated coding/session recovery';
    executionAccount = 'SYSTEM'; osArchitecture = [Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString();
    outcome = 'unconfirmed'; taskRemoved = $false }
Register-ScheduledTask -TaskName $taskName -Action $action -Principal $principal -Settings $settings | Out-Null
try {
    Start-ScheduledTask -TaskName $taskName
    $deadline = [Diagnostics.Stopwatch]::StartNew()
    while (-not (Test-Path -LiteralPath $result)) {
        if ($deadline.Elapsed.TotalSeconds -gt 330) { throw 'Native application fixture task exceeded its deadline' }
        Start-Sleep -Milliseconds 250
    }
    if ((Get-Content -LiteralPath $result -Raw).Trim() -ne '0') {
        Get-Content -LiteralPath $workerError -ErrorAction SilentlyContinue | Write-Output
        throw 'Native application fixture gate failed; retain original fixture state'
    }
    $tests = Get-Content -LiteralPath $report -Raw | ConvertFrom-Json
    $account = Get-Content -LiteralPath (Join-Path $public 'account-scope.json') -Raw | ConvertFrom-Json
    if ($account.isSystem -ne $true) { throw 'Actual fixture execution account was not verified' }
    if ($tests.stats.expected -ne 2 -or $tests.stats.unexpected -ne 0 -or $tests.stats.skipped -ne 0 -or $tests.stats.flaky -ne 0) { throw 'Expected both native application gates with no skips or retries' }
    if ((Get-FileHash -Algorithm SHA256 -LiteralPath $image).Hash.ToLowerInvariant() -ne $imageSHA -or
        (Get-FileHash -Algorithm SHA256 -LiteralPath $test).Hash.ToLowerInvariant() -ne $testSHA -or
        (Get-FileHash -Algorithm SHA256 -LiteralPath $childTest).Hash.ToLowerInvariant() -ne $childTestSHA) { throw 'Qualification input changed during execution' }
    $receipt.outcome = 'passed'
} finally {
    Stop-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue
    Unregister-ScheduledTask -TaskName $taskName -Confirm:$false
    $receipt.taskRemoved = -not [bool](Get-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue)
    $receipt | ConvertTo-Json -Depth 5 | Set-Content -Encoding utf8 (Join-Path $public 'application-receipt.json')
}
if (-not $receipt.taskRemoved) { throw 'Fixture task removal unconfirmed' }
$receipt | ConvertTo-Json -Depth 5 | Write-Output
