[CmdletBinding()]
param(
  [Parameter(Mandatory=$true)][string]$ArtifactDirectory,
  [Parameter(Mandatory=$true)][ValidatePattern('^\d+\.\d+\.\d+$')][string]$Version,
  [Parameter(Mandatory=$true)][string]$Output
)
$ErrorActionPreference='Stop'
if(Test-Path -LiteralPath $Output){throw 'Inspection output exists; preserve or remove it explicitly before retrying'}
$engine=New-Object -ComObject WindowsInstaller.Installer
$results=@()
try {
  foreach($architecture in @('x64','arm64')) {
    $name="webmux-$Version-windows-$architecture-native.msi"
    $file=(Resolve-Path -LiteralPath (Join-Path $ArtifactDirectory $name)).Path
    $database=$engine.OpenDatabase($file,0)
    $properties=@{}
    foreach($property in @('ProductCode','UpgradeCode','ProductName','ProductVersion','Manufacturer','ALLUSERS','MSIINSTALLPERUSER')) {
      $view=$database.OpenView('SELECT `Value` FROM `Property` WHERE `Property` = '''+$property+'''')
      $view.Execute();$row=$view.Fetch()
      $properties[$property]=if($row){$row.GetType().InvokeMember('StringData',[Reflection.BindingFlags]::GetProperty,$null,$row,@(1))}else{$null}
      $view.Close()
    }
    $summary=$database.GetType().InvokeMember('SummaryInformation',[Reflection.BindingFlags]::GetProperty,$null,$database,@(0))
    $template=$summary.GetType().InvokeMember('Property',[Reflection.BindingFlags]::GetProperty,$null,$summary,@(7))
    $results+=[pscustomobject]@{architecture=$architecture;name=$name;url="https://github.com/jordanhubbard/webmux/releases/download/v$Version/$name";sha256=(Get-FileHash -LiteralPath $file -Algorithm SHA256).Hash.ToLowerInvariant();properties=$properties;template=$template}
  }
  $json=[pscustomobject]@{version=$Version;installers=$results}|ConvertTo-Json -Depth 6
  [IO.File]::WriteAllText($Output,$json,(New-Object Text.UTF8Encoding $false))
} finally {[void][Runtime.InteropServices.Marshal]::ReleaseComObject($engine)}
# Reads MSI databases only. Does not execute installers, custom actions or payloads.
Write-Output "Inspected both MSI databases: $Output"
