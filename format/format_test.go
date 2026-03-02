//go:build fts5

package format

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/piranha/qqmd/store"
)

func sampleSearchResults() []store.SearchResult {
	return []store.SearchResult{
		{
			DocumentResult: store.DocumentResult{
				ID:          1,
				Collection:  "docs",
				Filepath:    "guide.md",
				DisplayPath: "qqmd://docs/guide.md",
				Title:       "Installation Guide",
				Hash:        "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
				ModifiedAt:  "2025-01-15T10:00:00Z",
				BodyLength:  500,
				Body:        "# Installation Guide\n\nHow to install the software.\n\nStep 1: Download.",
			},
			Score:  1.0,
			Source: "fts",
		},
		{
			DocumentResult: store.DocumentResult{
				ID:          2,
				Collection:  "docs",
				Filepath:    "faq.md",
				DisplayPath: "qqmd://docs/faq.md",
				Title:       "FAQ",
				Hash:        "1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
				ModifiedAt:  "2025-01-16T10:00:00Z",
				BodyLength:  300,
				Body:        "# FAQ\n\nFrequently asked questions about the install process.",
			},
			Score:  0.75,
			Source: "fts",
		},
	}
}

func sampleDoc() *store.DocumentResult {
	return &store.DocumentResult{
		ID:          1,
		Collection:  "docs",
		Filepath:    "guide.md",
		DisplayPath: "qqmd://docs/guide.md",
		Title:       "Installation Guide",
		Hash:        "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
		ModifiedAt:  "2025-01-15T10:00:00Z",
		BodyLength:  100,
		Body:        "# Installation Guide\n\nHow to install.",
		Context:     "Setup documentation",
	}
}

// --- 3.1 Search results ---

func TestFormatSearchResults_JSON(t *testing.T) {
	t.Parallel()
	results := sampleSearchResults()
	out := FormatSearchResults(results, "json", Options{Query: "install"})

	var arr []map[string]interface{}
	if err := json.Unmarshal([]byte(out), &arr); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if len(arr) != 2 {
		t.Errorf("expected 2 entries, got %d", len(arr))
	}
	entry := arr[0]
	if _, ok := entry["docid"]; !ok {
		t.Error("missing docid field")
	}
	if _, ok := entry["score"]; !ok {
		t.Error("missing score field")
	}
	if _, ok := entry["file"]; !ok {
		t.Error("missing file field")
	}
	// Should have snippet, not body (not full)
	if _, ok := entry["snippet"]; !ok {
		t.Error("expected snippet field in non-full mode")
	}
}

func TestFormatSearchResults_JSON_Full(t *testing.T) {
	t.Parallel()
	results := sampleSearchResults()
	out := FormatSearchResults(results, "json", Options{Full: true, Query: "install"})

	var arr []map[string]interface{}
	json.Unmarshal([]byte(out), &arr)
	if _, ok := arr[0]["body"]; !ok {
		t.Error("expected body field in full mode")
	}
}

func TestFormatSearchResults_CSV(t *testing.T) {
	t.Parallel()
	results := sampleSearchResults()
	out := FormatSearchResults(results, "csv", Options{Query: "install"})

	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected header + data rows, got %d lines", len(lines))
	}
	// Header
	if !strings.Contains(lines[0], "docid") {
		t.Error("CSV header should contain 'docid'")
	}
}

func TestFormatSearchResults_MD(t *testing.T) {
	t.Parallel()
	results := sampleSearchResults()
	out := FormatSearchResults(results, "md", Options{Query: "install"})

	if !strings.Contains(out, "# ") {
		t.Error("markdown should contain headings")
	}
	if !strings.Contains(out, "#abcdef") {
		t.Error("markdown should contain docid")
	}
}

