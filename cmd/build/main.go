// FILE: cmd/build/main.go
// PURPOSE: Standalone, zero-dependency-on-luminka build CLI.
// OWNS: Icon generation, GCC resolution, go-winres, and go build orchestration.
// EXPORTS: main
//
// Usage:
//
//	go run ./cmd/build ./starter                  # browser build
//	go run ./cmd/build ./starter --webview          # webview build
//	go run ./cmd/build . --webview --gcc /opt/gcc  # custom gcc path
//	go run ./cmd/build ./starter --tags scripts,shell
//	go run ./cmd/build ./starter --icon assets/lumi.png --out myapp.exe
//
// Imported module:
//
//	go run github.com/lirrensi/luminka/cmd/build@latest . --webview

package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// ---- flags ----

type icnsEntry struct {
	size   int
	osType string
}

var (
	webview  = flag.Bool("webview", false, "Build with -tags webview (enables CGO and native window)")
	tags     = flag.String("tags", "", "Additional build tags, comma-separated (e.g. scripts,shell)")
	gccPath  = flag.String("gcc", "", "Path to gcc executable (overrides auto-detection)")
	iconSrc  = flag.String("icon", "assets/lumi.png", "Source PNG for icon generation (empty to skip)")
	outName  = flag.String("out", "", "Custom output binary name (default: luminka-<app>.exe)")
)

func main() {
	flag.Usage = usage

	// Reorder args so flags come before positional — Go's flag package
	// stops at the first non-flag arg, so "./starter -webview" fails.
	// This lets users write flags in any order.
	reorderArgs()

	flag.Parse()

	appDir := "."
	if flag.NArg() > 0 {
		appDir = flag.Arg(0)
	}

	// Resolve absolute paths early
	cwd, err := os.Getwd()
	if err != nil {
		fatal("Failed to get working directory: %v", err)
	}
	appDir, err = filepath.Abs(appDir)
	if err != nil {
		fatal("Failed to resolve app directory: %v", err)
	}
	appName := filepath.Base(appDir)

	// ---- 1. Validate app directory ----
	if _, err := os.Stat(filepath.Join(appDir, "main.go")); os.IsNotExist(err) {
		fatal("main.go not found in %s", appDir)
	}
	distDir := filepath.Join(appDir, "dist")
	if s, err := os.Stat(distDir); os.IsNotExist(err) || !s.IsDir() {
		fatal("dist/ directory not found in %s", appDir)
	}
	if empty, _ := isDirEmpty(distDir); empty {
		fatal("dist/ directory is empty in %s — build your frontend assets first", appDir)
	}

	// ---- 2. Resolve GCC (webview only) ----
	if *webview {
		gcc, err := resolveGCC(*gccPath)
		if err != nil {
			fatal("GCC not found: %v\nUse --gcc <path> to specify manually, or install MinGW/MSYS2/TDM-GCC", err)
		}
		os.Setenv("CC", gcc)
		os.Setenv("CGO_ENABLED", "1")
		fmt.Printf("[tools] gcc: %s\n", gcc)
	}

	// ---- 3. Build icons (optional) ----
	if *iconSrc != "" {
		iconPath := filepath.Join(cwd, *iconSrc)
		if _, err := os.Stat(iconPath); err == nil {
			fmt.Printf("[icons] Building from %s...\n", *iconSrc)
			if err := buildIcons(iconPath, appDir); err != nil {
				fatal("Icon build failed: %v", err)
			}
			fmt.Println("[icons] Done.")
		}
	}

	// ---- 4. go-winres (optional) ----
	winresJSON := filepath.Join(appDir, "winres", "winres.json")
	if _, err := os.Stat(winresJSON); err == nil {
		fmt.Println("[winres] Embedding Windows resources...")
		cmd := exec.Command("go", "run", "github.com/tc-hib/go-winres@latest", "make", "--in", winresJSON)
		cmd.Dir = appDir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fatal("go-winres failed: %v", err)
		}
	}

	// ---- 5. go build ----
	buildTags := []string{}
	if *webview {
		buildTags = append(buildTags, "webview")
	}
	if *tags != "" {
		for _, t := range strings.Split(*tags, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				buildTags = append(buildTags, t)
			}
		}
	}

	output := *outName
	if output == "" {
		output = fmt.Sprintf("luminka-%s", appName)
		if *webview {
			output += "-webview"
		}
		if runtime.GOOS == "windows" {
			output += ".exe"
		}
	}
	// Output relative to CWD (where user invoked the command)
	output = filepath.Join(cwd, output)

	fmt.Printf("[build] go build %s (%s mode)...\n", appName, modeLabel(*webview))
	buildArgs := []string{"build"}
	if runtime.GOOS == "windows" {
		buildArgs = append(buildArgs, "-ldflags", "-H windowsgui")
	}
	if len(buildTags) > 0 {
		buildArgs = append(buildArgs, "-tags", strings.Join(buildTags, ","))
	}
	buildArgs = append(buildArgs, "-o", output, ".")
	// Run from appDir so relative embed paths resolve correctly
	cmd := exec.Command("go", buildArgs...)
	cmd.Dir = appDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	// Inherit environment, plus any we've set (CC, CGO_ENABLED)
	cmd.Env = os.Environ()
	if err := cmd.Run(); err != nil {
		fatal("go build failed: %v", err)
	}

	rel, _ := filepath.Rel(cwd, output)
	if rel == "" {
		rel = output
	}
	fmt.Printf("[build] Success: %s\n", rel)
}

