package llm

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Model definitions matching the original qmd defaults.
type ModelDef struct {
	Name     string
	HFRepo   string // HuggingFace repo (org/repo)
	Filename string // filename within repo
}

var DefaultEmbedModel = ModelDef{
	Name:     "embeddinggemma-300M",
	HFRepo:   "ggml-org/embeddinggemma-300M-GGUF",
	Filename: "embeddinggemma-300M-Q8_0.gguf",
}

var DefaultChatModel = ModelDef{
	Name:     "qmd-query-expansion-1.7B",
	HFRepo:   "tobil/qmd-query-expansion-1.7B-gguf",
	Filename: "qmd-query-expansion-1.7B-q4_k_m.gguf",
}

func (m ModelDef) URL() string {
	return fmt.Sprintf("https://huggingface.co/%s/resolve/main/%s", m.HFRepo, m.Filename)
}

func CacheDir() string {
	if d := os.Getenv("XDG_CACHE_HOME"); d != "" {
		return filepath.Join(d, "qmd")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "qmd")
}

func ModelsDir() string { return filepath.Join(CacheDir(), "models") }
func BinDir() string    { return filepath.Join(CacheDir(), "bin") }

// httpClient follows redirects (important for GitHub API and download URLs).
var httpClient = &http.Client{
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return fmt.Errorf("too many redirects")
		}
		return nil
	},
}

// DownloadFile downloads a URL to a local path with progress reporting.
// Returns immediately if the file already exists and is non-empty.
func DownloadFile(url, dest, description string) error {
	if fi, err := os.Stat(dest); err == nil && fi.Size() > 0 {
		return nil
	}

	os.MkdirAll(filepath.Dir(dest), 0o755)
	fmt.Fprintf(os.Stderr, "Downloading %s...\n", description)

	resp, err := httpClient.Get(url)
	if err != nil {
		return fmt.Errorf("download %s: %w", description, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: HTTP %d", description, resp.StatusCode)
	}

	tmp := dest + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}

	total := resp.ContentLength
	var written int64
	buf := make([]byte, 64*1024)
	lastPct := -1

	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, err := f.Write(buf[:n]); err != nil {
				f.Close()
				os.Remove(tmp)
				return err
			}
			written += int64(n)
			if total > 0 {
				pct := int(written * 100 / total)
				if pct/5 != lastPct/5 {
					fmt.Fprintf(os.Stderr, "  %d%% (%d / %d MB)\n", pct, written>>20, total>>20)
					lastPct = pct
				}
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			f.Close()
			os.Remove(tmp)
			return readErr
		}
	}
	f.Close()

	if total > 0 && written != total {
		os.Remove(tmp)
		return fmt.Errorf("download %s: incomplete (%d of %d bytes)", description, written, total)
	}

	return os.Rename(tmp, dest)
}

// EnsureLlamaServer returns the path to a llama-server binary, downloading if needed.
func EnsureLlamaServer() (string, error) {
	// 1. Env override
	if p := os.Getenv("QMD_LLAMA_SERVER"); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
		return "", fmt.Errorf("QMD_LLAMA_SERVER=%s not found", p)
	}

	// 2. Check PATH
	if p, err := exec.LookPath("llama-server"); err == nil {
		return p, nil
	}

	// 3. Check cache
	bin := filepath.Join(BinDir(), "llama-server")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	if fi, err := os.Stat(bin); err == nil && fi.Size() > 0 {
		return bin, nil
	}

	// 4. Download from GitHub releases
	return downloadLlamaServer(bin)
}

