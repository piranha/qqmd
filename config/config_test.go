//go:build fts5

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// resetIndex resets the global indexName for test isolation.
func resetIndex(t *testing.T) {
	t.Helper()
	old := indexName
	indexName = "index"
	t.Cleanup(func() { indexName = old })
}

// setConfigDir sets QQMD_CONFIG_DIR to a temp dir for test isolation.
// NOTE: Uses os.Setenv so tests calling this MUST NOT use t.Parallel().
func setConfigDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	old := os.Getenv("QQMD_CONFIG_DIR")
	os.Setenv("QQMD_CONFIG_DIR", dir)
	resetIndex(t)
	t.Cleanup(func() {
		if old == "" {
			os.Unsetenv("QQMD_CONFIG_DIR")
		} else {
			os.Setenv("QQMD_CONFIG_DIR", old)
		}
	})
	return dir
}

// --- 1.1 KDL round-trip ---

func TestMarshalUnmarshalEmpty(t *testing.T) {
	t.Parallel()
	cfg := &Config{Collections: make(map[string]Collection)}
	data, err := marshalConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	got, err := unmarshalConfig(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Collections) != 0 {
		t.Fatalf("expected 0 collections, got %d", len(got.Collections))
	}
}

func TestMarshalUnmarshalMinimal(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		Collections: map[string]Collection{
			"docs": {Path: "/tmp/docs", Pattern: "**/*.md"},
		},
	}
	data, err := marshalConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	got, err := unmarshalConfig(data)
	if err != nil {
		t.Fatal(err)
	}
	c, ok := got.Collections["docs"]
	if !ok {
		t.Fatal("collection 'docs' not found")
	}
	if c.Path != "/tmp/docs" {
		t.Errorf("path = %q, want /tmp/docs", c.Path)
	}
	if c.Pattern != "**/*.md" {
		t.Errorf("pattern = %q, want **/*.md", c.Pattern)
	}
}

func TestMarshalUnmarshalFull(t *testing.T) {
	t.Parallel()
	tr := true
	cfg := &Config{
		Collections: map[string]Collection{
			"wiki": {
				Path:             "/home/user/wiki",
				Pattern:          "**/*.md",
				Update:           "git pull",
				IncludeByDefault: &tr,
				Context: map[string]string{
					"/guides": "User guides and tutorials",
					"/api":    "API reference documentation",
				},
			},
		},
	}
	data, err := marshalConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	got, err := unmarshalConfig(data)
	if err != nil {
		t.Fatal(err)
	}
	c := got.Collections["wiki"]
	if c.Path != "/home/user/wiki" {
		t.Errorf("path = %q", c.Path)
	}
	if c.Pattern != "**/*.md" {
		t.Errorf("pattern = %q", c.Pattern)
	}
	if c.Update != "git pull" {
		t.Errorf("update = %q", c.Update)
	}
	if c.IncludeByDefault == nil || !*c.IncludeByDefault {
		t.Error("include-by-default should be true")
	}
	if len(c.Context) != 2 {
		t.Errorf("contexts = %d, want 2", len(c.Context))
	}
	if c.Context["/guides"] != "User guides and tutorials" {
		t.Errorf("context /guides = %q", c.Context["/guides"])
	}
}

func TestMarshalIncludeByDefaultFalse(t *testing.T) {
	t.Parallel()
	f := false
	cfg := &Config{
		Collections: map[string]Collection{
			"test": {Path: "/tmp", Pattern: "*.md", IncludeByDefault: &f},
		},
	}
	data, err := marshalConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	got, err := unmarshalConfig(data)
	if err != nil {
		t.Fatal(err)
	}
	c := got.Collections["test"]
	if c.IncludeByDefault == nil {
		t.Fatal("IncludeByDefault is nil, expected *false")
	}
	if *c.IncludeByDefault {
		t.Error("IncludeByDefault should be false")
	}
}

