# Installation and Distribution Guide

This document is the map for app developers (people building apps with Luminka) who want to distribute their app to end users.

Luminka apps are portable by default — the binary runs from any folder with no installation required. This guide covers the **optional** step of making that binary discoverable and convenient for end users who want it in their PATH, on their desktop, or delivered through a platform package.

---

## The Map

### What kind of distribution fits your app?

| You want your users to... | Use this |
|---|---|
| Copy the `.exe` anywhere and run it | No scripts needed — this is the default portable behavior. Ship the raw binary. |
| Run the app from any terminal by typing its name | [`install-path.sh` / `.ps1`](#1-install-path) — copies to `~/.local/bin/` and adds to PATH |
| Keep the binary in a sync folder (Dropbox, Git), still want a desktop shortcut | [`install-portable.sh` / `.ps1`](#2-install-portable) — creates a shortcut without moving the binary |
| Install the app to a dedicated home folder with sandboxed data | [`install-home.sh` / `.ps1`](#3-install-home) — copies to `~/.app-name/` and fixes the root there |
| Remove any of the above cleanly | [`uninstall.sh` / `.ps1`](#4-uninstall) — reverses PATH, shortcut, and data |
| Attach a Windows `.zip` to your release | [`go run ./cmd/build zip`](#packaging-subcommands) — produces `app-windows-x64.zip` |
| Attach a Linux `.tar.gz` to your release | [`go run ./cmd/build tar`](#packaging-subcommands) — produces `app-linux-x64.tar.gz` |
| Attach a Debian/Ubuntu `.deb` to your release | [`go run ./cmd/build deb`](#packaging-subcommands) — produces `app_1.0.0_amd64.deb` |
| Attach a macOS `.app` bundle to your release | [`go run ./cmd/build appdir`](#packaging-subcommands) — produces `App.app/` |

---

## Install Script Templates

All install script templates live in `scripts/install/` in the Luminka repository. They are designed to be **copied into your own project**, customized with your app's name, and shipped alongside your binary.

### How to use them

1. Copy the script(s) that match your distribution model into your own repo.
2. Replace the placeholder strings (`__APP_NAME__`, `__BINARY_NAME__`, etc.) with your app's values.
3. Include the customized scripts in your release artifacts.
4. Publish alongside your binary — end users run the script after downloading.

### Placeholder reference

| Placeholder | What to replace with |
|---|---|
| `__APP_NAME__` | Your app's display name, e.g. `My Kanban` |
| `__BINARY_NAME__` | Your binary filename (without path), e.g. `my-kanban-app` |
| `__BINARY_NAME__.exe` | Same but with Windows extension, e.g. `my-kanban-app.exe` |
| `__SHORTCUT_NAME__` | Name for the desktop shortcut, e.g. `My Kanban` |
| `__DESKTOP_COMMENT__` | Short description for `.desktop` file, e.g. `A kanban board for local project files` |
| `__INTERNET_DOMAIN__` | Reverse-domain identifier for `.desktop` file, e.g. `com.example.mykanban` |

---

### 1. Install-Path

**Scenario**: The user wants to run the app from any terminal by typing its name, and also see it on their desktop or in their app launcher.

**What it does**:

- Copies the binary to `~/.local/bin/<binary_name>` (the `~/.local/bin/` directory is the POSIX convention for user-scoped executables; Windows uses `%USERPROFILE%\.local\bin\`)
- Adds that directory to `PATH` in the user's shell profile if not already present
- Creates a desktop shortcut (`.desktop` on Linux, `.lnk` on Windows, `.command` or `.app` wrapper on macOS)
- Prints what it did so the user knows the result

**Linux / macOS** — `install-path.sh`:

```bash
# install-path.sh — Copy binary to ~/.local/bin, add to PATH, create desktop shortcut
# Placeholders: __APP_NAME__, __BINARY_NAME__, __SHORTCUT_NAME__, __DESKTOP_COMMENT__, __INTERNET_DOMAIN__

set -euo pipefail

APP_NAME="__APP_NAME__"
BINARY_NAME="__BINARY_NAME__"
SHORTCUT_NAME="__SHORTCUT_NAME__"
DESKTOP_COMMENT="__DESKTOP_COMMENT__"
INTERNET_DOMAIN="__INTERNET_DOMAIN__"

BIN_DIR="$HOME/.local/bin"
SHORTCUT_DIR="$HOME/Desktop"
DATA_DIR="$HOME/.local/share/applications"

SOURCE="${1:-./$BINARY_NAME}"
if [ ! -f "$SOURCE" ]; then
  echo "Usage: $0 [path-to-binary]"
  echo "Binary not found at $SOURCE — pass the path to your built binary."
  exit 1
fi

mkdir -p "$BIN_DIR" "$DATA_DIR"

# Copy binary
cp "$SOURCE" "$BIN_DIR/$BINARY_NAME"
chmod +x "$BIN_DIR/$BINARY_NAME"
echo "Installed to $BIN_DIR/$BINARY_NAME"

# Add to PATH in shell profile
for RC in "$HOME/.bashrc" "$HOME/.zshrc" "$HOME/.config/fish/config.fish"; do
  if [ -f "$RC" ]; then
    case "$RC" in
      *.fish)
        LINE="fish_add_path $BIN_DIR"
        ;;
      *)
        LINE="export PATH=\"\$PATH:$BIN_DIR\""
        ;;
    esac
    if ! grep -qF "$BIN_DIR" "$RC" 2>/dev/null; then
      echo "" >> "$RC"
      echo "# Added by $APP_NAME install" >> "$RC"
      echo "$LINE" >> "$RC"
      echo "Added PATH to $RC"
    fi
  fi
done

# Desktop shortcut (Linux)
if [ "$(uname)" = "Linux" ]; then
  DESKTOP_FILE="$SHORTCUT_DIR/$SHORTCUT_NAME.desktop"
  mkdir -p "$SHORTCUT_DIR"
  cat > "$DESKTOP_FILE" << EOF
[Desktop Entry]
Type=Application
Name=$SHORTCUT_NAME
Comment=$DESKTOP_COMMENT
Exec=$BIN_DIR/$BINARY_NAME
Icon=$BIN_DIR/$BINARY_NAME
Terminal=false
Categories=Utility;
StartupWMClass=$BINARY_NAME
EOF
  chmod +x "$DESKTOP_FILE"
  # Also copy to applications directory for app launcher
  cp "$DESKTOP_FILE" "$DATA_DIR/$INTERNET_DOMAIN.$BINARY_NAME.desktop"
  echo "Created desktop shortcut: $DESKTOP_FILE"
fi

# macOS: create a .command file on Desktop
if [ "$(uname)" = "Darwin" ]; then
  COMMAND_FILE="$SHORTCUT_DIR/$SHORTCUT_NAME.command"
  echo "#!/bin/bash" > "$COMMAND_FILE"
  echo "exec \"$BIN_DIR/$BINARY_NAME\"" >> "$COMMAND_FILE"
  chmod +x "$COMMAND_FILE"
  echo "Created desktop shortcut: $COMMAND_FILE"
fi

echo ""
echo "$APP_NAME installed successfully."
echo "Binary: $BIN_DIR/$BINARY_NAME"
echo "Desktop: $SHORTCUT_DIR/$SHORTCUT_NAME (desktop shortcut)"
echo ""
echo "To start using from terminal, restart your shell or run:"
echo "  export PATH=\"\$PATH:$BIN_DIR\""
```

**Windows** — `install-path.ps1`:

```powershell
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
```

---

### 2. Install-Portable

**Scenario**: The user keeps the binary in a synchronized folder (Dropbox, Git repo, USB drive) and just wants a desktop shortcut to launch it from there.

**What it does**:

- Does **not** move the binary
- Does **not** touch PATH or shell profiles
- Creates a desktop shortcut that points to the binary at its current location
- On Linux, also registers the shortcut in `~/.local/share/applications` for app launcher integration

**Linux / macOS** — `install-portable.sh`:

```bash
# install-portable.sh — Create desktop shortcut to binary in-place (no copy)
# Placeholders: __SHORTCUT_NAME__, __BINARY_NAME__, __DESKTOP_COMMENT__, __INTERNET_DOMAIN__

set -euo pipefail

SHORTCUT_NAME="__SHORTCUT_NAME__"
BINARY_NAME="__BINARY_NAME__"
DESKTOP_COMMENT="__DESKTOP_COMMENT__"
INTERNET_DOMAIN="__INTERNET_DOMAIN__"

SOURCE="${1:-./$BINARY_NAME}"
if [ ! -f "$SOURCE" ]; then
  echo "Usage: $0 [path-to-binary]"
  echo "Binary not found at $SOURCE"
  exit 1
fi

# Resolve absolute path
SOURCE="$(cd "$(dirname "$SOURCE")" && pwd)/$(basename "$SOURCE")"
SHORTCUT_DIR="$HOME/Desktop"
DATA_DIR="$HOME/.local/share/applications"
mkdir -p "$SHORTCUT_DIR" "$DATA_DIR"

if [ "$(uname)" = "Linux" ]; then
  DESKTOP_FILE="$SHORTCUT_DIR/$SHORTCUT_NAME.desktop"
  cat > "$DESKTOP_FILE" << EOF
[Desktop Entry]
Type=Application
Name=$SHORTCUT_NAME
Comment=$DESKTOP_COMMENT
Exec=$SOURCE
Terminal=false
Categories=Utility;
StartupWMClass=$BINARY_NAME
EOF
  chmod +x "$DESKTOP_FILE"
  cp "$DESKTOP_FILE" "$DATA_DIR/$INTERNET_DOMAIN.$BINARY_NAME.desktop"
  echo "Created desktop shortcut: $DESKTOP_FILE"
fi

if [ "$(uname)" = "Darwin" ]; then
  COMMAND_FILE="$SHORTCUT_DIR/$SHORTCUT_NAME.command"
  echo "#!/bin/bash" > "$COMMAND_FILE"
  echo "exec \"$SOURCE\"" >> "$COMMAND_FILE"
  chmod +x "$COMMAND_FILE"
  echo "Created desktop shortcut: $COMMAND_FILE"
fi

echo "Binary stays at: $SOURCE"
```

**Windows** — `install-portable.ps1`:

```powershell
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
```

---

### 3. Install-Home

**Scenario**: The user wants the app installed cleanly in a dedicated folder — the data stays with the app and is sandboxed from wherever the user happens to run the shortcut.

**What it does**:

- Copies the binary to `~/.<app-name>/`
- Creates a desktop shortcut that launches the binary with `--root` pointing to that folder
- The app's data, logs, and state all live inside `~/.<app-name>/`
- The user can delete `~/.<app-name>/` to fully remove the app

**Linux / macOS** — `install-home.sh`:

```bash
# install-home.sh — Install to ~/.app-name/ with fixed root
# Placeholders: __APP_NAME__, __BINARY_NAME__, __SHORTCUT_NAME__

set -euo pipefail

APP_NAME="__APP_NAME__"
BINARY_NAME="__BINARY_NAME__"
SHORTCUT_NAME="__SHORTCUT_NAME__"

SOURCE="${1:-./$BINARY_NAME}"
if [ ! -f "$SOURCE" ]; then
  echo "Usage: $0 [path-to-binary]"
  echo "Binary not found at $SOURCE"
  exit 1
fi

INSTALL_DIR="$HOME/.$APP_NAME"
SHORTCUT_DIR="$HOME/Desktop"
DATA_DIR="$HOME/.local/share/applications"
mkdir -p "$INSTALL_DIR" "$SHORTCUT_DIR" "$DATA_DIR"

cp "$SOURCE" "$INSTALL_DIR/$BINARY_NAME"
chmod +x "$INSTALL_DIR/$BINARY_NAME"

# Create launch script that uses --root
LAUNCHER="$INSTALL_DIR/run.sh"
cat > "$LAUNCHER" << EOF
#!/bin/bash
exec "$INSTALL_DIR/$BINARY_NAME" --root "$INSTALL_DIR"
EOF
chmod +x "$LAUNCHER"

if [ "$(uname)" = "Linux" ]; then
  DESKTOP_FILE="$SHORTCUT_DIR/$SHORTCUT_NAME.desktop"
  cat > "$DESKTOP_FILE" << EOF
[Desktop Entry]
Type=Application
Name=$SHORTCUT_NAME
Exec=$LAUNCHER
Icon=$INSTALL_DIR/$BINARY_NAME
Terminal=false
Categories=Utility;
EOF
  chmod +x "$DESKTOP_FILE"
  cp "$DESKTOP_FILE" "$DATA_DIR/$APP_NAME.desktop"
  echo "Created desktop shortcut: $DESKTOP_FILE"
fi

if [ "$(uname)" = "Darwin" ]; then
  COMMAND_FILE="$SHORTCUT_DIR/$SHORTCUT_NAME.command"
  echo "#!/bin/bash" > "$COMMAND_FILE"
  echo "exec \"$LAUNCHER\"" >> "$COMMAND_FILE"
  chmod +x "$COMMAND_FILE"
fi

echo "Installed to $INSTALL_DIR"
echo "All app data will stay in $INSTALL_DIR"
echo "To fully remove: rm -rf $INSTALL_DIR"
```

**Windows** — `install-home.ps1`:

```powershell
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
```

---

### 4. Uninstall

**Scenario**: The user wants to cleanly remove the app and optionally its data.

**What it does**:

- Removes the binary from `~/.local/bin/` (path install) or `~/.<app-name>/` (home install)
- Removes the desktop shortcut
- Removes the PATH entry from shell profiles (path install)
- Optionally removes app data (prompts first)

**Linux / macOS** — `uninstall.sh`:

```bash
# uninstall.sh — Remove app from PATH, delete shortcut, optionally delete data
# Placeholders: __APP_NAME__, __BINARY_NAME__, __INTERNET_DOMAIN__

set -euo pipefail

APP_NAME="__APP_NAME__"
BINARY_NAME="__BINARY_NAME__"
INTERNET_DOMAIN="__INTERNET_DOMAIN__"
INSTALL_DIR="$HOME/.local/bin"
HOME_DIR="$HOME/.$APP_NAME"
SHORTCUT_DIR="$HOME/Desktop"
DATA_DIR="$HOME/.local/share/applications"

REMOVED=false

# Remove from ~/.local/bin
if [ -f "$INSTALL_DIR/$BINARY_NAME" ]; then
  rm "$INSTALL_DIR/$BINARY_NAME"
  echo "Removed binary: $INSTALL_DIR/$BINARY_NAME"
  REMOVED=true
fi

# Remove home install directory
if [ -d "$HOME_DIR" ]; then
  rm -rf "$HOME_DIR"
  echo "Removed home install: $HOME_DIR"
  REMOVED=true
fi

# Remove desktop shortcuts
for f in "$SHORTCUT_DIR/$APP_NAME"* "$SHORTCUT_DIR/$BINARY_NAME"* "$SHORTCUT_DIR/$INTERNET_DOMAIN.$BINARY_NAME"*; do
  if [ -f "$f" ]; then
    rm "$f"
    echo "Removed shortcut: $f"
    REMOVED=true
  fi
done

# Remove from applications directory
for f in "$DATA_DIR/$INTERNET_DOMAIN.$BINARY_NAME"* "$DATA_DIR/$APP_NAME"*; do
  if [ -f "$f" ]; then
    rm "$f"
    REMOVED=true
  fi
done

# Remove PATH entry from shell profiles
for RC in "$HOME/.bashrc" "$HOME/.zshrc" "$HOME/.config/fish/config.fish"; do
  if [ -f "$RC" ]; then
    sed -i "/# Added by $APP_NAME install/d" "$RC" 2>/dev/null || true
    sed -i "\|$INSTALL_DIR|d" "$RC" 2>/dev/null || true
  fi
done

if [ "$REMOVED" = false ]; then
  echo "$APP_NAME does not appear to be installed."
  exit 0
fi

echo ""
echo "$APP_NAME has been removed."

# Optionally remove data
if [ -d "$HOME_DIR" ]; then
  echo ""
  echo "App data may still exist at: $HOME_DIR"
  echo "Run 'rm -rf $HOME_DIR' to remove it."
fi
```

**Windows** — `uninstall.ps1`:

```powershell
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
```

---

## Packaging Subcommands

The build CLI at `cmd/build/` supports platform packaging subcommands. These run **after** `go build` has produced the binary. Each subcommand wraps the binary and install scripts into a distribution archive.

### Prerequisites

- The app has been built (a binary exists at `<app-dir>/<binary-name>`)
- The install scripts you want to ship are in `<app-dir>/scripts/install/`
- Platform-specific packaging tools may be needed:
  - `.deb` requires `dpkg-deb` (available on Debian/Ubuntu)
  - `.appdir` requires no external tools (creates a macOS `.app` folder structure)

### Subcommand reference

| Subcommand | Output | Platform | External deps |
|---|---|---|---|
| `zip` | `<name>-<version>-windows-x64.zip` | Windows | None |
| `tar` | `<name>-<version>-linux-x64.tar.gz` | Linux | None |
| `deb` | `<name>_<version>_amd64.deb` | Linux | `dpkg-deb` |
| `appdir` | `<name>.app/` | macOS | None |

### Usage

```bash
# Build the app first
go run ./cmd/build ./starter

# Package into a .zip
go run ./cmd/build zip ./starter

# Package into a .deb
go run ./cmd/build deb ./starter

# Package into a macOS .app
go run ./cmd/build appdir ./starter

# Specify a custom output name
go run ./cmd/build zip ./starter --out my-app-v1.0.zip
```

Each subcommand:
1. Resolves the app directory and its built binary
2. Scans for install scripts in `scripts/install/` inside the app directory
3. Bundles the binary + chosen scripts into the archive
4. Validates the result
5. Errors out clearly if the format cannot be produced on the current host

### CI integration example (GitHub Actions)

```yaml
release:
  strategy:
    matrix:
      os: [ubuntu-latest, macos-latest, windows-latest]
  steps:
    - uses: actions/checkout@v4
    - uses: actions/setup-go@v5
    - run: go run ./cmd/build ./starter

    - name: Package (Linux)
      if: runner.os == 'Linux'
      run: |
        go run ./cmd/build tar ./starter
        go run ./cmd/build deb ./starter

    - name: Package (macOS)
      if: runner.os == 'macOS'
      run: go run ./cmd/build appdir ./starter

    - name: Package (Windows)
      if: runner.os == 'Windows'
      run: go run ./cmd/build zip ./starter

    - name: Upload artifacts
      uses: actions/upload-artifact@v4
      with:
        path: |
          *.zip
          *.tar.gz
          *.deb
          *.app/
```

---

## Choosing what to ship

The full matrix of options can feel overwhelming. Here is a simple rule of thumb:

> **Ship the raw binary + the install scripts that match how you expect users to use the app.**

| Your app is... | Ship these |
|---|---|
| A portable tool (like a kanban for one project folder) | Just the binary. Users keep it in their project. |
| A utility users reach for often (note app, timer, launcher) | Binary + `install-path.sh` / `.ps1` |
| A media organizer or long-running app users keep open | Binary + `install-home.sh` / `.ps1` (sandboxed data) |
| Closing a sync folder and want a desktop shortcut | Binary + `install-portable.sh` / `.ps1` |

Always include `uninstall.sh` / `.ps1` when you ship any install script.

Attach platform archives (`.zip`, `.tar.gz`, `.deb`, `.app`) if you ship for users who expect standard package formats. This is optional — the binary + install scripts remain the primary distribution path.

---

## Script maintenance contract

The scripts in `scripts/install/` are shipped as **templates**. They are correct at the time of the Luminka release. If you customize them for your app:

- Keep the placeholders clearly named so a future Luminka version bump only requires replacing the same strings
- Do not embed secrets or paths specific to your build machine
- Test on a clean user profile before shipping

The Luminka repository guarantees that the script templates are internally consistent (placeholders match between scripts, the uninstall reverses all install actions). The framework does not own your customized copies.
