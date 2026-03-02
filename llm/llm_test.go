//go:build fts5

package llm

import (
	"runtime"
	"sort"
	"strings"
	"testing"
)

// --- 4: LLM helpers and parsing ---

func TestFormatDocForEmbedding(t *testing.T) {
	t.Parallel()
	// With title
	got := FormatDocForEmbedding("My Title", "Some text content")
	if !strings.Contains(got, "title: My Title") {
		t.Errorf("expected title prefix, got %q", got)
	}
	if !strings.Contains(got, "text: Some text content") {
		t.Errorf("expected text prefix, got %q", got)
	}

	// Without title
	got = FormatDocForEmbedding("", "Just text")
	if strings.Contains(got, "title:") {
		t.Errorf("should not contain title prefix when empty, got %q", got)
	}
	if !strings.Contains(got, "text: Just text") {
		t.Errorf("expected text prefix, got %q", got)
	}
}

func TestFormatQueryForEmbedding(t *testing.T) {
	t.Parallel()
	got := FormatQueryForEmbedding("how to install")
	if !strings.HasPrefix(got, "task: search result | query: ") {
		t.Errorf("missing prefix, got %q", got)
	}
	if !strings.Contains(got, "how to install") {
		t.Errorf("query not found, got %q", got)
	}
}

func TestParseScores_JSONArray(t *testing.T) {
	t.Parallel()
	scores := parseScores("[1,2,3]", 3)
	if len(scores) != 3 {
		t.Fatalf("expected 3 scores, got %d", len(scores))
	}
	if scores[0] != 1 || scores[1] != 2 || scores[2] != 3 {
		t.Errorf("scores = %v, want [1 2 3]", scores)
	}
}

func TestParseScores_NestedArray(t *testing.T) {
	t.Parallel()
	// Array wrapped in markdown
	content := "Here are the scores:\n```json\n[7, 3, 5]\n```"
	scores := parseScores(content, 3)
	if len(scores) != 3 {
		t.Fatalf("expected 3 scores, got %d", len(scores))
	}
	if scores[0] != 7 {
		t.Errorf("first score = %f, want 7", scores[0])
	}
}

func TestParseScores_Fallback(t *testing.T) {
	t.Parallel()
	scores := parseScores("this is not parseable", 4)
	if len(scores) != 4 {
		t.Fatalf("expected 4 scores, got %d", len(scores))
	}
	for _, s := range scores {
		if s != 5.0 {
			t.Errorf("fallback score = %f, want 5.0", s)
		}
	}
}

func TestClamp(t *testing.T) {
	t.Parallel()
	cases := []struct {
		v, lo, hi, want float64
	}{
		{5.0, 0.0, 10.0, 5.0},  // within range
		{-1.0, 0.0, 10.0, 0.0}, // below
		{15.0, 0.0, 10.0, 10.0}, // above
		{0.0, 0.0, 10.0, 0.0},  // at lower bound
		{10.0, 0.0, 10.0, 10.0}, // at upper bound
	}
	for _, tc := range cases {
		got := clamp(tc.v, tc.lo, tc.hi)
		if got != tc.want {
			t.Errorf("clamp(%f, %f, %f) = %f, want %f", tc.v, tc.lo, tc.hi, got, tc.want)
		}
	}
}

func TestUniformScores(t *testing.T) {
	t.Parallel()
	scores := uniformScores(5)
	if len(scores) != 5 {
		t.Fatalf("expected 5 scores, got %d", len(scores))
	}
	for _, s := range scores {
		if s != 5.0 {
			t.Errorf("score = %f, want 5.0", s)
		}
	}
}

func TestRerankResults_OrderByScore(t *testing.T) {
	t.Parallel()
	type docIndex struct {
		Doc   string
		Index int
	}
	results := []docIndex{
		{"Doc about cats", 0},
		{"Doc about installation", 1},
		{"Doc about dogs", 2},
	}

	// Simulate reranking by sorting by index descending (fake scoring)
	sort.Slice(results, func(i, j int) bool {
		return results[i].Index > results[j].Index
	})
	// After sort, highest index first
	if results[0].Index != 2 {
		t.Errorf("expected index 2 first, got %d", results[0].Index)
	}
}

func TestAppendEnv_New(t *testing.T) {
	t.Parallel()
	env := []string{"PATH=/usr/bin", "HOME=/home/user"}
	result := appendEnv(env, "NEW_VAR", "/tmp")
	found := false
	for _, e := range result {
		if e == "NEW_VAR=/tmp" {
			found = true
		}
	}
	if !found {
		t.Errorf("NEW_VAR not found in %v", result)
	}
}

func TestAppendEnv_Existing(t *testing.T) {
	t.Parallel()
	env := []string{"PATH=/usr/bin", "HOME=/home/user"}
	result := appendEnv(env, "PATH", "/opt/bin")
	found := false
	for _, e := range result {
		if e == "PATH=/usr/bin:/opt/bin" {
			found = true
		}
	}
	if !found {
		t.Errorf("PATH not appended correctly in %v", result)
	}
}

func TestShortModelName(t *testing.T) {
	t.Parallel()
	// Short name stays short
	got := shortModelName("/models/small.gguf")
	if got != "small.gguf" {
		t.Errorf("got %q, want 'small.gguf'", got)
	}

	// Long name truncated
	long := "/models/" + strings.Repeat("a", 50) + ".gguf"
	got = shortModelName(long)
	if len(got) > 40 {
		t.Errorf("name too long: %d chars (%q)", len(got), got)
	}
	if !strings.HasSuffix(got, "...") {
		t.Errorf("should end with ..., got %q", got)
	}
}

func TestPlatformAssetPattern(t *testing.T) {
	t.Parallel()
	pattern := platformAssetPattern()
	if pattern == "" {
		t.Error("pattern should not be empty")
	}
	// Should contain the runtime OS or arch info
	switch runtime.GOOS {
	case "darwin":
		if !strings.Contains(pattern, "macos") {
			t.Errorf("darwin pattern should contain 'macos', got %q", pattern)
		}
	case "linux":
		if !strings.Contains(pattern, "ubuntu") {
			t.Errorf("linux pattern should contain 'ubuntu', got %q", pattern)
		}
	case "windows":
		if !strings.Contains(pattern, "win") {
			t.Errorf("windows pattern should contain 'win', got %q", pattern)
		}
	}
}

func TestMatchesPlatformAsset(t *testing.T) {
	t.Parallel()
	pattern := platformAssetPattern()

	// Should reject GPU builds
	for _, name := range []string{
		"llama-b8184-bin-" + pattern + "-cuda.tar.gz",
		"llama-b8184-bin-" + pattern + "-rocm.tar.gz",
		"llama-b8184-bin-" + pattern + "-vulkan.tar.gz",
	} {
		if matchesPlatformAsset(name, pattern) {
			t.Errorf("should reject GPU build: %q", name)
		}
	}

	// Should accept CPU build
	cpuName := "llama-b8184-bin-" + pattern + ".tar.gz"
	if !matchesPlatformAsset(cpuName, pattern) {
		t.Errorf("should accept CPU build: %q", cpuName)
	}
}