// ---- icon building ----

func buildIcons(sourcePNG, appDir string) error {
	src, err := loadPNG(sourcePNG)
	if err != nil {
		return fmt.Errorf("loading source: %w", err)
	}

	windowsDir := filepath.Join(appDir, "build", "icons", "windows")
	macosDir := filepath.Join(appDir, "build", "icons", "macos")
	linuxDir := filepath.Join(appDir, "build", "icons", "linux")
	winresDir := filepath.Join(appDir, "winres")

	for _, d := range []string{windowsDir, macosDir, linuxDir, winresDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			return err
		}
	}

	windowsSizes := []int{16, 32, 48, 64, 128, 256, 512}
	linuxSizes := []int{128, 256, 512}
	icnsSizes := []icnsEntry{
		{16, "icp4"}, {32, "icp5"}, {64, "icp6"},
		{128, "ic07"}, {256, "ic08"}, {512, "ic09"},
	}

	var windowsPNGs []string
	var icon256Path, icon16Path string

	// Windows: all sizes PNG + ICO
	for _, size := range windowsSizes {
		path := filepath.Join(windowsDir, fmt.Sprintf("%dx%d.png", size, size))
		if err := writeResizedPNG(src, path, size); err != nil {
			return err
		}
		windowsPNGs = append(windowsPNGs, path)
		if size == 256 {
			icon256Path = path
		}
		if size == 16 {
			icon16Path = path
		}
	}

	// Linux: selected sizes PNG
	for _, size := range linuxSizes {
		path := filepath.Join(linuxDir, fmt.Sprintf("%dx%d.png", size, size))
		if err := writeResizedPNG(src, path, size); err != nil {
			return err
		}
	}

	// winres copies
	if icon256Path != "" {
		copyFile(icon256Path, filepath.Join(winresDir, "icon.png"))
	}
	if icon16Path != "" {
		copyFile(icon16Path, filepath.Join(winresDir, "icon16.png"))
	}

	// ICO from Windows PNGs
	if err := buildICO(windowsPNGs, filepath.Join(windowsDir, "icon.ico")); err != nil {
		return fmt.Errorf("ico: %w", err)
	}

	// ICNS
	if err := buildICNS(src, icnsSizes, filepath.Join(macosDir, "icon.icns")); err != nil {
		return fmt.Errorf("icns: %w", err)
	}

	return nil
}

func loadPNG(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return png.Decode(f)
}

func writeResizedPNG(src image.Image, path string, size int) error {
	dst := bilinearResize(src, size, size)
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, dst)
}

