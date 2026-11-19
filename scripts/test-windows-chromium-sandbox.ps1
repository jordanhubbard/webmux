#Requires -Version 7.0
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$TestRoot,
    [Parameter(Mandatory = $true)][string]$ReceiptPath
)
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
if (-not $IsWindows) { throw 'Sandbox access controls require native Windows.' }
if (Test-Path -LiteralPath $TestRoot) { throw 'Scope-control root must be new and caller-owned.' }
$prepare = Join-Path $PSScriptRoot 'prepare-windows-chromium-sandbox.ps1'
$root = [IO.Path]::GetFullPath($TestRoot)
$cache = Join-Path $root 'cache'
$bundle = Join-Path $cache 'chromium-1/chrome-win64'
$canaryProfile = Join-Path $root 'private-profile'
New-Item -ItemType Directory -Path $bundle -Force | Out-Null
New-Item -ItemType Directory -Path $canaryProfile -Force | Out-Null
$exe = Join-Path $bundle 'chrome.exe'
$canary = Join-Path $canaryProfile 'chrome.exe'
[IO.File]::WriteAllText($exe, 'owned fixture: never execute')
[IO.File]::WriteAllText($canary, 'private profile canary')
$sections = [Security.AccessControl.AccessControlSections]::Access
function Get-Dacl($path) { return (Get-Acl -LiteralPath $path).GetSecurityDescriptorSddlForm($sections) }
function Expect-Refusal([string]$path, [string]$expected) {
    $refused = $false
    try {
        & $prepare -BrowserPath $path -InstallRoot $cache -ReceiptPath (Join-Path $root 'refused.json') | Out-Null
    } catch {
        if (-not $_.Exception.Message.Contains($expected, [StringComparison]::Ordinal)) { throw }
        $refused = $true
    }
    if (-not $refused) { throw 'Expected sandbox access refusal did not occur.' }
}
try {
    $canaryProfileDacl = Get-Dacl $canaryProfile
    $canaryDacl = Get-Dacl $canary
    $canaryHash = (Get-FileHash -LiteralPath $canary -Algorithm SHA256).Hash
    Expect-Refusal $canary 'restricted to the selected owned Playwright Chromium bundle'

    & $prepare -BrowserPath $exe -InstallRoot $cache -ReceiptPath (Join-Path $root 'first.json') | Out-Null
    $first = Get-Content -LiteralPath (Join-Path $root 'first.json') -Raw | ConvertFrom-Json
    $once = Get-Dacl $bundle
    $exeOnce = Get-Dacl $exe
    & $prepare -BrowserPath $exe -InstallRoot $cache -ReceiptPath (Join-Path $root 'second.json') | Out-Null
    if ((Get-Dacl $bundle) -cne $once -or (Get-Dacl $exe) -cne $exeOnce) {
        throw 'Sandbox access preparation was not idempotent.'
    }

    $junction = Join-Path $bundle 'outside-link'
    New-Item -ItemType Junction -Path $junction -Target $canaryProfile | Out-Null
    Expect-Refusal $exe 'bundle containing reparse points'
    Remove-Item -LiteralPath $junction -Force
    if ((Get-Dacl $bundle) -cne $once) { throw 'Reparse-point refusal changed bundle access.' }

    $acl = Get-Acl -LiteralPath $exe
    $sid = [Security.Principal.SecurityIdentifier]::new($first.capabilitySID)
    $deny = [Security.AccessControl.FileSystemAccessRule]::new($sid,
        [Security.AccessControl.FileSystemRights]::ReadAndExecute,
        [Security.AccessControl.AccessControlType]::Deny)
    $acl.AddAccessRule($deny)
    Set-Acl -LiteralPath $exe -AclObject $acl
    $deniedDacl = Get-Dacl $exe
    Expect-Refusal $exe 'explicit read/execute denial'
    if ((Get-Dacl $exe) -cne $deniedDacl -or (Get-Dacl $bundle) -cne $once) {
        throw 'Explicit-denial refusal changed access.'
    }
    if ((Get-Dacl $canaryProfile) -cne $canaryProfileDacl -or (Get-Dacl $canary) -cne $canaryDacl -or
        (Get-FileHash -LiteralPath $canary -Algorithm SHA256).Hash -ne $canaryHash) {
        throw 'Sandbox access preparation changed the private profile canary.'
    }
    $receipt = [ordered]@{
        schema = 1
        platform = 'windows'
        ownedFixtureCapabilityGrant = $true
        outsideInstallRootRefused = $true
        reparsePointRefused = $true
        explicitCapabilityDenialRefused = $true
        idempotent = $true
        privateProfileACLAndBytesUnchanged = $true
        nativeBrowserAcceptance = 'not-established-by-scope-controls'
    }
    New-Item -ItemType Directory -Path (Split-Path -Parent ([IO.Path]::GetFullPath($ReceiptPath))) -Force | Out-Null
    $receipt | ConvertTo-Json | Set-Content -LiteralPath $ReceiptPath -Encoding utf8
    $receipt | ConvertTo-Json
} finally {
    Remove-Item -LiteralPath $root -Recurse -Force
}
