//go:build fts5

package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var binaryPath string

func TestMain(m *testing.M) {
	// Build the binary once before all tests
	tmp, err := os.MkdirTemp("", "qqmd-test-bin-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "creating temp dir: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmp)

	binaryPath = filepath.Join(tmp, "qqmd")
	cmd := exec.Command("go", "build", "-tags", "fts5", "-o", binaryPath, "..")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "building binary: %v\n", err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

// run executes the qqmd binary with the given args and environment.
// It returns stdout and any error.
func run(t *testing.T, configDir, cacheDir string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(binaryPath, args...)
	cmd.Env = append(os.Environ(),
		"QQMD_CONFIG_DIR="+configDir,
		"XDG_CACHE_HOME="+cacheDir,
	)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// setupEnv creates temp dirs for config and cache.
func setupEnv(t *testing.T) (configDir, cacheDir string) {
	t.Helper()
	configDir = t.TempDir()
	cacheDir = t.TempDir()
	return
}

// createMdDir creates a temp directory with some markdown files.
func createMdDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, name)
		os.MkdirAll(filepath.Dir(path), 0o755)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// --- 5: CMD integration tests ---

func TestCollectionAddListRemove(t *testing.T) {
	t.Parallel()
	cfg, cache := setupEnv(t)
	dir := createMdDir(t, map[string]string{
		"readme.md": "# Readme\n\nHello world.",
	})

	// Add
	out, err := run(t, cfg, cache, "collection", "add", "--name", "test", dir)
	if err != nil {
		t.Fatalf("add: %v\n%s", err, out)
	}

	// List
	out, err = run(t, cfg, cache, "collection", "ls", "--json")
	if err != nil {
		t.Fatalf("list: %v\n%s", err, out)
	}
	if !strings.Contains(out, "test") {
		t.Errorf("collection 'test' not in list output:\n%s", out)
	}

	// Remove
	out, err = run(t, cfg, cache, "collection", "rm", "test")
	if err != nil {
		t.Fatalf("remove: %v\n%s", err, out)
	}

	// Verify gone
	out, _ = run(t, cfg, cache, "collection", "ls", "--json")
	if strings.Contains(out, `"name":"test"`) || strings.Contains(out, `"name": "test"`) {
		t.Error("collection should be gone after remove")
	}
}

func TestCollectionRename(t *testing.T) {
	t.Parallel()
	cfg, cache := setupEnv(t)
	dir := createMdDir(t, map[string]string{"a.md": "# A"})

	run(t, cfg, cache, "collection", "add", "--name", "old", dir)
	out, err := run(t, cfg, cache, "collection", "rename", "old", "new")
	if err != nil {
		t.Fatalf("rename: %v\n%s", err, out)
	}

	out, _ = run(t, cfg, cache, "collection", "ls", "--json")
	if strings.Contains(out, `"old"`) {
		t.Error("old name should be gone")
	}
	if !strings.Contains(out, "new") {
		t.Error("new name should be present")
	}
}

func TestCollectionSet(t *testing.T) {
	t.Parallel()
	cfg, cache := setupEnv(t)
	dir := createMdDir(t, map[string]string{"a.md": "# A"})

	run(t, cfg, cache, "collection", "add", "--name", "test", dir)

	// Set update command
	out, err := run(t, cfg, cache, "collection", "set", "test", "--update-cmd", "git pull")
	if err != nil {
		t.Fatalf("set: %v\n%s", err, out)
	}

	// Set include
	out, err = run(t, cfg, cache, "collection", "set", "test", "--include=false")
	if err != nil {
		t.Fatalf("set include: %v\n%s", err, out)
	}

	// Verify via show
	out, err = run(t, cfg, cache, "collection", "show", "test")
	if err != nil {
		t.Fatalf("show: %v\n%s", err, out)
	}
	if !strings.Contains(out, "git pull") {
		t.Error("update-cmd should be set")
	}
}

func TestCollectionShow(t *testing.T) {
	t.Parallel()
	cfg, cache := setupEnv(t)
	dir := createMdDir(t, map[string]string{"a.md": "# A"})

	run(t, cfg, cache, "collection", "add", "--name", "mytest", dir)
	out, err := run(t, cfg, cache, "collection", "show", "mytest")
	if err != nil {
		t.Fatalf("show: %v\n%s", err, out)
	}

	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(out), &obj); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if _, ok := obj["Name"]; !ok {
		t.Error("missing Name field")
	}
}