// bilinearResize resizes an image using bilinear interpolation (stdlib only, zero dependencies).
func bilinearResize(src image.Image, dstW, dstH int) *image.RGBA {
	srcB := src.Bounds()
	srcW, srcH := srcB.Dx(), srcB.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))

	for dy := 0; dy < dstH; dy++ {
		for dx := 0; dx < dstW; dx++ {
			// Map destination pixel to source coordinates
			sx := (float64(dx) + 0.5) * float64(srcW)/float64(dstW) - 0.5
			sy := (float64(dy) + 0.5) * float64(srcH)/float64(dstH) - 0.5

			// Clamp
			if sx < 0 { sx = 0 }
			if sy < 0 { sy = 0 }
			if sx >= float64(srcW)-1 { sx = float64(srcW) - 1.001 }
			if sy >= float64(srcH)-1 { sy = float64(srcH) - 1.001 }

			x0, y0 := int(sx), int(sy)
			x1, y1 := x0+1, y0+1
			if x1 >= srcW { x1 = srcW - 1 }
			if y1 >= srcH { y1 = srcH - 1 }

			fx := sx - float64(x0)
			fy := sy - float64(y0)

			r00, g00, b00, a00 := src.At(x0, y0).RGBA()
			r10, g10, b10, a10 := src.At(x1, y0).RGBA()
			r01, g01, b01, a01 := src.At(x0, y1).RGBA()
			r11, g11, b11, a11 := src.At(x1, y1).RGBA()

			// Convert back from range 0..65535 to 0..255
			r := uint8(roundClamp(lerp4(float64(r00), float64(r10), float64(r01), float64(r11), fx, fy)) >> 8)
			g := uint8(roundClamp(lerp4(float64(g00), float64(g10), float64(g01), float64(g11), fx, fy)) >> 8)
			b := uint8(roundClamp(lerp4(float64(b00), float64(b10), float64(b01), float64(b11), fx, fy)) >> 8)
			a := uint8(roundClamp(lerp4(float64(a00), float64(a10), float64(a01), float64(a11), fx, fy)) >> 8)

			dst.SetRGBA(dx, dy, color.RGBA{r, g, b, a})
		}
	}
	return dst
}

func lerp4(v00, v10, v01, v11 float64, fx, fy float64) float64 {
	top := v00*(1-fx) + v10*fx
	bot := v01*(1-fx) + v11*fx
	return top*(1-fy) + bot*fy
}

func roundClamp(v float64) uint32 {
	if v < 0 {
		return 0
	}
	if v > 65535 {
		return 65535
	}
	return uint32(v + 0.5)
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}

func buildICO(pngPaths []string, outPath string) error {
	var images []image.Image
	for _, p := range pngPaths {
		img, err := loadPNG(p)
		if err != nil {
			return err
		}
		images = append(images, img)
	}

	var dataBuf bytes.Buffer
	type entry struct {
		width, height byte
		size          uint32
		offset        uint32
	}
	var entries []entry
	headerSize := 6 + len(images)*16

	for _, img := range images {
		var pngBuf bytes.Buffer
		if err := png.Encode(&pngBuf, img); err != nil {
			return err
		}
		b := pngBuf.Bytes()
		offset := uint32(headerSize + dataBuf.Len())
		w, h := img.Bounds().Dx(), img.Bounds().Dy()
		bw, bh := byte(w), byte(h)
		if w >= 256 {
			bw = 0
		}
		if h >= 256 {
			bh = 0
		}
		entries = append(entries, entry{bw, bh, uint32(len(b)), offset})
		dataBuf.Write(b)
	}

	var out bytes.Buffer
	// ICO header
	binary.Write(&out, binary.LittleEndian, uint16(0))  // reserved
	binary.Write(&out, binary.LittleEndian, uint16(1))  // type: ICO
	binary.Write(&out, binary.LittleEndian, uint16(len(images))) // count
	for _, e := range entries {
		out.WriteByte(e.width)
		out.WriteByte(e.height)
		out.WriteByte(0) // color palette
		out.WriteByte(0) // reserved
		binary.Write(&out, binary.LittleEndian, uint16(1))   // planes
		binary.Write(&out, binary.LittleEndian, uint16(32))  // bpp
		binary.Write(&out, binary.LittleEndian, e.size)
		binary.Write(&out, binary.LittleEndian, e.offset)
	}
	out.Write(dataBuf.Bytes())

	return os.WriteFile(outPath, out.Bytes(), 0644)
}

