// Package format provides output formatters for qqmd search results and documents.
package format

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/piranha/qqmd/store"
)

type Options struct {
	Full        bool
	Query       string
	LineNumbers bool
}

// FormatSearchResults formats search results in the specified output format.
func FormatSearchResults(results []store.SearchResult, format string, opts Options) string {
	switch format {
	case "json":
		return searchResultsJSON(results, opts)
	case "csv":
		return searchResultsCSV(results, opts)
	case "md":
		return searchResultsMD(results, opts)
	case "xml":
		return searchResultsXML(results, opts)
	case "files":
		return searchResultsFiles(results)
	default:
		return searchResultsJSON(results, opts)
	}
}

// FormatDocument formats a single document in the specified output format.
func FormatDocument(doc *store.DocumentResult, format string, opts Options) string {
	switch format {
	case "json":
		return documentJSON(doc, opts)
	case "md":
		return documentMD(doc, opts)
	case "xml":
		return documentXML(doc, opts)
	case "csv":
		return documentCSV(doc, opts)
	default:
		return documentJSON(doc, opts)
	}
}

// FormatDocuments formats multiple documents in the specified output format.
func FormatDocuments(docs []store.DocumentResult, format string, opts Options) string {
	switch format {
	case "json":
		return documentsJSON(docs, opts)
	case "md":
		return documentsMD(docs, opts)
	case "xml":
		return documentsXML(docs, opts)
	case "csv":
		return documentsCSV(docs, opts)
	case "files":
		return documentsFiles(docs)
	default:
		return documentsJSON(docs, opts)
	}
}

// FormatFileList formats a list of documents for the ls command.
func FormatFileList(docs []store.DocumentResult, format string) string {
	switch format {
	case "json":
		return fileListJSON(docs)
	case "csv":
		return fileListCSV(docs)
	default:
		return fileListJSON(docs)
	}
}

// FormatStatus formats store status information.
func FormatStatus(st *store.StoreStatus, format string) string {
	switch format {
	case "json":
		return statusJSON(st)
	default:
		return statusJSON(st)
	}
}

// --- Search Results ---

func searchResultsJSON(results []store.SearchResult, opts Options) string {
	type entry struct {
		Docid   string  `json:"docid"`
		Score   float64 `json:"score"`
		File    string  `json:"file"`
		Title   string  `json:"title"`
		Context string  `json:"context,omitempty"`
		Body    string  `json:"body,omitempty"`
		Snippet string  `json:"snippet,omitempty"`
	}
	entries := make([]entry, len(results))
	for i, r := range results {
		e := entry{
			Docid:   "#" + r.Docid(),
			Score:   round2(r.Score),
			File:    r.DisplayPath,
			Title:   r.Title,
			Context: r.Context,
		}
		if opts.Full && r.Body != "" {
			body := r.Body
			if opts.LineNumbers {
				body = store.AddLineNumbers(body, 1)
			}
			e.Body = body
		} else if r.Body != "" {
			snippet, _ := store.ExtractSnippet(r.Body, opts.Query, 300)
			if opts.LineNumbers {
				snippet = store.AddLineNumbers(snippet, 1)
			}
			e.Snippet = snippet
		}
		entries[i] = e
	}
	data, _ := json.MarshalIndent(entries, "", "  ")
	return string(data)
}

func searchResultsCSV(results []store.SearchResult, opts Options) string {
	var sb strings.Builder
	sb.WriteString("docid,score,file,title,context,snippet\n")
	for _, r := range results {
		snippet := ""
		if r.Body != "" {
			if opts.Full {
				snippet = r.Body
			} else {
				snippet, _ = store.ExtractSnippet(r.Body, opts.Query, 500)
			}
			if opts.LineNumbers {
				snippet = store.AddLineNumbers(snippet, 1)
			}
		}
		fmt.Fprintf(&sb, "%s,%s,%s,%s,%s,%s\n",
			escapeCSV("#"+r.Docid()),
			fmt.Sprintf("%.4f", r.Score),
			escapeCSV(r.DisplayPath),
			escapeCSV(r.Title),
			escapeCSV(r.Context),
			escapeCSV(snippet))
	}
	return sb.String()
}