func TestMarshalIncludeByDefaultNil(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		Collections: map[string]Collection{
			"test": {Path: "/tmp", Pattern: "*.md", IncludeByDefault: nil},
		},
	}
	data, err := marshalConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if strings.Contains(s, "include-by-default") {
		t.Errorf("output should not contain include-by-default, got:\n%s", s)
	}
}

func TestMarshalGlobalContext(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		GlobalContext: "You are a helpful assistant.",
		Collections:  make(map[string]Collection),
	}
	data, err := marshalConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	got, err := unmarshalConfig(data)
	if err != nil {
		t.Fatal(err)
	}
	if got.GlobalContext != "You are a helpful assistant." {
		t.Errorf("GlobalContext = %q", got.GlobalContext)
	}
}

func TestMarshalMultipleCollections(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		Collections: map[string]Collection{
			"alpha": {Path: "/a", Pattern: "*.md"},
			"beta":  {Path: "/b", Pattern: "*.txt"},
		},
	}
	data, err := marshalConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	got, err := unmarshalConfig(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Collections) != 2 {
		t.Fatalf("expected 2 collections, got %d", len(got.Collections))
	}
	if got.Collections["alpha"].Path != "/a" {
		t.Errorf("alpha path = %q", got.Collections["alpha"].Path)
	}
	if got.Collections["beta"].Path != "/b" {
		t.Errorf("beta path = %q", got.Collections["beta"].Path)
	}
}

func TestUnmarshalMalformedKDL(t *testing.T) {
	t.Parallel()
	_, err := unmarshalConfig([]byte("this is {{{ not valid kdl"))
	if err == nil {
		t.Error("expected error for malformed KDL")
	}
}

func TestUnmarshalExtraNodes(t *testing.T) {
	t.Parallel()
	kdl := `
unknown-node "hello"
collection "test" {
    path "/tmp"
    pattern "*.md"
}
future-feature 42
`
	cfg, err := unmarshalConfig([]byte(kdl))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg.Collections["test"]; !ok {
		t.Error("collection 'test' not found after ignoring unknown nodes")
	}
}

func TestUnmarshalCollectionNoArgs(t *testing.T) {
	t.Parallel()
	kdl := `
collection {
    path "/tmp"
    pattern "*.md"
}
collection "valid" {
    path "/home"
    pattern "**/*.md"
}
`
	cfg, err := unmarshalConfig([]byte(kdl))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Collections) != 1 {
		t.Fatalf("expected 1 collection (no-arg skipped), got %d", len(cfg.Collections))
	}
	if _, ok := cfg.Collections["valid"]; !ok {
		t.Error("collection 'valid' not found")
	}
}

// --- 1.2 Load / Save / file lifecycle ---

func TestLoadConfigMissing(t *testing.T) {
	setConfigDir(t)
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Collections == nil {
		t.Error("Collections map should be initialized, not nil")
	}
	if len(cfg.Collections) != 0 {
		t.Errorf("expected 0 collections, got %d", len(cfg.Collections))
	}
}

func TestSaveAndLoad(t *testing.T) {
	setConfigDir(t)
	tr := true
	cfg := &Config{
		GlobalContext: "global ctx",
		Collections: map[string]Collection{
			"test": {
				Path: "/tmp/test", Pattern: "**/*.md",
				IncludeByDefault: &tr,
				Context:          map[string]string{"/": "root context"},
			},
		},
	}
	if err := SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	got, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if got.GlobalContext != "global ctx" {
		t.Errorf("GlobalContext = %q", got.GlobalContext)
	}
	c := got.Collections["test"]
	if c.Path != "/tmp/test" {
		t.Errorf("path = %q", c.Path)
	}
}

