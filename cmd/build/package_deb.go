// FILE: cmd/build/package_deb.go
// PURPOSE: Produce a Debian/Ubuntu .deb package from a built Luminka app binary.
// OWNS: Debian .deb packaging subcommand (deb).
// EXPORTS: cmdPackageDeb(appDir string) error
// DOCS: docs/installation.md#packaging-subcommands
// DEPENDS_ON: cmd/build/main.go (copyFile — all in package main)

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

func cmdPackageDeb(appDir string) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf(".deb packaging is only supported on Linux")
	}

	// Check dpkg-deb availability
	if _, err := exec.LookPath("dpkg-deb"); err != nil {
		return fmt.Errorf("dpkg-deb not found — install dpkg-dev to build .deb packages")
	}

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

	tmpDir, err := os.MkdirTemp("", "luminka-deb-*")
	if err != nil {
		return fmt.Errorf("creating temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// DEBIAN/control
	debDir := filepath.Join(tmpDir, fmt.Sprintf("%s_1.0.0_amd64", appName))
	debianDir := filepath.Join(debDir, "DEBIAN")
	binDir := filepath.Join(debDir, "usr", "local", "bin")
	os.MkdirAll(debianDir, 0755)
	os.MkdirAll(binDir, 0755)

	// Copy binary
	copyFile(binaryPath, filepath.Join(binDir, binaryName))

	// Write control file
	controlContent := fmt.Sprintf(`Package: %s
Version: 1.0.0
Architecture: amd64
Maintainer: Luminka Developer <developer@example.com>
Description: %s — built with Luminka
 Built from the %s app using the Luminka framework.
`, appName, appName, appName)

	os.WriteFile(filepath.Join(debianDir, "control"), []byte(controlContent), 0644)

	cwd, _ := os.Getwd()
	debOut := filepath.Join(cwd, fmt.Sprintf("%s_1.0.0_amd64.deb", appName))

	cmd := exec.Command("dpkg-deb", "--build", debDir, debOut)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("dpkg-deb failed: %w", err)
	}

	fmt.Printf("[package] Created: %s\n", filepath.Base(debOut))
	return nil
}