func searchResultsMD(results []store.SearchResult, opts Options) string {
	var sb strings.Builder
	for _, r := range results {
		heading := r.Title
		if heading == "" {
			heading = r.DisplayPath
		}
		var content string
		if opts.Full && r.Body != "" {
			content = r.Body
		} else if r.Body != "" {
			content, _ = store.ExtractSnippet(r.Body, opts.Query, 500)
		}
		if opts.LineNumbers && content != "" {
			content = store.AddLineNumbers(content, 1)
		}
		sb.WriteString("---\n")
		fmt.Fprintf(&sb, "# %s\n\n", heading)
		fmt.Fprintf(&sb, "**docid:** `#%s`\n", r.Docid())
		if r.Context != "" {
			fmt.Fprintf(&sb, "**context:** %s\n", r.Context)
		}
		sb.WriteString("\n")
		if content != "" {
			sb.WriteString(content)
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

func searchResultsXML(results []store.SearchResult, opts Options) string {
	var sb strings.Builder
	for _, r := range results {
		titleAttr := ""
		if r.Title != "" {
			titleAttr = fmt.Sprintf(` title="%s"`, escapeXML(r.Title))
		}
		ctxAttr := ""
		if r.Context != "" {
			ctxAttr = fmt.Sprintf(` context="%s"`, escapeXML(r.Context))
		}
		var content string
		if opts.Full && r.Body != "" {
			content = r.Body
		} else if r.Body != "" {
			content, _ = store.ExtractSnippet(r.Body, opts.Query, 500)
		}
		if opts.LineNumbers && content != "" {
			content = store.AddLineNumbers(content, 1)
		}
		fmt.Fprintf(&sb, `<file docid="#%s" name="%s"%s%s>`+"\n",
			r.Docid(), escapeXML(r.DisplayPath), titleAttr, ctxAttr)
		sb.WriteString(escapeXML(content))
		sb.WriteString("\n</file>\n\n")
	}
	return sb.String()
}

func searchResultsFiles(results []store.SearchResult) string {
	var sb strings.Builder
	for _, r := range results {
		ctx := ""
		if r.Context != "" {
			ctx = fmt.Sprintf(",\"%s\"", strings.ReplaceAll(r.Context, "\"", "\"\""))
		}
		fmt.Fprintf(&sb, "#%s,%.2f,%s%s\n", r.Docid(), r.Score, r.DisplayPath, ctx)
	}
	return sb.String()
}

// --- Single Document ---

func documentJSON(doc *store.DocumentResult, opts Options) string {
	type entry struct {
		File       string `json:"file"`
		Title      string `json:"title"`
		Context    string `json:"context,omitempty"`
		Hash       string `json:"hash"`
		Docid      string `json:"docid"`
		ModifiedAt string `json:"modified_at"`
		BodyLength int    `json:"body_length"`
		Body       string `json:"body,omitempty"`
	}
	e := entry{
		File:       doc.DisplayPath,
		Title:      doc.Title,
		Context:    doc.Context,
		Hash:       doc.Hash,
		Docid:      "#" + doc.Docid(),
		ModifiedAt: doc.ModifiedAt,
		BodyLength: doc.BodyLength,
	}
	if doc.Body != "" {
		body := doc.Body
		if opts.LineNumbers {
			body = store.AddLineNumbers(body, 1)
		}
		e.Body = body
	}
	data, _ := json.MarshalIndent(e, "", "  ")
	return string(data)
}

func documentMD(doc *store.DocumentResult, opts Options) string {
	var sb strings.Builder
	title := doc.Title
	if title == "" {
		title = doc.DisplayPath
	}
	fmt.Fprintf(&sb, "# %s\n\n", title)
	if doc.Context != "" {
		fmt.Fprintf(&sb, "**Context:** %s\n\n", doc.Context)
	}
	fmt.Fprintf(&sb, "**File:** %s\n", doc.DisplayPath)
	fmt.Fprintf(&sb, "**Modified:** %s\n\n", doc.ModifiedAt)
	if doc.Body != "" {
		sb.WriteString("---\n\n")
		body := doc.Body
		if opts.LineNumbers {
			body = store.AddLineNumbers(body, 1)
		}
		sb.WriteString(body)
		sb.WriteString("\n")
	}
	return sb.String()
}

func documentXML(doc *store.DocumentResult, opts Options) string {
	var sb strings.Builder
	sb.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<document>\n")
	fmt.Fprintf(&sb, "  <file>%s</file>\n", escapeXML(doc.DisplayPath))
	fmt.Fprintf(&sb, "  <title>%s</title>\n", escapeXML(doc.Title))
	if doc.Context != "" {
		fmt.Fprintf(&sb, "  <context>%s</context>\n", escapeXML(doc.Context))
	}
	fmt.Fprintf(&sb, "  <hash>%s</hash>\n", escapeXML(doc.Hash))
	fmt.Fprintf(&sb, "  <modifiedAt>%s</modifiedAt>\n", escapeXML(doc.ModifiedAt))
	fmt.Fprintf(&sb, "  <bodyLength>%d</bodyLength>\n", doc.BodyLength)
	if doc.Body != "" {
		body := doc.Body
		if opts.LineNumbers {
			body = store.AddLineNumbers(body, 1)
		}
		fmt.Fprintf(&sb, "  <body>%s</body>\n", escapeXML(body))
	}
	sb.WriteString("</document>")
	return sb.String()
}

func documentCSV(doc *store.DocumentResult, opts Options) string {
	body := doc.Body
	if opts.LineNumbers && body != "" {
		body = store.AddLineNumbers(body, 1)
	}
	return fmt.Sprintf("file,title,context,hash,modified_at,body\n%s,%s,%s,%s,%s,%s\n",
		escapeCSV(doc.DisplayPath),
		escapeCSV(doc.Title),
		escapeCSV(doc.Context),
		escapeCSV(doc.Hash),
		escapeCSV(doc.ModifiedAt),
		escapeCSV(body))
}

// --- Multiple Documents ---

func documentsJSON(docs []store.DocumentResult, opts Options) string {
	type entry struct {
		File    string `json:"file"`
		Title   string `json:"title"`
		Context string `json:"context,omitempty"`
		Body    string `json:"body,omitempty"`
	}
	entries := make([]entry, len(docs))
	for i, d := range docs {
		body := d.Body
		if opts.LineNumbers && body != "" {
			body = store.AddLineNumbers(body, 1)
		}
		entries[i] = entry{
			File:    d.DisplayPath,
			Title:   d.Title,
			Context: d.Context,
			Body:    body,
		}
	}
	data, _ := json.MarshalIndent(entries, "", "  ")
	return string(data)
}

func documentsMD(docs []store.DocumentResult, opts Options) string {
	var sb strings.Builder
	for _, d := range docs {
		fmt.Fprintf(&sb, "## %s\n\n", d.DisplayPath)
		if d.Title != "" && d.Title != d.DisplayPath {
			fmt.Fprintf(&sb, "**Title:** %s\n\n", d.Title)
		}
		if d.Context != "" {
			fmt.Fprintf(&sb, "**Context:** %s\n\n", d.Context)
		}
		body := d.Body
		if opts.LineNumbers && body != "" {
			body = store.AddLineNumbers(body, 1)
		}
		sb.WriteString("```\n")
		sb.WriteString(body)
		sb.WriteString("\n```\n\n")
	}
	return sb.String()
}

func documentsXML(docs []store.DocumentResult, opts Options) string {
	var sb strings.Builder
	sb.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<documents>\n")
	for _, d := range docs {
		sb.WriteString("  <document>\n")
		fmt.Fprintf(&sb, "    <file>%s</file>\n", escapeXML(d.DisplayPath))
		fmt.Fprintf(&sb, "    <title>%s</title>\n", escapeXML(d.Title))
		if d.Context != "" {
			fmt.Fprintf(&sb, "    <context>%s</context>\n", escapeXML(d.Context))
		}
		body := d.Body
		if opts.LineNumbers && body != "" {
			body = store.AddLineNumbers(body, 1)
		}
		fmt.Fprintf(&sb, "    <body>%s</body>\n", escapeXML(body))
		sb.WriteString("  </document>\n")
	}
	sb.WriteString("</documents>")
	return sb.String()
}

func documentsCSV(docs []store.DocumentResult, opts Options) string {
	var sb strings.Builder
	sb.WriteString("file,title,context,body\n")
	for _, d := range docs {
		body := d.Body
		if opts.LineNumbers && body != "" {
			body = store.AddLineNumbers(body, 1)
		}
		fmt.Fprintf(&sb, "%s,%s,%s,%s\n",
			escapeCSV(d.DisplayPath),
			escapeCSV(d.Title),
			escapeCSV(d.Context),
			escapeCSV(body))
	}
	return sb.String()
}

func documentsFiles(docs []store.DocumentResult) string {
	var sb strings.Builder
	for _, d := range docs {
		ctx := ""
		if d.Context != "" {
			ctx = fmt.Sprintf(",\"%s\"", strings.ReplaceAll(d.Context, "\"", "\"\""))
		}
		fmt.Fprintf(&sb, "%s%s\n", d.DisplayPath, ctx)
	}
	return sb.String()
}

// --- File List ---

func fileListJSON(docs []store.DocumentResult) string {
	type entry struct {
		File       string `json:"file"`
		Title      string `json:"title"`
		Docid      string `json:"docid"`
		ModifiedAt string `json:"modified_at"`
		BodyLength int    `json:"body_length"`
	}
	entries := make([]entry, len(docs))
	for i, d := range docs {
		entries[i] = entry{
			File:       d.DisplayPath,
			Title:      d.Title,
			Docid:      "#" + d.Docid(),
			ModifiedAt: d.ModifiedAt,
			BodyLength: d.BodyLength,
		}
	}
	data, _ := json.MarshalIndent(entries, "", "  ")
	return string(data)
}

func fileListCSV(docs []store.DocumentResult) string {
	var sb strings.Builder
	sb.WriteString("docid,file,title,modified_at,body_length\n")
	for _, d := range docs {
		fmt.Fprintf(&sb, "#%s,%s,%s,%s,%d\n",
			d.Docid(),
			escapeCSV(d.DisplayPath),
			escapeCSV(d.Title),
			d.ModifiedAt,
			d.BodyLength)
	}
	return sb.String()
}

// --- Status ---

func statusJSON(st *store.StoreStatus) string {
	data, _ := json.MarshalIndent(st, "", "  ")
	return string(data)
}

// --- Helpers ---

func escapeCSV(value string) string {
	if value == "" {
		return ""
	}
	if strings.ContainsAny(value, ",\"\n") {
		return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
	}
	return value
}

func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	return s
}

func round2(f float64) float64 {
	return float64(int(f*100+0.5)) / 100
}