func TestConfigPathRespectsEnv(t *testing.T) {
	resetIndex(t)

	// Test QQMD_CONFIG_DIR
	dir := t.TempDir()
	os.Setenv("QQMD_CONFIG_DIR", dir)
	t.Cleanup(func() { os.Unsetenv("QQMD_CONFIG_DIR") })
	p := ConfigPath()
	if !strings.HasPrefix(p, dir) {
		t.Errorf("ConfigPath() = %q, should start with %q", p, dir)
	}

	// Test XDG_CONFIG_HOME fallback
	os.Unsetenv("QQMD_CONFIG_DIR")
	xdg := t.TempDir()
	oldXDG := os.Getenv("XDG_CONFIG_HOME")
	os.Setenv("XDG_CONFIG_HOME", xdg)
	t.Cleanup(func() {
		if oldXDG == "" {
			os.Unsetenv("XDG_CONFIG_HOME")
		} else {
			os.Setenv("XDG_CONFIG_HOME", oldXDG)
		}
	})
	p = ConfigPath()
	if !strings.HasPrefix(p, filepath.Join(xdg, "qqmd")) {
		t.Errorf("ConfigPath() = %q, should start with %q", p, filepath.Join(xdg, "qqmd"))
	}
}

func TestConfigPathExtension(t *testing.T) {
	setConfigDir(t)
	p := ConfigPath()
	if !strings.HasSuffix(p, ".kdl") {
		t.Errorf("ConfigPath() = %q, should end with .kdl", p)
	}
}

// --- 1.3 SetIndexName ---

func TestSetIndexNameSimple(t *testing.T) {
	dir := setConfigDir(t)
	SetIndexName("myindex")
	p := ConfigPath()
	expected := filepath.Join(dir, "myindex.kdl")
	if p != expected {
		t.Errorf("ConfigPath() = %q, want %q", p, expected)
	}
}

func TestSetIndexNameWithSlashes(t *testing.T) {
	setConfigDir(t)
	SetIndexName("path/to/index")
	p := ConfigPath()
	// Should not contain forward slashes in filename
	base := filepath.Base(p)
	if strings.Contains(base, "/") {
		t.Errorf("base name %q should not contain /", base)
	}
	if !strings.HasSuffix(base, ".kdl") {
		t.Errorf("should end with .kdl, got %q", base)
	}
	// Should not start with underscore
	name := strings.TrimSuffix(base, ".kdl")
	if strings.HasPrefix(name, "_") {
		t.Errorf("name %q should not start with _", name)
	}
}

// --- 1.4 Collection CRUD ---

func TestAddCollection(t *testing.T) {
	setConfigDir(t)
	dir := t.TempDir()
	if err := AddCollection("test", dir, ""); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	c, ok := cfg.Collections["test"]
	if !ok {
		t.Fatal("collection not found")
	}
	if !filepath.IsAbs(c.Path) {
		t.Errorf("path %q should be absolute", c.Path)
	}
	if c.Pattern != "**/*.md" {
		t.Errorf("default pattern = %q, want **/*.md", c.Pattern)
	}
}

func TestAddCollectionCustomPattern(t *testing.T) {
	setConfigDir(t)
	dir := t.TempDir()
	if err := AddCollection("test", dir, "**/*.txt"); err != nil {
		t.Fatal(err)
	}
	cfg, _ := LoadConfig()
	if cfg.Collections["test"].Pattern != "**/*.txt" {
		t.Errorf("pattern = %q, want **/*.txt", cfg.Collections["test"].Pattern)
	}
}

func TestAddCollectionPreservesContext(t *testing.T) {
	setConfigDir(t)
	dir := t.TempDir()
	// Add with context
	if err := AddCollection("test", dir, ""); err != nil {
		t.Fatal(err)
	}
	if err := AddContext("test", "/docs", "documentation"); err != nil {
		t.Fatal(err)
	}
	// Re-add same collection
	if err := AddCollection("test", dir, ""); err != nil {
		t.Fatal(err)
	}
	cfg, _ := LoadConfig()
	c := cfg.Collections["test"]
	if c.Context == nil || c.Context["/docs"] != "documentation" {
		t.Error("context should be preserved on re-add")
	}
}

