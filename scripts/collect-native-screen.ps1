param([Parameter(Mandatory = $true)][string]$OutputDirectory)

# Hosted Actions jobs prohibit CREATE_BREAKAWAY_FROM_JOB. Collect only the
# synthetic ConPTY fixture in a short-lived SYSTEM task outside that job; the
# browser still runs as the ordinary runner account. No sign-in state is used.
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$OutputDirectory = [IO.Path]::GetFullPath($OutputDirectory)
if (Test-Path -LiteralPath $OutputDirectory) { throw 'Capture directory must be new' }
New-Item -ItemType Directory -Path $OutputDirectory | Out-Null
$go = (Get-Command go -ErrorAction Stop).Source
$node = (Get-Command node -ErrorAction Stop).Source
$image = Join-Path $OutputDirectory 'native-screen.test.exe'
$captures = Join-Path $OutputDirectory 'captures.json'
$log = Join-Path $OutputDirectory 'tests.jsonl'
$result = Join-Path $OutputDirectory 'exit-code.txt'
$workerError = Join-Path $OutputDirectory 'worker-error.txt'
$worker = Join-Path $OutputDirectory 'worker.ps1'
Push-Location (Join-Path $PSScriptRoot '../webmux/server')
try {
    & $go test -c -o $image ./internal/nativeprovider
    if ($LASTEXITCODE -ne 0) { throw 'Native fixture compilation failed' }
} finally { Pop-Location }

function Quote-Literal([string]$Value) { return "'" + $Value.Replace("'", "''") + "'" }
$workerPath = (Split-Path $node) + ';' + (Split-Path $go) + ';' + $env:SystemRoot + '\System32;' + $env:SystemRoot
$source = @'
$ErrorActionPreference = 'Stop'
try {
    $env:PATH = __PATH__
    $env:GOTOOLCHAIN = 'local'
    $env:WEBMUX_NATIVE_CURSOR_CAPTURES = __CAPTURES__
    & __GO__ tool test2json -t -p github.com/jordanhubbard/webmux/server/internal/nativeprovider __IMAGE__ -test.v -test.run '^TestNativeProviderSavedCursorCaptures$' -test.timeout 120s 2>&1 | Set-Content -Encoding UTF8 -LiteralPath __LOG__
    [IO.File]::WriteAllText(__RESULT__, [string]$LASTEXITCODE)
} catch {
    $_ | Out-String | Set-Content -Encoding UTF8 -LiteralPath __ERROR__
    [IO.File]::WriteAllText(__RESULT__, '1')
}
'@
foreach ($entry in @{
    '__PATH__' = $workerPath; '__GO__' = $go; '__IMAGE__' = $image;
    '__CAPTURES__' = $captures; '__LOG__' = $log; '__RESULT__' = $result;
    '__ERROR__' = $workerError
}.GetEnumerator()) { $source = $source.Replace($entry.Key, (Quote-Literal $entry.Value)) }
Set-Content -LiteralPath $worker -Value $source -Encoding UTF8
$taskName = 'WebMux-Screen-' + [guid]::NewGuid().ToString('N')
$action = New-ScheduledTaskAction -Execute "$env:SystemRoot\System32\WindowsPowerShell\v1.0\powershell.exe" -Argument "-NoProfile -NonInteractive -ExecutionPolicy Bypass -File `"$worker`""
$principal = New-ScheduledTaskPrincipal -UserId 'SYSTEM' -LogonType ServiceAccount -RunLevel Highest
$settings = New-ScheduledTaskSettingsSet -ExecutionTimeLimit (New-TimeSpan -Minutes 3) -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries
Register-ScheduledTask -TaskName $taskName -Action $action -Principal $principal -Settings $settings | Out-Null
try {
    Start-ScheduledTask -TaskName $taskName
    $deadline = [Diagnostics.Stopwatch]::StartNew()
    while (-not (Test-Path -LiteralPath $result)) {
        if ($deadline.Elapsed.TotalSeconds -gt 180) { throw 'Native screen task exceeded its deadline' }
        Start-Sleep -Milliseconds 250
    }
    Get-Content -LiteralPath $log -ErrorAction SilentlyContinue | Write-Output
    if ((Get-Content -LiteralPath $result -Raw).Trim() -ne '0') {
        Get-Content -LiteralPath $workerError -ErrorAction SilentlyContinue | Write-Output
        throw 'Native screen task failed'
    }
    if (-not (Test-Path -LiteralPath $captures)) { throw 'Native screen task produced no captures' }
    Get-FileHash -Algorithm SHA256 -LiteralPath $image | Format-List | Out-String | Write-Output
} finally {
    Stop-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue
    Unregister-ScheduledTask -TaskName $taskName -Confirm:$false
}
