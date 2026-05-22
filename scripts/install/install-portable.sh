#!/usr/bin/env bash
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
