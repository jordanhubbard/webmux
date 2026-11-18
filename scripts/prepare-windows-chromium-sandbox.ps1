#Requires -Version 7.0
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$BrowserPath,
    [Parameter(Mandatory = $true)][string]$InstallRoot,
    [Parameter(Mandatory = $true)][string]$ReceiptPath
)
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
if (-not $IsWindows) { throw 'Chromium sandbox installation access requires native Windows.' }

# Only the caller-owned Playwright bundle is eligible; never touch profiles or
# a system browser. Reject links before adding inheritable access.
$exe = (Resolve-Path -LiteralPath $BrowserPath).ProviderPath
$cache = (Resolve-Path -LiteralPath $InstallRoot).ProviderPath.TrimEnd('\')
$bundle = Split-Path -Parent $exe
$version = Split-Path -Parent $bundle
if ([IO.Path]::GetFileName($exe) -ine 'chrome.exe' -or
    (Split-Path -Parent $version) -ine $cache -or
    (Split-Path -Leaf $version) -notmatch '^chromium-[0-9]+$' -or
    (Split-Path -Leaf $bundle) -notin @('chrome-win', 'chrome-win64')) {
    throw 'Sandbox access is restricted to the selected owned Playwright Chromium bundle.'
}
foreach ($path in @($cache, $version, $bundle)) {
    if ((Get-Item -LiteralPath $path -Force).Attributes -band [IO.FileAttributes]::ReparsePoint) {
        throw 'Refusing sandbox access through a reparse point.'
    }
}
$pending = [Collections.Generic.Queue[string]]::new()
$pending.Enqueue($bundle)
while ($pending.Count -ne 0) {
    foreach ($entry in @(Get-ChildItem -LiteralPath ($pending.Dequeue()) -Force -ErrorAction Stop)) {
        if ($entry.Attributes -band [IO.FileAttributes]::ReparsePoint) {
            throw 'Refusing sandbox access to a bundle containing reparse points.'
        }
        if ($entry.PSIsContainer) { $pending.Enqueue($entry.FullName) }
    }
}

# Chromium sandbox/policy/win/sandbox_win.cc adds this named capability to its
# AppContainer configuration before checking executable read/execute access.
# Windows API: https://learn.microsoft.com/windows/win32/api/securitybaseapi/nf-securitybaseapi-derivecapabilitysidsfromname
# Derive it through the documented Windows API; do not grant all applications.
if (-not ('WebMuxChromiumCapability' -as [type])) {
    Add-Type -TypeDefinition @'
using System;
using System.ComponentModel;
using System.Runtime.InteropServices;
using System.Security.Principal;
public static class WebMuxChromiumCapability {
    [DllImport("kernelbase.dll", CharSet=CharSet.Unicode, ExactSpelling=true, SetLastError=true)]
    [return: MarshalAs(UnmanagedType.Bool)]
    static extern bool DeriveCapabilitySidsFromName(string name,
        out IntPtr groups, out uint groupCount, out IntPtr capabilities, out uint capabilityCount);
    [DllImport("kernel32.dll", ExactSpelling=true)]
    static extern IntPtr LocalFree(IntPtr memory);
    static void FreeArray(IntPtr array, uint count) {
        if (array == IntPtr.Zero) return;
        for (int i=0; i<count; i++) {
            IntPtr sid=Marshal.ReadIntPtr(array, checked(i*IntPtr.Size));
            if (sid != IntPtr.Zero) LocalFree(sid);
        }
        LocalFree(array);
    }
    public static string InstallFilesSid() {
        IntPtr groups=IntPtr.Zero, capabilities=IntPtr.Zero;
        uint groupCount=0, capabilityCount=0;
        try {
            if (!DeriveCapabilitySidsFromName("lpacChromeInstallFiles", out groups,
                    out groupCount, out capabilities, out capabilityCount))
                throw new Win32Exception(Marshal.GetLastWin32Error());
            if (capabilityCount != 1 || capabilities == IntPtr.Zero)
                throw new InvalidOperationException("Unexpected Chromium capability count.");
            return new SecurityIdentifier(Marshal.ReadIntPtr(capabilities)).Value;
        } finally {
            FreeArray(groups, groupCount);
            FreeArray(capabilities, capabilityCount);
        }
    }
}
'@
}
$sidText = [WebMuxChromiumCapability]::InstallFilesSid()
if (-not $sidText.StartsWith('S-1-15-3-1024-', [StringComparison]::Ordinal)) {
    throw 'Unexpected named capability SID; no access changed.'
}
$sid = [Security.Principal.SecurityIdentifier]::new($sidText)
$rights = [Security.AccessControl.FileSystemRights]::ReadAndExecute -bor
    [Security.AccessControl.FileSystemRights]::Synchronize
$inheritance = [Security.AccessControl.InheritanceFlags]::ContainerInherit -bor
    [Security.AccessControl.InheritanceFlags]::ObjectInherit

function Get-OtherRules($acl) {
    return (@($acl.GetAccessRules($true, $true, [Security.Principal.SecurityIdentifier]) |
        Where-Object { $_.IdentityReference.Value -ne $sidText } |
        ForEach-Object { '{0}|{1}|{2}|{3}|{4}|{5}' -f $_.IdentityReference.Value,
            $_.AccessControlType, [int]$_.FileSystemRights, $_.InheritanceFlags,
            $_.PropagationFlags, $_.IsInherited } | Sort-Object) -join "`n")
}
function Get-CapabilityAllowRights($acl) {
    $allowed = 0
    foreach ($rule in $acl.GetAccessRules($true, $true, [Security.Principal.SecurityIdentifier])) {
        if ($rule.IdentityReference.Value -eq $sidText -and
            $rule.AccessControlType -eq [Security.AccessControl.AccessControlType]::Allow) {
            $allowed = $allowed -bor [int]$rule.FileSystemRights
        }
    }
    return $allowed
}
function Test-CapabilityReadDeny($acl) {
    foreach ($entry in $acl.GetAccessRules($true, $true, [Security.Principal.SecurityIdentifier])) {
        if ($entry.IdentityReference.Value -eq $sidText -and
            $entry.AccessControlType -eq [Security.AccessControl.AccessControlType]::Deny -and
            ([int]$entry.FileSystemRights -band [int]$rights) -ne 0) { return $true }
    }
    return $false
}
if ((Test-CapabilityReadDeny (Get-Acl -LiteralPath $bundle)) -or
    (Test-CapabilityReadDeny (Get-Acl -LiteralPath $exe))) {
    throw 'Selected Chromium capability has an explicit read/execute denial; no access changed.'
}
$hashBefore = (Get-FileHash -LiteralPath $exe -Algorithm SHA256).Hash
$acl = Get-Acl -LiteralPath $bundle
$beforeOther = Get-OtherRules $acl
$beforeOwner = $acl.GetOwner([Security.Principal.SecurityIdentifier]).Value
$beforeGroup = $acl.GetGroup([Security.Principal.SecurityIdentifier]).Value
$beforeProtected = $acl.AreAccessRulesProtected
$beforeRights = Get-CapabilityAllowRights $acl
$fileBefore = Get-CapabilityAllowRights (Get-Acl -LiteralPath $exe)
$rule = [Security.AccessControl.FileSystemAccessRule]::new($sid, $rights, $inheritance,
    [Security.AccessControl.PropagationFlags]::None,
    [Security.AccessControl.AccessControlType]::Allow)
$hasGrant = $false
foreach ($entry in $acl.GetAccessRules($true, $true, [Security.Principal.SecurityIdentifier])) {
    if ($entry.IdentityReference.Value -eq $sidText -and
        $entry.AccessControlType -eq [Security.AccessControl.AccessControlType]::Allow -and
        ([int]$entry.FileSystemRights -band [int]$rights) -eq [int]$rights -and
        ($entry.InheritanceFlags -band $inheritance) -eq $inheritance -and
        $entry.PropagationFlags -eq [Security.AccessControl.PropagationFlags]::None) {
        $hasGrant = $true
    }
}
if (-not $hasGrant) {
    $acl.AddAccessRule($rule)
    Set-Acl -LiteralPath $bundle -AclObject $acl
}
$after = Get-Acl -LiteralPath $bundle
$fileAfter = Get-CapabilityAllowRights (Get-Acl -LiteralPath $exe)
$afterRights = Get-CapabilityAllowRights $after
if ((Get-OtherRules $after) -cne $beforeOther -or
    $after.GetOwner([Security.Principal.SecurityIdentifier]).Value -ne $beforeOwner -or
    $after.GetGroup([Security.Principal.SecurityIdentifier]).Value -ne $beforeGroup -or
    $after.AreAccessRulesProtected -ne $beforeProtected -or
    (($afterRights -band (-bnot $beforeRights)) -band (-bnot [int]$rights)) -ne 0 -or
    ($fileAfter -band [int]$rights) -ne [int]$rights) {
    throw 'Chromium capability access verification failed.'
}
$hashAfter = (Get-FileHash -LiteralPath $exe -Algorithm SHA256).Hash
if ($hashAfter -ne $hashBefore) { throw 'Chromium executable bytes changed during access preparation.' }
$receipt = [ordered]@{
    schema = 1
    changed = (-not $hasGrant)
    capability = 'lpacChromeInstallFiles'
    capabilitySID = $sidText
    allowedRights = 'ReadAndExecute,Synchronize'
    scope = 'selected-owned-playwright-chromium-bundle'
    beforeExecutableCapabilityAllow = (($fileBefore -band [int]$rights) -eq [int]$rights)
    afterExecutableCapabilityAllow = (($fileAfter -band [int]$rights) -eq [int]$rights)
    executableSHA256 = $hashAfter.ToLowerInvariant()
    otherRulesOwnerGroupProtectionUnchanged = $true
    nativeSandboxAcceptance = 'not-established-by-ACL-receipt'
}
$parent = Split-Path -Parent ([IO.Path]::GetFullPath($ReceiptPath))
New-Item -ItemType Directory -Path $parent -Force | Out-Null
$receipt | ConvertTo-Json -Depth 4 | Set-Content -LiteralPath $ReceiptPath -Encoding utf8
$receipt | ConvertTo-Json -Depth 4
