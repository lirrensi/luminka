# install-portable.ps1 — Create desktop shortcut to binary in-place (no copy)
# Placeholders: __APP_NAME__, __BINARY_NAME__

param(
  [string]$BinaryPath = ".\__BINARY_NAME__.exe"
)

$AppName = "__APP_NAME__"
$ShortcutDir = [Environment]::GetFolderPath("Desktop")
$ShortcutPath = "$ShortcutDir\$AppName.lnk"
$BinaryFullPath = Resolve-Path $BinaryPath

$WScript = New-Object -ComObject WScript.Shell
$Shortcut = $WScript.CreateShortcut($ShortcutPath)
$Shortcut.TargetPath = $BinaryFullPath
$Shortcut.WorkingDirectory = (Split-Path $BinaryFullPath -Parent)
$Shortcut.Description = $AppName
$Shortcut.Save()

Write-Host "Created desktop shortcut: $ShortcutPath"
Write-Host "Binary stays at: $BinaryFullPath"
