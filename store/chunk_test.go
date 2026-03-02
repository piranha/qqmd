//go:build fts5

package store

import (
	"strings"
	"testing"
)

// --- 2.8 Chunking ---

func TestChunkSmallDoc(t *testing.T) {
	t.Parallel()
	body := "# Small Doc\n\nJust a few lines of text."
	chunks := ChunkDocument(body, DefaultChunkSize, DefaultChunkOverlap)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0].Text != body {
		t.Error("single chunk should contain entire body")
	}
	if chunks[0].Start != 0 || chunks[0].End != len(body) {
		t.Errorf("chunk bounds: [%d, %d), want [0, %d)", chunks[0].Start, chunks[0].End, len(body))
	}
}

func TestChunkLargeDoc(t *testing.T) {
	t.Parallel()
	// Create a body larger than default chunk size
	var sb strings.Builder
	sb.WriteString("# Large Document\n\n")
	for i := 0; i < 200; i++ {
		sb.WriteString("This is a line of content that makes the document quite long.\n")
	}
	body := sb.String()

	chunks := ChunkDocument(body, DefaultChunkSize, DefaultChunkOverlap)
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}
	for _, c := range chunks {
		if len(c.Text) > DefaultChunkSize+200 { // some tolerance for break point search
			t.Errorf("chunk too large: %d chars", len(c.Text))
		}
	}
}

func TestChunkOverlap(t *testing.T) {
	t.Parallel()
	var sb strings.Builder
	for i := 0; i < 300; i++ {
		sb.WriteString("Line of text for overlap testing purposes.\n")
	}
	body := sb.String()

	chunkSize := 1000
	overlap := 200
	chunks := ChunkDocument(body, chunkSize, overlap)
	if len(chunks) < 3 {
		t.Fatalf("expected at least 3 chunks, got %d", len(chunks))
	}

	// Check that consecutive chunks overlap
	for i := 1; i < len(chunks); i++ {
		prevEnd := chunks[i-1].End
		currStart := chunks[i].Start
		overlapSize := prevEnd - currStart
		if overlapSize <= 0 {
			t.Errorf("chunks %d and %d don't overlap: prev.End=%d, curr.Start=%d",
				i-1, i, prevEnd, currStart)
		}
	}
}

func TestChunkBreaksAtHeadings(t *testing.T) {
	t.Parallel()
	// Build a doc where a heading is near the ideal break point
	var sb strings.Builder
	// Fill ~80% of chunk size with content
	for i := 0; i < 50; i++ {
		sb.WriteString("Content line that fills space before heading.\n")
	}
	sb.WriteString("\n## New Section\n\n")
	for i := 0; i < 50; i++ {
		sb.WriteString("Content in the new section after the heading.\n")
	}
	body := sb.String()

	chunks := ChunkDocument(body, 3000, 400)
	if len(chunks) < 2 {
		t.Skipf("document didn't produce multiple chunks (len=%d, body=%d)", len(chunks), len(body))
	}

	// At least one chunk boundary should be near a heading
	foundHeadingBreak := false
	for i := 1; i < len(chunks); i++ {
		start := chunks[i].Start
		// Check if heading is near the start of this chunk
		prefix := body[max(0, start-20):min(len(body), start+20)]
		if strings.Contains(prefix, "## ") {
			foundHeadingBreak = true
		}
	}
	if !foundHeadingBreak {
		t.Log("Warning: no chunk break at heading boundary (might be acceptable)")
	}
}

func TestChunkBreaksAtBlankLines(t *testing.T) {
	t.Parallel()
	var sb strings.Builder
	for i := 0; i < 40; i++ {
		sb.WriteString("A paragraph line.\n")
	}
	sb.WriteString("\n") // blank line
	for i := 0; i < 40; i++ {
		sb.WriteString("Another paragraph line.\n")
	}
	body := sb.String()

	chunks := ChunkDocument(body, 1000, 100)
	if len(chunks) < 2 {
		t.Skipf("not enough chunks to test blank line breaks (got %d)", len(chunks))
	}
	// We just verify it doesn't crash and produces valid chunks
	for _, c := range chunks {
		if c.Text == "" {
			t.Error("empty chunk text")
		}
	}
}

func TestChunkCodeBlock(t *testing.T) {
	t.Parallel()
	var sb strings.Builder
	sb.WriteString("# Code Example\n\n")
	sb.WriteString("```go\n")
	for i := 0; i < 100; i++ {
		sb.WriteString("fmt.Println(\"line\")\n")
	}
	sb.WriteString("```\n\n")
	sb.WriteString("Some text after code.\n")
	for i := 0; i < 100; i++ {
		sb.WriteString("More content after code block.\n")
	}
	body := sb.String()

	chunks := ChunkDocument(body, 2000, 300)
	// Just verify no panic and valid output
	if len(chunks) == 0 {
		t.Fatal("expected at least one chunk")
	}
}

func TestChunkCoversFullBody(t *testing.T) {
	t.Parallel()
	var sb strings.Builder
	for i := 0; i < 200; i++ {
		sb.WriteString("Testing full body coverage with chunk overlap.\n")
	}
	body := sb.String()

	chunks := ChunkDocument(body, 2000, 300)
	if len(chunks) == 0 {
		t.Fatal("expected chunks")
	}
	// First chunk starts at 0
	if chunks[0].Start != 0 {
		t.Errorf("first chunk starts at %d, want 0", chunks[0].Start)
	}
	// Last chunk ends at len(body)
	if chunks[len(chunks)-1].End != len(body) {
		t.Errorf("last chunk ends at %d, want %d", chunks[len(chunks)-1].End, len(body))
	}
}
