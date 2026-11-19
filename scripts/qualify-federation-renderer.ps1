param([Parameter(Mandatory = $true)][string]$OutputDirectory)

# Like collect-native-screen.ps1, run only the synthetic native fixture in a
# bounded SYSTEM task outside Actions' restrictive job. No account sign-in or
# operator state is used; a skip cannot satisfy this qualification.
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$OutputDirectory = [IO.Path]::GetFullPath($OutputDirectory)
if (Test-Path -LiteralPath $OutputDirectory) { throw 'Qualification directory must be new' }
New-Item -ItemType Directory -Path $OutputDirectory | Out-Null
$go = (Get-Command go -ErrorAction Stop).Source
$image = Join-Path $OutputDirectory 'federation-renderer.test.exe'
$log = Join-Path $OutputDirectory 'tests.jsonl'
$result = Join-Path $OutputDirectory 'exit-code.txt'
$worker = Join-Path $OutputDirectory 'worker.ps1'
Push-Location (Join-Path $PSScriptRoot '../webmux/server')
try {
    & $go test -c -o $image ./internal/httpapi
    if ($LASTEXITCODE -ne 0) { throw 'Native federation fixture compilation failed' }
} finally { Pop-Location }
function Quote-Literal([string]$Value) { return "'" + $Value.Replace("'", "''") + "'" }
$workerPath = (Split-Path $go) + ';' + $env:SystemRoot + '\System32;' + $env:SystemRoot
$source = @'
$ErrorActionPreference = 'Stop'
try {
    $env:PATH = __PATH__
    $env:GOTOOLCHAIN = 'local'
    $env:WEBMUX_REQUIRE_NATIVE_FEDERATION = '1'
    & __GO__ tool test2json -t -p github.com/jordanhubbard/webmux/server/internal/httpapi __IMAGE__ '-test.v' '-test.run=^TestFederationNativeRendererProfilePreservesOwner$' '-test.timeout=120s' 2>&1 | Set-Content -Encoding UTF8 -LiteralPath __LOG__
    $code = [string]$LASTEXITCODE
} catch {
    $_ | Out-String | Add-Content -Encoding UTF8 -LiteralPath __LOG__
    $code = '1'
}
[IO.File]::WriteAllText(__RESULT__ + '.tmp', $code)
[IO.File]::Move(__RESULT__ + '.tmp', __RESULT__)
'@
foreach ($entry in @{'__PATH__'=$workerPath; '__GO__'=$go; '__IMAGE__'=$image; '__LOG__'=$log; '__RESULT__'=$result}.GetEnumerator()) {
    $source = $source.Replace($entry.Key, (Quote-Literal $entry.Value))
}
Set-Content -LiteralPath $worker -Value $source -Encoding UTF8
$taskName = 'WebMux-Federation-' + [guid]::NewGuid().ToString('N')
$action = New-ScheduledTaskAction -Execute "$env:SystemRoot\System32\WindowsPowerShell\v1.0\powershell.exe" -Argument "-NoProfile -NonInteractive -ExecutionPolicy Bypass -File `"$worker`""
$principal = New-ScheduledTaskPrincipal -UserId 'SYSTEM' -LogonType ServiceAccount -RunLevel Highest
$settings = New-ScheduledTaskSettingsSet -ExecutionTimeLimit (New-TimeSpan -Minutes 3) -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries
Register-ScheduledTask -TaskName $taskName -Action $action -Principal $principal -Settings $settings | Out-Null
try {
    Start-ScheduledTask -TaskName $taskName
    $deadline = [Diagnostics.Stopwatch]::StartNew()
    while (-not (Test-Path -LiteralPath $result)) {
        if ($deadline.Elapsed.TotalSeconds -gt 180) { throw 'Native federation task exceeded its deadline' }
        Start-Sleep -Milliseconds 250
    }
    Get-Content -LiteralPath $log | Write-Output
    if ((Get-Content -LiteralPath $result -Raw).Trim() -ne '0') { throw 'Native federation task failed' }
    $events = @(Get-Content -LiteralPath $log | ForEach-Object { $_ | ConvertFrom-Json })
    $passes = @($events | Where-Object { $_.Action -eq 'pass' -and $null -ne $_.PSObject.Properties['Test'] -and $_.Test -eq 'TestFederationNativeRendererProfilePreservesOwner' })
    if ($passes.Count -ne 1 -or @($events | Where-Object Action -eq 'skip').Count -ne 0) { throw 'Native contract requires one actual pass and no skips' }
    Get-FileHash -Algorithm SHA256 -LiteralPath $image | Format-List | Out-String | Write-Output
} finally {
    Stop-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue
    Unregister-ScheduledTask -TaskName $taskName -Confirm:$false
}
