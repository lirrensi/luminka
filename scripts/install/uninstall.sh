#!/usr/bin/env bash
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