func downloadLlamaServer(dest string) (string, error) {
	fmt.Fprintln(os.Stderr, "Looking up latest llama.cpp release...")

	// The repo may redirect (ggerganov -> ggml-org), so follow redirects
	resp, err := httpClient.Get("https://api.github.com/repos/ggerganov/llama.cpp/releases/latest")
	if err != nil {
		return "", fmt.Errorf("fetching llama.cpp releases: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("GitHub API returned %d: %s", resp.StatusCode, string(body))
	}

	var release struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", fmt.Errorf("parsing release: %w", err)
	}

	pattern := platformAssetPattern()
	var downloadURL, assetName string
	for _, a := range release.Assets {
		if matchesPlatformAsset(a.Name, pattern) {
			downloadURL = a.BrowserDownloadURL
			assetName = a.Name
			break
		}
	}
	if downloadURL == "" {
		return "", fmt.Errorf("no llama-server binary for %s/%s in release %s (looked for %q)",
			runtime.GOOS, runtime.GOARCH, release.TagName, pattern)
	}

	// Download archive
	archivePath := dest + archiveSuffix(assetName)
	if err := DownloadFile(downloadURL, archivePath, fmt.Sprintf("llama-server (%s)", assetName)); err != nil {
		return "", err
	}

	// Extract llama-server and all shared libraries to BinDir
	binDir := BinDir()
	var extractErr error
	if strings.HasSuffix(assetName, ".tar.gz") {
		extractErr = extractAllFromTarGz(archivePath, binDir)
	} else {
		extractErr = extractAllFromZip(archivePath, binDir)
	}
	if extractErr != nil {
		os.Remove(archivePath)
		return "", fmt.Errorf("extracting llama-server: %w", extractErr)
	}
	os.Remove(archivePath)
	os.Chmod(dest, 0o755)

	// Create soname symlinks for shared libraries (e.g. libmtmd.so.0 -> libmtmd.so.0.0.8184)
	createLibSymlinks(binDir)

	fmt.Fprintf(os.Stderr, "llama-server installed at %s\n", dest)
	return dest, nil
}

func archiveSuffix(name string) string {
	if strings.HasSuffix(name, ".tar.gz") {
		return ".tar.gz"
	}
	return ".zip"
}

func platformAssetPattern() string {
	switch {
	case runtime.GOOS == "linux" && runtime.GOARCH == "amd64":
		return "ubuntu-x64"
	case runtime.GOOS == "linux" && runtime.GOARCH == "arm64":
		return "ubuntu-arm64"
	case runtime.GOOS == "darwin" && runtime.GOARCH == "arm64":
		return "macos-arm64"
	case runtime.GOOS == "darwin" && runtime.GOARCH == "amd64":
		return "macos-x64"
	case runtime.GOOS == "windows" && runtime.GOARCH == "amd64":
		return "win-cpu-x64"
	case runtime.GOOS == "windows" && runtime.GOARCH == "arm64":
		return "win-cpu-arm64"
	default:
		return fmt.Sprintf("%s-%s", runtime.GOOS, runtime.GOARCH)
	}
}

func matchesPlatformAsset(name, pattern string) bool {
	// Match names like "llama-b8184-bin-ubuntu-x64.tar.gz"
	// Reject CUDA/ROCm/Vulkan builds - prefer CPU for portability
	return strings.Contains(name, "-bin-") &&
		strings.Contains(name, pattern) &&
		!strings.Contains(name, "cuda") &&
		!strings.Contains(name, "rocm") &&
		!strings.Contains(name, "vulkan") &&
		!strings.Contains(name, "sycl") &&
		!strings.Contains(name, "hip") &&
		!strings.Contains(name, "opencl") &&
		!strings.Contains(name, "aclgraph") &&
		(strings.HasSuffix(name, ".tar.gz") || strings.HasSuffix(name, ".zip"))
}

// shouldExtract returns true if the file is a binary or shared library we want.
func shouldExtract(name string) bool {
	base := filepath.Base(name)
	// Extract executables and shared libraries
	if base == "llama-server" || base == "llama-server.exe" {
		return true
	}
	if strings.HasSuffix(base, ".so") || strings.Contains(base, ".so.") {
		return true
	}
	if strings.HasSuffix(base, ".dylib") {
		return true
	}
	if strings.HasSuffix(base, ".dll") {
		return true
	}
	return false
}