func TestFormatSearchResults_XML(t *testing.T) {
	t.Parallel()
	results := []store.SearchResult{
		{
			DocumentResult: store.DocumentResult{
				ID:          1,
				Collection:  "test",
				Filepath:    "file.md",
				DisplayPath: "qqmd://test/file.md",
				Title:       "Title with <special> & \"chars\"",
				Hash:        "aabbccdd11223344",
				Body:        "Content with <html> & 'quotes'",
			},
			Score: 0.9,
		},
	}
	out := FormatSearchResults(results, "xml", Options{Query: "test"})

	// Check XML escaping
	if strings.Contains(out, "<special>") {
		t.Error("< and > should be escaped")
	}
	if !strings.Contains(out, "&lt;special&gt;") {
		t.Error("expected XML-escaped < and >")
	}
	if !strings.Contains(out, "&amp;") {
		t.Error("expected XML-escaped &")
	}
}

func TestFormatSearchResults_Files(t *testing.T) {
	t.Parallel()
	results := sampleSearchResults()
	out := FormatSearchResults(results, "files", Options{})

	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	// Format: #docid,score,path
	if !strings.HasPrefix(lines[0], "#") {
		t.Error("each line should start with #docid")
	}
	if !strings.Contains(lines[0], "qqmd://") {
		t.Error("each line should contain path")
	}
}

func TestFormatSearchResults_LineNumbers(t *testing.T) {
	t.Parallel()
	results := sampleSearchResults()
	out := FormatSearchResults(results, "json", Options{
		Full:        true,
		Query:       "install",
		LineNumbers: true,
	})

	var arr []map[string]interface{}
	json.Unmarshal([]byte(out), &arr)
	body, ok := arr[0]["body"].(string)
	if !ok {
		t.Fatal("expected body string")
	}
	// Line numbers should be present
	if !strings.Contains(body, "1: ") {
		t.Error("expected line numbers in body")
	}
}

// --- 3.2 Single document ---

func TestFormatDocument_JSON(t *testing.T) {
	t.Parallel()
	doc := sampleDoc()
	out := FormatDocument(doc, "json", Options{})

	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(out), &obj); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	for _, key := range []string{"file", "title", "hash", "docid", "modified_at", "body_length"} {
		if _, ok := obj[key]; !ok {
			t.Errorf("missing field %q", key)
		}
	}
}

func TestFormatDocument_MD(t *testing.T) {
	t.Parallel()
	doc := sampleDoc()
	out := FormatDocument(doc, "md", Options{})

	if !strings.Contains(out, "# Installation Guide") {
		t.Error("should contain title as heading")
	}
	if !strings.Contains(out, "---") {
		t.Error("should contain separator")
	}
}

func TestFormatDocument_XML(t *testing.T) {
	t.Parallel()
	doc := sampleDoc()
	out := FormatDocument(doc, "xml", Options{})

	if !strings.Contains(out, "<document>") {
		t.Error("should contain <document> tag")
	}
	if !strings.Contains(out, "<title>") {
		t.Error("should contain <title> tag")
	}
}

func TestFormatDocument_CSV(t *testing.T) {
	t.Parallel()
	doc := sampleDoc()
	out := FormatDocument(doc, "csv", Options{})

	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 2 {
		t.Error("expected header + data row")
	}
	if !strings.Contains(lines[0], "file") {
		t.Error("header should contain 'file'")
	}
}

// --- 3.3 Multiple documents ---

func TestFormatDocuments_JSON(t *testing.T) {
	t.Parallel()
	docs := []store.DocumentResult{
		{DisplayPath: "qqmd://a/1.md", Title: "Doc 1", Body: "content 1"},
		{DisplayPath: "qqmd://b/2.md", Title: "Doc 2", Body: "content 2"},
	}
	out := FormatDocuments(docs, "json", Options{})

	var arr []map[string]interface{}
	if err := json.Unmarshal([]byte(out), &arr); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(arr) != 2 {
		t.Errorf("expected 2 entries, got %d", len(arr))
	}
}