func TestRemoveCollection(t *testing.T) {
	setConfigDir(t)
	dir := t.TempDir()
	AddCollection("test", dir, "")
	if err := RemoveCollection("test"); err != nil {
		t.Fatal(err)
	}
	cfg, _ := LoadConfig()
	if _, ok := cfg.Collections["test"]; ok {
		t.Error("collection should be removed")
	}
	// Removing non-existent returns error
	if err := RemoveCollection("nope"); err == nil {
		t.Error("expected error removing non-existent collection")
	}
}

func TestRenameCollection(t *testing.T) {
	setConfigDir(t)
	dir := t.TempDir()
	AddCollection("old", dir, "")
	if err := RenameCollection("old", "new"); err != nil {
		t.Fatal(err)
	}
	cfg, _ := LoadConfig()
	if _, ok := cfg.Collections["old"]; ok {
		t.Error("old name should be gone")
	}
	if _, ok := cfg.Collections["new"]; !ok {
		t.Error("new name should be present")
	}
}

func TestRenameCollectionConflict(t *testing.T) {
	setConfigDir(t)
	dir := t.TempDir()
	AddCollection("a", dir, "")
	AddCollection("b", dir, "")
	if err := RenameCollection("a", "b"); err == nil {
		t.Error("expected error renaming to existing name")
	}
}

func TestRenameCollectionNotFound(t *testing.T) {
	setConfigDir(t)
	if err := RenameCollection("ghost", "new"); err == nil {
		t.Error("expected error renaming missing collection")
	}
}

func TestGetCollectionExists(t *testing.T) {
	setConfigDir(t)
	dir := t.TempDir()
	AddCollection("test", dir, "**/*.md")
	nc, err := GetCollection("test")
	if err != nil {
		t.Fatal(err)
	}
	if nc == nil {
		t.Fatal("expected non-nil")
	}
	if nc.Name != "test" {
		t.Errorf("name = %q", nc.Name)
	}
	if nc.Pattern != "**/*.md" {
		t.Errorf("pattern = %q", nc.Pattern)
	}
}

func TestGetCollectionNotFound(t *testing.T) {
	setConfigDir(t)
	nc, err := GetCollection("nope")
	if err != nil {
		t.Fatal(err)
	}
	if nc != nil {
		t.Error("expected nil for missing collection")
	}
}

func TestListCollections(t *testing.T) {
	setConfigDir(t)
	dir := t.TempDir()
	AddCollection("a", dir, "")
	AddCollection("b", dir, "")
	colls, err := ListCollections()
	if err != nil {
		t.Fatal(err)
	}
	if len(colls) != 2 {
		t.Errorf("expected 2, got %d", len(colls))
	}
}

func TestUpdateCollectionSettings(t *testing.T) {
	setConfigDir(t)
	dir := t.TempDir()
	AddCollection("test", dir, "")

	cmd := "make build"
	if err := UpdateCollectionSettings("test", &cmd, nil); err != nil {
		t.Fatal(err)
	}
	cfg, _ := LoadConfig()
	if cfg.Collections["test"].Update != "make build" {
		t.Errorf("update = %q", cfg.Collections["test"].Update)
	}

	inc := false
	if err := UpdateCollectionSettings("test", nil, &inc); err != nil {
		t.Fatal(err)
	}
	cfg, _ = LoadConfig()
	if cfg.Collections["test"].IncludeByDefault == nil || *cfg.Collections["test"].IncludeByDefault {
		t.Error("include-by-default should be false")
	}
	// Update should still be set
	if cfg.Collections["test"].Update != "make build" {
		t.Error("update should still be set")
	}
}

func TestUpdateCollectionNotFound(t *testing.T) {
	setConfigDir(t)
	cmd := "test"
	if err := UpdateCollectionSettings("nope", &cmd, nil); err == nil {
		t.Error("expected error for missing collection")
	}
}

// --- 1.5 Default collection names ---