// extractAllFromTarGz extracts llama-server and all shared libraries.
func extractAllFromTarGz(archivePath, destDir string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()

	os.MkdirAll(destDir, 0o755)
	found := false

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		if hdr.Typeflag != tar.TypeReg {
			continue
		}

		if !shouldExtract(hdr.Name) {
			continue
		}

		base := filepath.Base(hdr.Name)
		dest := filepath.Join(destDir, base)
		out, err := os.Create(dest)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, tr); err != nil {
			out.Close()
			return err
		}
		out.Close()
		os.Chmod(dest, 0o755)

		if base == "llama-server" || base == "llama-server.exe" {
			found = true
		}
	}
	if !found {
		return fmt.Errorf("llama-server not found in tar.gz archive")
	}
	return nil
}

// extractAllFromZip extracts llama-server and all shared libraries.
func extractAllFromZip(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	os.MkdirAll(destDir, 0o755)
	found := false

	for _, f := range r.File {
		if !shouldExtract(f.Name) {
			continue
		}

		base := filepath.Base(f.Name)
		dest := filepath.Join(destDir, base)

		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.Create(dest)
		if err != nil {
			rc.Close()
			return err
		}
		if _, err := io.Copy(out, rc); err != nil {
			out.Close()
			rc.Close()
			return err
		}
		out.Close()
		rc.Close()
		os.Chmod(dest, 0o755)

		if base == "llama-server" || base == "llama-server.exe" {
			found = true
		}
	}
	if !found {
		return fmt.Errorf("llama-server not found in zip archive")
	}
	return nil
}

// createLibSymlinks creates soname symlinks for versioned shared libraries.
// e.g., libmtmd.so.0.0.8184 -> libmtmd.so.0 -> libmtmd.so
func createLibSymlinks(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.Contains(name, ".so.") {
			continue
		}
		// For "libfoo.so.X.Y.Z", create "libfoo.so.X" and "libfoo.so"
		// Find the base: everything up to and including ".so"
		soIdx := strings.Index(name, ".so.")
		if soIdx < 0 {
			continue
		}
		baseSo := name[:soIdx+3] // "libfoo.so"

		// Create major version symlink: libfoo.so.X
		rest := name[soIdx+4:] // "X.Y.Z" or "X"
		major := rest
		if dot := strings.Index(rest, "."); dot >= 0 {
			major = rest[:dot]
		}
		majorName := baseSo + "." + major

		// Only create if they don't exist
		majorPath := filepath.Join(dir, majorName)
		if _, err := os.Lstat(majorPath); os.IsNotExist(err) {
			os.Symlink(name, majorPath)
		}

		// Create unversioned symlink: libfoo.so
		basePath := filepath.Join(dir, baseSo)
		if _, err := os.Lstat(basePath); os.IsNotExist(err) {
			os.Symlink(majorName, basePath)
		}
	}
}

// EnsureModel downloads a model GGUF file if not already cached, and returns its local path.
func EnsureModel(model ModelDef) (string, error) {
	dest := filepath.Join(ModelsDir(), model.Filename)
	if err := DownloadFile(model.URL(), dest, model.Name+" model"); err != nil {
		return "", err
	}
	return dest, nil
}

// EnsureEmbedModel returns the path to the embedding model, downloading if needed.
func EnsureEmbedModel() (string, error) {
	if p := os.Getenv("QMD_EMBED_MODEL"); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
		return "", fmt.Errorf("QMD_EMBED_MODEL=%s not found as local file", p)
	}
	return EnsureModel(DefaultEmbedModel)
}

// EnsureChatModel returns the path to the chat model, downloading if needed.
func EnsureChatModel() (string, error) {
	if p := os.Getenv("QMD_CHAT_MODEL"); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
		return "", fmt.Errorf("QMD_CHAT_MODEL=%s not found as local file", p)
	}
	return EnsureModel(DefaultChatModel)
}