func TestUpdateReindexes(t *testing.T) {
	t.Parallel()
	cfg, cache := setupEnv(t)
	dir := createMdDir(t, map[string]string{
		"first.md": "# First\n\nInitial content.",
	})

	run(t, cfg, cache, "collection", "add", "--name", "test", dir)

	// Add a new file
	os.WriteFile(filepath.Join(dir, "second.md"), []byte("# Second\n\nNew file."), 0o644)

	out, err := run(t, cfg, cache, "update")
	if err != nil {
		t.Fatalf("update: %v\n%s", err, out)
	}
	if !strings.Contains(out, "1 added") {
		t.Errorf("expected '1 added' in output:\n%s", out)
	}
}

func TestSearchAfterIndex(t *testing.T) {
	t.Parallel()
	cfg, cache := setupEnv(t)
	dir := createMdDir(t, map[string]string{
		"guide.md": "# Installation Guide\n\nHow to install the software step by step.",
		"faq.md":   "# FAQ\n\nFrequently asked questions about configuration.",
	})

	run(t, cfg, cache, "collection", "add", "--name", "docs", dir)
	out, err := run(t, cfg, cache, "search", "installation", "--json")
	if err != nil {
		t.Fatalf("search: %v\n%s", err, out)
	}

	var results []map[string]interface{}
	if err := json.Unmarshal([]byte(out), &results); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if len(results) == 0 {
		t.Error("expected search results")
	}
}

