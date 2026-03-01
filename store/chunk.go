package store

import (
	"strings"
)

const (
	DefaultChunkSize    = 3600 // ~900 tokens at ~4 chars/token
	DefaultChunkOverlap = 540  // 15% overlap
)

// ChunkDocument splits a document body into overlapping chunks using
// markdown-aware break points.
func ChunkDocument(body string, chunkSize, overlap int) []Chunk {
	if chunkSize <= 0 {
		chunkSize = DefaultChunkSize
	}
	if overlap <= 0 {
		overlap = DefaultChunkOverlap
	}

	if len(body) <= chunkSize {
		return []Chunk{{Text: body, Start: 0, End: len(body)}}
	}

	breakPoints := scanBreakPoints(body)
	var chunks []Chunk
	pos := 0

	for pos < len(body) {
		end := pos + chunkSize
		if end >= len(body) {
			chunks = append(chunks, Chunk{Text: body[pos:], Start: pos, End: len(body)})
			break
		}

		// Find best break point near end of chunk
		cutoff := findBestCutoff(breakPoints, pos, end, chunkSize)
		if cutoff <= pos {
			cutoff = end // No good break found, hard cut
		}

		chunks = append(chunks, Chunk{Text: body[pos:cutoff], Start: pos, End: cutoff})

		// Advance with overlap
		pos = cutoff - overlap
		if pos < 0 {
			pos = 0
		}
		// Don't go backwards
		if len(chunks) > 1 && pos <= chunks[len(chunks)-2].Start {
			pos = cutoff
		}
	}

	return chunks
}

type breakPoint struct {
	pos   int
	score int
}

func scanBreakPoints(body string) []breakPoint {
	var points []breakPoint
	lines := strings.Split(body, "\n")
	pos := 0

	inCodeBlock := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Track code blocks
		if strings.HasPrefix(trimmed, "```") {
			inCodeBlock = !inCodeBlock
			points = append(points, breakPoint{pos: pos + len(line), score: 10})
			pos += len(line) + 1
			continue
		}

		if inCodeBlock {
			pos += len(line) + 1
			continue
		}

		// Score break points by markdown structure
		score := 0
		switch {
		case strings.HasPrefix(trimmed, "# "):
			score = 100
		case strings.HasPrefix(trimmed, "## "):
			score = 90
		case strings.HasPrefix(trimmed, "### "):
			score = 80
		case strings.HasPrefix(trimmed, "#### "):
			score = 70
		case strings.HasPrefix(trimmed, "---") || strings.HasPrefix(trimmed, "***"):
			score = 85
		case trimmed == "":
			score = 50 // Blank line (paragraph break)
		case strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") ||
			strings.HasPrefix(trimmed, "1."):
			score = 30 // List item
		default:
			score = 10 // Regular line
		}

		if score > 0 {
			points = append(points, breakPoint{pos: pos + len(line), score: score})
		}
		pos += len(line) + 1
	}

	return points
}

func findBestCutoff(points []breakPoint, chunkStart, idealEnd, chunkSize int) int {
	// Search window: last 25% of the chunk
	searchStart := idealEnd - chunkSize/4
	if searchStart < chunkStart {
		searchStart = chunkStart
	}

	bestPos := 0
	bestScore := -1.0

	for _, bp := range points {
		if bp.pos <= searchStart || bp.pos > idealEnd+100 {
			continue
		}
		// Score combines break quality and distance from ideal end
		dist := float64(idealEnd - bp.pos)
		if dist < 0 {
			dist = -dist * 2 // Penalize going past ideal end
		}
		distPenalty := dist * dist / float64(chunkSize*chunkSize)
		score := float64(bp.score) * (1.0 - distPenalty)
		if score > bestScore {
			bestScore = score
			bestPos = bp.pos
		}
	}

	return bestPos
}