func buildICNS(src image.Image, sizes []icnsEntry, outPath string) error {
	var data bytes.Buffer
	for _, s := range sizes {
		img := bilinearResize(src, s.size, s.size)
		var pngBuf bytes.Buffer
		if err := png.Encode(&pngBuf, img); err != nil {
			return err
		}
		b := pngBuf.Bytes()
		length := uint32(len(b) + 8)
		data.WriteString(s.osType)
		binary.Write(&data, binary.BigEndian, length)
		data.Write(b)
	}
	totalLen := uint32(data.Len() + 8)
	var out bytes.Buffer
	out.WriteString("icns")
	binary.Write(&out, binary.BigEndian, totalLen)
	out.Write(data.Bytes())
	return os.WriteFile(outPath, out.Bytes(), 0644)
}

// ---- GCC resolution ----

func resolveGCC(explicitPath string) (string, error) {
	if explicitPath != "" {
		if _, err := os.Stat(explicitPath); err != nil {
			return "", fmt.Errorf("--gcc path not found: %s", explicitPath)
		}
		return explicitPath, nil
	}

	// 1. PATH
	if p, err := exec.LookPath("gcc"); err == nil {
		return p, nil
	}

	// 2. Platform-specific search
	if runtime.GOOS == "windows" {
		return findGCCWindows()
	}

	return "", errors.New("gcc not on PATH — install GCC or use --gcc <path>")
}

func findGCCWindows() (string, error) {
	// Exhaustive hardcoded paths — the CEO said 10 minutes of scanning is fine.
	paths := []string{
		// MSYS2 variants
		`C:\msys64\mingw64\bin\gcc.exe`,
		`C:\msys64\mingw32\bin\gcc.exe`,
		`C:\msys64\ucrt64\bin\gcc.exe`,
		`C:\msys64\clang64\bin\gcc.exe`,
		`C:\msys32\mingw32\bin\gcc.exe`,
		// Standalone Mingw-w64
		`C:\mingw64\bin\gcc.exe`,
		`C:\mingw32\bin\gcc.exe`,
		// TDM-GCC
		`C:\TDM-GCC-64\bin\gcc.exe`,
		`C:\TDM-GCC-32\bin\gcc.exe`,
		// Chocolatey / Scoop
		filepath.Join(os.Getenv("ProgramData"), `chocolatey\bin\gcc.exe`),
		filepath.Join(os.Getenv("USERPROFILE"), `scoop\apps\mingw\current\bin\gcc.exe`),
		filepath.Join(os.Getenv("USERPROFILE"), `scoop\apps\gcc\current\bin\gcc.exe`),
		// Cygwin
		`C:\cygwin64\bin\gcc.exe`,
		`C:\cygwin\bin\gcc.exe`,
		// Visual Studio (clang-cl with gcc frontend — rare but possible)
		filepath.Join(os.Getenv("USERPROFILE"), `.espressif\tools\xtensa-esp32-elf\esp-2021r2-patch5-8.4.0\xtensa-esp32-elf\bin\xtensa-esp32-elf-gcc.exe`),
	}

	for _, p := range paths {
		if p == "" {
			continue
		}
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	// 3. Wildcard scan for mingw-w64 installations in Program Files
	programFiles := []string{
		os.Getenv("ProgramFiles"),
		os.Getenv("ProgramFiles(x86)"),
	}
	for _, pf := range programFiles {
		if pf == "" {
			continue
		}
		matches, _ := filepath.Glob(filepath.Join(pf, "mingw-w64", "*", "mingw64", "bin", "gcc.exe"))
		for _, m := range matches {
			return m, nil
		}
		matches, _ = filepath.Glob(filepath.Join(pf, "mingw-w64", "*", "mingw32", "bin", "gcc.exe"))
		for _, m := range matches {
			return m, nil
		}
	}

	// 4. Recursive scan of likely tool directories (last resort)
	scanRoots := []string{
		`C:\msys64`,
		`C:\msys32`,
		`C:\mingw64`,
		`C:\mingw32`,
		`C:\TDM-GCC-64`,
		`C:\TDM-GCC-32`,
		`C:\cygwin64`,
		`C:\cygwin`,
	}
	for _, root := range scanRoots {
		if found := recursiveFindGCC(root); found != "" {
			return found, nil
		}
	}

	return "", errors.New("gcc not found after exhaustive scan — install MinGW-w64, MSYS2, or TDM-GCC, or use --gcc <path>")
}

func recursiveFindGCC(root string) string {
	var found string
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || found != "" {
			return nil
		}
		if !info.IsDir() && strings.EqualFold(info.Name(), "gcc.exe") {
			found = path
			return filepath.SkipAll
		}
		// Don't recurse into obviously huge unrelated directories
		if info.IsDir() && strings.HasPrefix(info.Name(), ".") {
			return filepath.SkipDir
		}
		return nil
	})
	return found
}

