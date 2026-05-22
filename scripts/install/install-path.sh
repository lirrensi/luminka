#!/usr/bin/env bash
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
