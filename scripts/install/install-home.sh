#!/usr/bin/env bash
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
