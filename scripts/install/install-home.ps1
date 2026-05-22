# install-home.ps1 — Install to ~\.app-name\ with fixed root
# Placeholders: __APP_NAME__, __BINARY_NAME__

param(
  [string]$BinaryPath = ".\__BINARY_NAME__.exe"
)

$AppName = "__APP_NAME__"
$InstallDir = "$env:USERPROFILE\.$AppName"
$ShortcutDir = [Environment]::GetFolderPath("Desktop")
$ShortcutPath = "$ShortcutDir\$AppName.lnk"

New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
Copy-Item $BinaryPath "$InstallDir\__BINARY_NAME__.exe" -Force

$WScript = New-Object -ComObject WScript.Shell
$Shortcut = $WScript.CreateShortcut($ShortcutPath)
$Shortcut.TargetPath = "$InstallDir\__BINARY_NAME__.exe"
$Shortcut.Arguments = "--root `"$InstallDir`""
$Shortcut.WorkingDirectory = $InstallDir
$Shortcut.Description = $AppName
$Shortcut.Save()

Write-Host "Installed to $InstallDir"
Write-Host "All app data will stay in $InstallDir"
Write-Host "To fully remove: Remove-Item -Recurse -Force '$InstallDir'"