func TestGetDefaultCollectionNames_AllDefault(t *testing.T) {
	setConfigDir(t)
	dir := t.TempDir()
	// nil IncludeByDefault treated as true
	AddCollection("a", dir, "")
	AddCollection("b", dir, "")
	names, err := GetDefaultCollectionNames()
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 {
		t.Errorf("expected 2, got %d", len(names))
	}
}

func TestGetDefaultCollectionNames_Mixed(t *testing.T) {
	setConfigDir(t)
	dir := t.TempDir()
	AddCollection("incl", dir, "")
	AddCollection("excl", dir, "")
	f := false
	UpdateCollectionSettings("excl", nil, &f)

	names, err := GetDefaultCollectionNames()
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 1 {
		t.Fatalf("expected 1, got %d: %v", len(names), names)
	}
	if names[0] != "incl" {
		t.Errorf("expected 'incl', got %q", names[0])
	}
}

// --- 1.6 Context management ---

func TestAddContext(t *testing.T) {
	setConfigDir(t)
	dir := t.TempDir()
	AddCollection("test", dir, "")
	if err := AddContext("test", "/docs", "documentation section"); err != nil {
		t.Fatal(err)
	}
	cfg, _ := LoadConfig()
	c := cfg.Collections["test"]
	if c.Context["/docs"] != "documentation section" {
		t.Errorf("context = %q", c.Context["/docs"])
	}
}

func TestAddContextUpdate(t *testing.T) {
	setConfigDir(t)
	dir := t.TempDir()
	AddCollection("test", dir, "")
	AddContext("test", "/docs", "old description")
	AddContext("test", "/docs", "new description")
	cfg, _ := LoadConfig()
	if cfg.Collections["test"].Context["/docs"] != "new description" {
		t.Error("context description should be updated")
	}
}

func TestAddContextMissingCollection(t *testing.T) {
	setConfigDir(t)
	if err := AddContext("nope", "/x", "desc"); err == nil {
		t.Error("expected error for missing collection")
	}
}

func TestRemoveContext(t *testing.T) {
	setConfigDir(t)
	dir := t.TempDir()
	AddCollection("test", dir, "")
	AddContext("test", "/a", "desc a")
	AddContext("test", "/b", "desc b")

	if err := RemoveContext("test", "/a"); err != nil {
		t.Fatal(err)
	}
	cfg, _ := LoadConfig()
	c := cfg.Collections["test"]
	if _, ok := c.Context["/a"]; ok {
		t.Error("/a should be removed")
	}
	if c.Context["/b"] != "desc b" {
		t.Error("/b should remain")
	}

	// Remove last context -> map becomes nil
	if err := RemoveContext("test", "/b"); err != nil {
		t.Fatal(err)
	}
	cfg, _ = LoadConfig()
	if cfg.Collections["test"].Context != nil {
		t.Error("context map should be nil after removing last entry")
	}
}

func TestRemoveContextNotFound(t *testing.T) {
	setConfigDir(t)
	dir := t.TempDir()
	AddCollection("test", dir, "")
	AddContext("test", "/a", "desc")
	if err := RemoveContext("test", "/nonexistent"); err == nil {
		t.Error("expected error for non-existent prefix")
	}
}

func TestRemoveContextNoContexts(t *testing.T) {
	setConfigDir(t)
	dir := t.TempDir()
	AddCollection("test", dir, "")
	if err := RemoveContext("test", "/x"); err == nil {
		t.Error("expected error when collection has no contexts")
	}
}

func TestListAllContexts(t *testing.T) {
	setConfigDir(t)
	dir := t.TempDir()
	// Set global context
	cfg := &Config{
		GlobalContext: "global help",
		Collections: map[string]Collection{
			"test": {Path: dir, Pattern: "*.md", Context: map[string]string{"/api": "api docs"}},
		},
	}
	SaveConfig(cfg)
	entries, err := ListAllContexts()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries (global + 1), got %d", len(entries))
	}
	foundGlobal := false
	foundApi := false
	for _, e := range entries {
		if e.Collection == "*" && e.Description == "global help" {
			foundGlobal = true
		}
		if e.Collection == "test" && e.Path == "/api" {
			foundApi = true
		}
	}
	if !foundGlobal {
		t.Error("missing global context entry")
	}
	if !foundApi {
		t.Error("missing /api context entry")
	}
}

