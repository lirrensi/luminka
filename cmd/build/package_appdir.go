// FILE: cmd/build/package_appdir.go
// PURPOSE: Produce a macOS .app bundle from a built Luminka app binary.
// OWNS: macOS .app bundle packaging subcommand (appdir).
// EXPORTS: cmdPackageAppDir(appDir string) error
// DOCS: docs/installation.md#packaging-subcommands
// DEPENDS_ON: cmd/build/main.go (copyFile — all in package main)

package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func cmdPackageAppDir(appDir string) error {
	absDir, err := filepath.Abs(appDir)
	if err != nil {
		return fmt.Errorf("resolving app directory: %w", err)
	}

	appName := filepath.Base(absDir)
	binaryName := fmt.Sprintf("luminka-%s", appName)

	binaryPath := filepath.Join(absDir, binaryName)
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		cwd, _ := os.Getwd()
		binaryPath = filepath.Join(cwd, binaryName)
		if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
			return fmt.Errorf("built binary %q not found — run 'go run ./cmd/build %s' first", binaryName, appDir)
		}
	}

	cwd, _ := os.Getwd()
	appBundle := filepath.Join(cwd, fmt.Sprintf("%s.app", appName))

	// Remove existing .app if present
	os.RemoveAll(appBundle)

	macosDir := filepath.Join(appBundle, "Contents", "MacOS")
	resourcesDir := filepath.Join(appBundle, "Contents", "Resources")
	os.MkdirAll(macosDir, 0755)
	os.MkdirAll(resourcesDir, 0755)

	// Copy binary
	copyFile(binaryPath, filepath.Join(macosDir, binaryName))

	// Write Info.plist
	plistContent := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleExecutable</key>
    <string>%s</string>
    <key>CFBundleIdentifier</key>
    <string>com.luminka.%s</string>
    <key>CFBundleName</key>
    <string>%s</string>
    <key>CFBundleVersion</key>
    <string>1.0.0</string>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    <key>LSMinimumSystemVersion</key>
    <string>10.15</string>
</dict>
</plist>
`, binaryName, appName, appName)

	os.WriteFile(filepath.Join(appBundle, "Contents", "Info.plist"), []byte(plistContent), 0644)

	// Look for icon
	iconSrc := filepath.Join(absDir, "build", "icons", "macos", "icon.icns")
	if _, err := os.Stat(iconSrc); err == nil {
		copyFile(iconSrc, filepath.Join(resourcesDir, "icon.icns"))
	}

	fmt.Printf("[package] Created: %s/\n", filepath.Base(appBundle))
	return nil
}