func TestFormatDocuments_Files(t *testing.T) {
	t.Parallel()
	docs := []store.DocumentResult{
		{DisplayPath: "qqmd://a/1.md"},
		{DisplayPath: "qqmd://b/2.md"},
	}
	out := FormatDocuments(docs, "files", Options{})

	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		t.Errorf("expected 2 lines, got %d", len(lines))
	}
	if !strings.Contains(lines[0], "qqmd://") {
		t.Error("each line should contain path")
	}
}

// --- 3.4 File list & status ---

func TestFormatFileList_JSON(t *testing.T) {
	t.Parallel()
	docs := []store.DocumentResult{
		{
			DisplayPath: "qqmd://test/file.md",
			Title:       "Test File",
			Hash:        "abcdef1234",
			ModifiedAt:  "2025-01-01T00:00:00Z",
			BodyLength:  42,
		},
	}
	out := FormatFileList(docs, "json")

	var arr []map[string]interface{}
	if err := json.Unmarshal([]byte(out), &arr); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	entry := arr[0]
	for _, key := range []string{"docid", "file", "title", "modified_at"} {
		if _, ok := entry[key]; !ok {
			t.Errorf("missing field %q", key)
		}
	}
}

func TestFormatFileList_CSV(t *testing.T) {
	t.Parallel()
	docs := []store.DocumentResult{
		{DisplayPath: "qqmd://test/file.md", Title: "Test", Hash: "abcdef", ModifiedAt: "2025-01-01"},
	}
	out := FormatFileList(docs, "csv")

	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 2 {
		t.Error("expected header + rows")
	}
	if !strings.Contains(lines[0], "docid") {
		t.Error("header should contain 'docid'")
	}
}

func TestFormatStatus_JSON(t *testing.T) {
	t.Parallel()
	st := &store.StoreStatus{
		DBPath:          "/tmp/test.sqlite",
		DBSize:          1024,
		TotalDocuments:  10,
		TotalContent:    8,
		TotalEmbeddings: 5,
		Collections:     map[string]int{"docs": 7, "wiki": 3},
	}
	out := FormatStatus(st, "json")

	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(out), &obj); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if _, ok := obj["DBPath"]; !ok {
		t.Error("missing DBPath field")
	}
	if _, ok := obj["TotalDocuments"]; !ok {
		t.Error("missing TotalDocuments field")
	}
}

// --- 3.5 Helpers ---

func TestEscapeCSV(t *testing.T) {
	t.Parallel()
	cases := []struct {
		input string
		want  string
	}{
		{"simple", "simple"},
		{"with,comma", `"with,comma"`},
		{`with"quote`, `"with""quote"`},
		{"with\nnewline", `"with` + "\n" + `newline"`},
		{"", ""},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got := escapeCSV(tc.input)
			if got != tc.want {
				t.Errorf("escapeCSV(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestEscapeXML(t *testing.T) {
	t.Parallel()
	input := `Hello & <world> "foo" 'bar'`
	got := escapeXML(input)
	if !strings.Contains(got, "&amp;") {
		t.Error("& not escaped")
	}
	if !strings.Contains(got, "&lt;") {
		t.Error("< not escaped")
	}
	if !strings.Contains(got, "&gt;") {
		t.Error("> not escaped")
	}
	if !strings.Contains(got, "&quot;") {
		t.Error("\" not escaped")
	}
	if !strings.Contains(got, "&apos;") {
		t.Error("' not escaped")
	}
}

func TestRound2(t *testing.T) {
	t.Parallel()
	cases := []struct {
		input float64
		want  float64
	}{
		{1.234, 1.23},
		{1.235, 1.24},
		{1.0, 1.0},
		{0.999, 1.0},
	}
	for _, tc := range cases {
		got := round2(tc.input)
		if got != tc.want {
			t.Errorf("round2(%f) = %f, want %f", tc.input, got, tc.want)
		}
	}
}