func TestFindContextForPath_LongestPrefix(t *testing.T) {
	setConfigDir(t)
	dir := t.TempDir()
	cfg := &Config{
		Collections: map[string]Collection{
			"test": {
				Path: dir, Pattern: "*.md",
				Context: map[string]string{
					"/guides":          "general guides",
					"/guides/advanced": "advanced guides",
				},
			},
		},
	}
	SaveConfig(cfg)
	ctx := FindContextForPath("test", "/guides/advanced/topic.md")
	if ctx != "advanced guides" {
		t.Errorf("context = %q, want 'advanced guides'", ctx)
	}
}

func TestFindContextForPath_FallbackGlobal(t *testing.T) {
	setConfigDir(t)
	dir := t.TempDir()
	cfg := &Config{
		GlobalContext: "global fallback",
		Collections: map[string]Collection{
			"test": {
				Path: dir, Pattern: "*.md",
				Context: map[string]string{
					"/api": "api context",
				},
			},
		},
	}
	SaveConfig(cfg)
	ctx := FindContextForPath("test", "/unrelated/file.md")
	if ctx != "global fallback" {
		t.Errorf("context = %q, want 'global fallback'", ctx)
	}
}

func TestFindContextForPath_NoContexts(t *testing.T) {
	setConfigDir(t)
	dir := t.TempDir()
	cfg := &Config{
		GlobalContext: "global",
		Collections: map[string]Collection{
			"test": {Path: dir, Pattern: "*.md"},
		},
	}
	SaveConfig(cfg)
	ctx := FindContextForPath("test", "/any/path.md")
	if ctx != "global" {
		t.Errorf("context = %q, want 'global'", ctx)
	}
}

func TestFindContextForPath_MissingCollection(t *testing.T) {
	setConfigDir(t)
	cfg := &Config{
		GlobalContext: "global",
		Collections:  make(map[string]Collection),
	}
	SaveConfig(cfg)
	ctx := FindContextForPath("nonexistent", "/file.md")
	if ctx != "global" {
		t.Errorf("context = %q, want 'global'", ctx)
	}
}

func TestFindContextForPath_NormalizesSlashes(t *testing.T) {
	setConfigDir(t)
	dir := t.TempDir()
	cfg := &Config{
		Collections: map[string]Collection{
			"test": {
				Path: dir, Pattern: "*.md",
				Context: map[string]string{
					"/docs": "docs context",
				},
			},
		},
	}
	SaveConfig(cfg)
	// Path without leading /
	ctx := FindContextForPath("test", "docs/file.md")
	if ctx != "docs context" {
		t.Errorf("context = %q, want 'docs context'", ctx)
	}
}

// --- 1.7 Validation ---

func TestIsValidCollectionName(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		valid bool
	}{
		{"foo", true},
		{"a-b", true},
		{"x_1", true},
		{"ABC", true},
		{"", false},
		{"a/b", false},
		{"a b", false},
		{"a.b", false},
		{"a@b", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsValidCollectionName(tc.name); got != tc.valid {
				t.Errorf("IsValidCollectionName(%q) = %v, want %v", tc.name, got, tc.valid)
			}
		})
	}
}

// Ensure temp dirs are actually used and the real filesystem is not touched.
func TestConfigDirIsolation(t *testing.T) {
	dir := setConfigDir(t)
	AddCollection("test", t.TempDir(), "")
	// Config file should be inside temp dir
	p := ConfigPath()
	if !strings.HasPrefix(p, dir) {
		t.Errorf("ConfigPath %q is not under temp dir %q", p, dir)
	}
	if _, err := os.Stat(p); err != nil {
		t.Errorf("config file should exist at %q", p)
	}
}
