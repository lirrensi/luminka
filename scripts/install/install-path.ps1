# install-path.ps1 — Copy binary to ~\.local\bin, add to PATH, create desktop shortcut
# Placeholders: __APP_NAME__, __BINARY_NAME__

param(
  [string]$BinaryPath = ".\__BINARY_NAME__.exe"
)

$AppName = "__APP_NAME__"
$BinDir = "$env:USERPROFILE\.local\bin"
$ShortcutDir = [Environment]::GetFolderPath("Desktop")
$ShortcutPath = "$ShortcutDir\$AppName.lnk"

if (-not (Test-Path $BinaryPath)) {
  Write-Error "Binary not found at $BinaryPath — pass the path to your .exe"
  exit 1
}

New-Item -ItemType Directory -Path $BinDir -Force | Out-Null

Copy-Item $BinaryPath "$BinDir\__BINARY_NAME__.exe" -Force
Write-Host "Installed to $BinDir\__BINARY_NAME__.exe"

# Add to PATH in $PROFILE
$ProfilePath = $PROFILE.CurrentUserAllHosts
if (-not (Test-Path $ProfilePath)) {
  New-Item -ItemType File -Path $ProfilePath -Force | Out-Null
}
$ProfileContent = Get-Content $ProfilePath -Raw -ErrorAction SilentlyContinue
$PathLine = "`$env:Path += `";$BinDir`""
if ($ProfileContent -notmatch [regex]::Escape($BinDir)) {
  Add-Content -Path $ProfilePath -Value "`n# Added by $AppName install`n$PathLine"
  Write-Host "Added PATH to $ProfilePath"
}

# Create desktop shortcut
$WScript = New-Object -ComObject WScript.Shell
$Shortcut = $WScript.CreateShortcut($ShortcutPath)
$Shortcut.TargetPath = "$BinDir\__BINARY_NAME__.exe"
$Shortcut.WorkingDirectory = "$BinDir"
$Shortcut.Description = $AppName
$Shortcut.Save()
Write-Host "Created desktop shortcut: $ShortcutPath"

Write-Host ""
Write-Host "$AppName installed successfully."
Write-Host "Binary: $BinDir\__BINARY_NAME__.exe"
Write-Host ""
Write-Host "To start using from terminal, restart PowerShell or run:"
Write-Host '  $env:Path += ";'$BinDir'"'
