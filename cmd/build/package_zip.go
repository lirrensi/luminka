// FILE: cmd/build/package_zip.go
// PURPOSE: Produce a Windows .zip archive of a built Luminka app binary + install scripts.
// OWNS: Windows zip packaging subcommand (zip).
// EXPORTS: cmdPackageZip(appDir string) error
// DOCS: docs/installation.md#packaging-subcommands
// DEPENDS_ON: cmd/build/main.go (copyFile, copyDir, resolveBinaryPath — all in package main)

package main

import (
	"archive/zip"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func cmdPackageZip(appDir string) error {
	absDir, err := filepath.Abs(appDir)
	if err != nil {
		return fmt.Errorf("resolving app directory: %w", err)
	}

	binaryName := fmt.Sprintf("luminka-%s", filepath.Base(absDir))
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}

	binaryPath := filepath.Join(absDir, binaryName)
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		// Try CWD
		cwd, _ := os.Getwd()
		binaryPath = filepath.Join(cwd, binaryName)
		if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
			return fmt.Errorf("built binary %q not found — run 'go run ./cmd/build %s' first", binaryName, appDir)
		}
	}

	// Create temp staging directory
	tmpDir, err := os.MkdirTemp("", "luminka-pkg-*")
	if err != nil {
		return fmt.Errorf("creating temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Stage directory structure
	stageName := strings.TrimSuffix(binaryName, ".exe")
	stageDir := filepath.Join(tmpDir, stageName)
	os.MkdirAll(stageDir, 0755)

	// Copy binary
	copyFile(binaryPath, filepath.Join(stageDir, filepath.Base(binaryPath)))

	// Copy install scripts if present
	scriptsSrc := filepath.Join(absDir, "scripts", "install")
	if info, err := os.Stat(scriptsSrc); err == nil && info.IsDir() {
		scriptsDst := filepath.Join(stageDir, "scripts", "install")
		copyDir(scriptsSrc, scriptsDst)
	}

	// Create .zip
	outName := fmt.Sprintf("%s-windows-x64.zip", stageName)
	cwd, _ := os.Getwd()
	outPath := filepath.Join(cwd, outName)

	outFile, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("creating zip: %w", err)
	}
	defer outFile.Close()

	zw := zip.NewWriter(outFile)
	defer zw.Close()

	err = filepath.Walk(stageDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, _ := filepath.Rel(stageDir, path)
		if relPath == "." {
			return nil
		}

		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(relPath)

		if info.IsDir() {
			header.Name += "/"
		} else {
			header.Method = zip.Deflate
		}

		writer, err := zw.CreateHeader(header)
		if err != nil {
			return err
		}

		if !info.IsDir() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			_, err = writer.Write(data)
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("writing zip: %w", err)
	}

	fmt.Printf("[package] Created: %s\n", outName)
	return nil
}