// ---- helpers ----

func isDirEmpty(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return true, err
	}
	return len(entries) == 0, nil
}

func modeLabel(webview bool) string {
	if webview {
		return "webview"
	}
	return "browser"
}

func fatal(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "ERROR: "+format+"\n", args...)
	os.Exit(1)
}

// reorderArgs moves known flags to the front of os.Args so that
// Go's flag package (which stops at the first non-flag arg) parses
// them regardless of position. "./starter --webview" and
// "--webview ./starter" both work.
func reorderArgs() {
	known := map[string]bool{
		"-webview": true, "--webview": true,
		"-gcc": true, "--gcc": true,
		"-tags": true, "--tags": true,
		"-icon": true, "--icon": true,
		"-out": true, "--out": true,
		"-h": true, "--help": true,
	}
	var flags, positional []string
	flags = append(flags, os.Args[0]) // program name stays first

	skipNext := false
	for i := 1; i < len(os.Args); i++ {
		a := os.Args[i]
		if skipNext {
			flags = append(flags, a)
			skipNext = false
			continue
		}
		if known[a] {
			flags = append(flags, a)
			// For flags that take a value, grab the next arg too
			if a == "-gcc" || a == "--gcc" || a == "-tags" || a == "--tags" ||
				a == "-icon" || a == "--icon" || a == "-out" || a == "--out" {
				if i+1 < len(os.Args) && !known[os.Args[i+1]] {
					flags = append(flags, os.Args[i+1])
					i++ // consumed the value
				}
			}
		} else if strings.HasPrefix(a, "-") {
			// Unknown flag — pass through (flag package will error)
			flags = append(flags, a)
		} else {
			positional = append(positional, a)
		}
	}
	flags = append(flags, positional...)
	os.Args = flags
}

func usage() {
	out := flag.CommandLine.Output()
	fmt.Fprintf(out, "Usage: go run ./cmd/build [flags] [app-directory]\n\n")
	fmt.Fprintf(out, "  app-directory    Application directory to build (default: current directory)\n\n")
	fmt.Fprintf(out, "Flags:\n")
	flag.PrintDefaults()
	fmt.Fprintf(out, "\nExamples:\n")
	fmt.Fprintf(out, "  go run ./cmd/build ./starter               # browser build\n")
	fmt.Fprintf(out, "  go run ./cmd/build ./starter --webview       # webview build\n")
	fmt.Fprintf(out, "  go run ./cmd/build . --webview --gcc C:\\msys64\\mingw64\\bin\\gcc.exe\n")
	fmt.Fprintf(out, "  go run ./cmd/build ./starter --tags scripts,shell\n")
	fmt.Fprintf(out, "  go run ./cmd/build . --icon assets/lumi.png --out myapp.exe\n")
	fmt.Fprintf(out, "\nRemote (imported module):\n")
	fmt.Fprintf(out, "  go run github.com/lirrensi/luminka/cmd/build@latest . --webview\n")
}
