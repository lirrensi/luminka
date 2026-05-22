# uninstall.ps1 — Remove app from PATH, delete shortcut, optionally delete data
# Placeholders: __APP_NAME__, __BINARY_NAME__

param(
  [switch]$RemoveData
)

$AppName = "__APP_NAME__"
$BinDir = "$env:USERPROFILE\.local\bin"
$HomeDir = "$env:USERPROFILE\.$AppName"
$ShortcutDir = [Environment]::GetFolderPath("Desktop")
$ShortcutPath = "$ShortcutDir\$AppName.lnk"
$BinaryPath = "$BinDir\__BINARY_NAME__.exe"

$Removed = $false

if (Test-Path $BinaryPath) {
  Remove-Item $BinaryPath -Force
  Write-Host "Removed binary: $BinaryPath"
  $Removed = $true
}

if (Test-Path $HomeDir) {
  if ($RemoveData) {
    Remove-Item $HomeDir -Recurse -Force
    Write-Host "Removed home install: $HomeDir"
  } else {
    Write-Host "Home install data preserved: $HomeDir (use -RemoveData to delete)"
  }
  $Removed = $true
}

if (Test-Path $ShortcutPath) {
  Remove-Item $ShortcutPath -Force
  Write-Host "Removed shortcut: $ShortcutPath"
  $Removed = $true
}

# Remove PATH entry from $PROFILE
$ProfilePath = $PROFILE.CurrentUserAllHosts
if (Test-Path $ProfilePath) {
  $content = Get-Content $ProfilePath -Raw
  $newContent = $content -replace "(?s)# Added by $AppName install.*`n.*`n?", ""
  if ($newContent -ne $content) {
    Set-Content $ProfilePath $newContent
    Write-Host "Removed PATH from $ProfilePath"
  }
}

if (-not $Removed) {
  Write-Host "$AppName does not appear to be installed."
}