func TestGetByVirtualPath(t *testing.T) {
	t.Parallel()
	cfg, cache := setupEnv(t)
	dir := createMdDir(t, map[string]string{
		"readme.md": "# Readme\n\nProject readme content.",
	})

	run(t, cfg, cache, "collection", "add", "--name", "proj", dir)
	out, err := run(t, cfg, cache, "get", "qqmd://proj/readme.md", "--json")
	if err != nil {
		t.Fatalf("get: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Readme") {
		t.Errorf("expected 'Readme' in output:\n%s", out)
	}
}

func TestGetByDocid(t *testing.T) {
	t.Parallel()
	cfg, cache := setupEnv(t)
	dir := createMdDir(t, map[string]string{
		"test.md": "# Test Document\n\nSome body content.",
	})

	run(t, cfg, cache, "collection", "add", "--name", "test", dir)

	// Get file list to find docid
	out, _ := run(t, cfg, cache, "ls", "--json")
	var files []map[string]interface{}
	json.Unmarshal([]byte(out), &files)
	if len(files) == 0 {
		t.Fatal("no files listed")
	}
	docid := files[0]["docid"].(string)

	out, err := run(t, cfg, cache, "get", docid, "--json")
	if err != nil {
		t.Fatalf("get by docid: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Test Document") {
		t.Error("expected doc content")
	}
}

func TestGetLineRange(t *testing.T) {
	t.Parallel()
	cfg, cache := setupEnv(t)
	dir := createMdDir(t, map[string]string{
		"lines.md": "Line 1\nLine 2\nLine 3\nLine 4\nLine 5\nLine 6",
	})

	run(t, cfg, cache, "collection", "add", "--name", "test", dir)
	out, err := run(t, cfg, cache, "get", "qqmd://test/lines.md", "--from", "3", "-l", "2", "--json")
	if err != nil {
		t.Fatalf("get: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Line 3") {
		t.Error("expected Line 3")
	}
	if strings.Contains(out, "Line 1") {
		t.Error("should not contain Line 1")
	}
}

func TestMultiGet(t *testing.T) {
	t.Parallel()
	cfg, cache := setupEnv(t)
	dir := createMdDir(t, map[string]string{
		"a.md": "# Alpha\n\nAlpha content.",
		"b.md": "# Beta\n\nBeta content.",
		"c.md": "# Gamma\n\nGamma content.",
	})

	run(t, cfg, cache, "collection", "add", "--name", "test", dir)
	out, err := run(t, cfg, cache, "multi-get", "**/*.md", "--json")
	if err != nil {
		t.Fatalf("multi-get: %v\n%s", err, out)
	}

	var docs []map[string]interface{}
	if err := json.Unmarshal([]byte(out), &docs); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if len(docs) < 2 {
		t.Errorf("expected multiple docs, got %d", len(docs))
	}
}

func TestLs(t *testing.T) {
	t.Parallel()
	cfg, cache := setupEnv(t)
	dir := createMdDir(t, map[string]string{
		"a.md": "# A\n\nContent.",
		"b.md": "# B\n\nContent.",
	})

	run(t, cfg, cache, "collection", "add", "--name", "test", dir)
	out, err := run(t, cfg, cache, "ls", "--json")
	if err != nil {
		t.Fatalf("ls: %v\n%s", err, out)
	}

	var files []map[string]interface{}
	if err := json.Unmarshal([]byte(out), &files); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if len(files) != 2 {
		t.Errorf("expected 2 files, got %d", len(files))
	}
}

func TestContextAddListRemove(t *testing.T) {
	t.Parallel()
	cfg, cache := setupEnv(t)
	dir := createMdDir(t, map[string]string{"a.md": "# A"})

	run(t, cfg, cache, "collection", "add", "--name", "test", dir)

	// Add context
	out, err := run(t, cfg, cache, "context", "add", "test/docs", "Documentation section")
	if err != nil {
		t.Fatalf("context add: %v\n%s", err, out)
	}

	// List
	out, err = run(t, cfg, cache, "context", "ls", "--json")
	if err != nil {
		t.Fatalf("context list: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Documentation section") {
		t.Errorf("context not found in list:\n%s", out)
	}

	// Remove
	out, err = run(t, cfg, cache, "context", "rm", "test", "/docs")
	if err != nil {
		t.Fatalf("context rm: %v\n%s", err, out)
	}

	// Verify gone
	out, _ = run(t, cfg, cache, "context", "ls", "--json")
	if strings.Contains(out, "Documentation section") {
		t.Error("context should be gone after remove")
	}
}

func TestCleanup(t *testing.T) {
	t.Parallel()
	cfg, cache := setupEnv(t)
	dir := createMdDir(t, map[string]string{"a.md": "# A\n\nContent."})

	run(t, cfg, cache, "collection", "add", "--name", "test", dir)

	// Remove file, re-index to deactivate
	os.Remove(filepath.Join(dir, "a.md"))
	run(t, cfg, cache, "update")

	out, err := run(t, cfg, cache, "cleanup")
	if err != nil {
		t.Fatalf("cleanup: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Cleanup complete") {
		t.Errorf("unexpected output:\n%s", out)
	}
}

func TestStatus(t *testing.T) {
	t.Parallel()
	cfg, cache := setupEnv(t)
	dir := createMdDir(t, map[string]string{"a.md": "# A\n\nContent."})

	run(t, cfg, cache, "collection", "add", "--name", "test", dir)
	out, err := run(t, cfg, cache, "status", "--json")
	if err != nil {
		t.Fatalf("status: %v\n%s", err, out)
	}

	var status map[string]interface{}
	if err := json.Unmarshal([]byte(out), &status); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	for _, key := range []string{"DBPath", "TotalDocuments", "Collections"} {
		if _, ok := status[key]; !ok {
			t.Errorf("missing key %q in status", key)
		}
	}
}

func TestOutputFormats(t *testing.T) {
	t.Parallel()
	cfg, cache := setupEnv(t)
	dir := createMdDir(t, map[string]string{
		"doc.md": "# Doc About Search\n\nSearch related content for testing all output formats.",
	})

	run(t, cfg, cache, "collection", "add", "--name", "test", dir)

	formats := []struct {
		flag   string
		check  string
	}{
		{"--json", "["},
		{"--csv", "docid"},
		{"--md", "#"},
		{"--xml", "<file"},
		{"--files", "qqmd://"},
	}

	for _, f := range formats {
		t.Run(f.flag, func(t *testing.T) {
			out, err := run(t, cfg, cache, "search", "search", f.flag)
			if err != nil {
				t.Fatalf("search %s: %v\n%s", f.flag, err, out)
			}
			if !strings.Contains(out, f.check) {
				t.Errorf("%s output missing %q:\n%s", f.flag, f.check, out)
			}
		})
	}
}

// --- Tests ported from qmd CLI integration suite ---

func TestSearchNoResults(t *testing.T) {
	t.Parallel()
	cfg, cache := setupEnv(t)
	dir := createMdDir(t, map[string]string{
		"doc.md": "# About Cats\n\nCats are great animals.",
	})

	run(t, cfg, cache, "collection", "add", "--name", "test", dir)
	out, err := run(t, cfg, cache, "search", "xyznonexistent123", "--json")
	if err != nil {
		t.Fatalf("search: %v\n%s", err, out)
	}

	var results []map[string]interface{}
	if err := json.Unmarshal([]byte(out), &results); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestSearchAllFlag(t *testing.T) {
	t.Parallel()
	cfg, cache := setupEnv(t)
	files := make(map[string]string)
	for i := 0; i < 20; i++ {
		name := fmt.Sprintf("doc%d.md", i)
		files[name] = fmt.Sprintf("# Doc %d\n\nSearchable content here.", i)
	}
	dir := createMdDir(t, files)

	run(t, cfg, cache, "collection", "add", "--name", "test", dir)
	out, err := run(t, cfg, cache, "search", "searchable content", "--all", "--json")
	if err != nil {
		t.Fatalf("search --all: %v\n%s", err, out)
	}

	var results []map[string]interface{}
	if err := json.Unmarshal([]byte(out), &results); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	// --all should return more than the default limit of 5
	if len(results) <= 5 {
		t.Errorf("expected more than 5 results with --all, got %d", len(results))
	}
}

func TestSearchFilesFormat(t *testing.T) {
	t.Parallel()
	cfg, cache := setupEnv(t)
	dir := createMdDir(t, map[string]string{
		"guide.md": "# Installation Guide\n\nHow to install the software.",
	})

	run(t, cfg, cache, "collection", "add", "--name", "docs", dir)
	out, err := run(t, cfg, cache, "search", "installation", "--files")
	if err != nil {
		t.Fatalf("search --files: %v\n%s", err, out)
	}
	if !strings.Contains(out, "qqmd://docs/") {
		t.Errorf("--files output should contain qqmd:// paths:\n%s", out)
	}
}

func TestSearchCollectionFilter(t *testing.T) {
	t.Parallel()
	cfg, cache := setupEnv(t)
	dir1 := createMdDir(t, map[string]string{
		"a.md": "# Alpha Doc\n\nSearchable alpha content.",
	})
	dir2 := createMdDir(t, map[string]string{
		"b.md": "# Beta Doc\n\nSearchable beta content.",
	})

	run(t, cfg, cache, "collection", "add", "--name", "alpha", dir1)
	run(t, cfg, cache, "collection", "add", "--name", "beta", dir2)

	out, err := run(t, cfg, cache, "search", "searchable", "-c", "alpha", "--json")
	if err != nil {
		t.Fatalf("search -c: %v\n%s", err, out)
	}

	var results []map[string]interface{}
	if err := json.Unmarshal([]byte(out), &results); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	for _, r := range results {
		file := r["file"].(string)
		if !strings.HasPrefix(file, "qqmd://alpha/") {
			t.Errorf("expected only alpha results, got %q", file)
		}
	}
}

func TestSearchJsonIncludesDocidAndPath(t *testing.T) {
	t.Parallel()
	cfg, cache := setupEnv(t)
	dir := createMdDir(t, map[string]string{
		"doc.md": "# Test Doc\n\nSome searchable test content.",
	})

	run(t, cfg, cache, "collection", "add", "--name", "mytest", dir)
	out, err := run(t, cfg, cache, "search", "searchable", "--json", "-n", "1")
	if err != nil {
		t.Fatalf("search: %v\n%s", err, out)
	}

	var results []map[string]interface{}
	if err := json.Unmarshal([]byte(out), &results); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if len(results) == 0 {
		t.Fatal("expected at least 1 result")
	}

	r := results[0]
	// Should have qqmd:// path
	file := r["file"].(string)
	if !strings.HasPrefix(file, "qqmd://mytest/") {
		t.Errorf("file = %q, want qqmd://mytest/ prefix", file)
	}
	// Should have docid
	docid := r["docid"].(string)
	if !strings.HasPrefix(docid, "#") {
		t.Errorf("docid = %q, want # prefix", docid)
	}
	// Should not contain filesystem paths
	if strings.Contains(file, "/Users/") || strings.Contains(file, "/tmp/") {
		t.Errorf("file should not contain filesystem paths: %q", file)
	}
}

func TestUpdateDetectsNewAndModified(t *testing.T) {
	t.Parallel()
	cfg, cache := setupEnv(t)
	dir := createMdDir(t, map[string]string{
		"first.md": "# First\n\nInitial content.",
	})

	run(t, cfg, cache, "collection", "add", "--name", "test", dir)

	// Add a new file and modify existing
	os.WriteFile(filepath.Join(dir, "second.md"), []byte("# Second\n\nNew file."), 0o644)
	os.WriteFile(filepath.Join(dir, "first.md"), []byte("# First\n\nModified content."), 0o644)

	out, err := run(t, cfg, cache, "update")
	if err != nil {
		t.Fatalf("update: %v\n%s", err, out)
	}
	if !strings.Contains(out, "1 added") {
		t.Errorf("expected '1 added' in output:\n%s", out)
	}
	if !strings.Contains(out, "1 updated") {
		t.Errorf("expected '1 updated' in output:\n%s", out)
	}
}

func TestIndexFlag(t *testing.T) {
	t.Parallel()
	cfg, cache := setupEnv(t)
	dir := createMdDir(t, map[string]string{"a.md": "# A"})

	// Use --index custom
	out, err := run(t, cfg, cache, "--index", "custom", "collection", "add", "--name", "test", dir)
	if err != nil {
		t.Fatalf("add with --index: %v\n%s", err, out)
	}

	// Verify custom.kdl config file was created
	customKdl := filepath.Join(cfg, "custom.kdl")
	if _, err := os.Stat(customKdl); err != nil {
		t.Errorf("custom.kdl should exist at %q: %v", customKdl, err)
	}
}
