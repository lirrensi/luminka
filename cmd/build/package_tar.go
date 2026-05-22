// FILE: cmd/build/package_tar.go
// PURPOSE: Produce a Linux .tar.gz archive of a built Luminka app binary + install scripts.
// OWNS: Linux tar.gz packaging subcommand (tar).
// EXPORTS: cmdPackageTar(appDir string) error
// DOCS: docs/installation.md#packaging-subcommands
// DEPENDS_ON: cmd/build/main.go (copyFile, copyDir — all in package main)

package main

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func cmdPackageTar(appDir string) error {
	absDir, err := filepath.Abs(appDir)
	if err != nil {
		return fmt.Errorf("resolving app directory: %w", err)
	}

	binaryName := fmt.Sprintf("luminka-%s", filepath.Base(absDir))

	binaryPath := filepath.Join(absDir, binaryName)
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		cwd, _ := os.Getwd()
		binaryPath = filepath.Join(cwd, binaryName)
		if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
			return fmt.Errorf("built binary %q not found — run 'go run ./cmd/build %s' first", binaryName, appDir)
		}
	}

	tmpDir, err := os.MkdirTemp("", "luminka-pkg-*")
	if err != nil {
		return fmt.Errorf("creating temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	stageName := strings.TrimSuffix(binaryName, ".exe")
	stageDir := filepath.Join(tmpDir, stageName)
	os.MkdirAll(stageDir, 0755)

	copyFile(binaryPath, filepath.Join(stageDir, filepath.Base(binaryPath)))

	scriptsSrc := filepath.Join(absDir, "scripts", "install")
	if info, err := os.Stat(scriptsSrc); err == nil && info.IsDir() {
		copyDir(scriptsSrc, filepath.Join(stageDir, "scripts", "install"))
	}

	cwd, _ := os.Getwd()
	outName := fmt.Sprintf("%s-linux-x64.tar.gz", stageName)
	outPath := filepath.Join(cwd, outName)

	outFile, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("creating tar: %w", err)
	}
	defer outFile.Close()

	gw := gzip.NewWriter(outFile)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	err = filepath.Walk(stageDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, _ := filepath.Rel(stageDir, path)
		if relPath == "." {
			return nil
		}

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(relPath)

		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		if !info.IsDir() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if _, err := tw.Write(data); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("writing tar: %w", err)
	}

	fmt.Printf("[package] Created: %s\n", outName)
	return nil
}
