// knowledge_ingestion_test.go implements SPEC-0040 tests for
// KnowledgeIngestionPipeline: documents ingest successfully, content is
// searchable afterward, and metadata (path/filename/format, plus
// parser-derived fields like a PDF's page count) remains attached
// (SPEC-0040's three testing criteria), across all five required source
// types: Markdown, PDF, Text, code repositories, and documentation trees.
package core

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"jarvis-pa/packages/errors"
)

func TestDetectFormat(t *testing.T) {
	tests := []struct {
		filename string
		want     IngestFormat
	}{
		{"README.md", FormatMarkdown},
		{"notes.markdown", FormatMarkdown},
		{"report.PDF", FormatPDF},
		{"notes.txt", FormatText},
		{"main.go", FormatCode},
		{"script.py", FormatCode},
		{"styles.CSS", FormatCode},
		{"image.png", ""},
		{"archive.zip", ""},
		{"noextension", ""},
	}
	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			if got := DetectFormat(tt.filename); got != tt.want {
				t.Errorf("DetectFormat(%q) = %q, want %q", tt.filename, got, tt.want)
			}
		})
	}
}

func TestIngestFormat_SourceType(t *testing.T) {
	tests := []struct {
		format IngestFormat
		want   SourceType
	}{
		{FormatMarkdown, SourceDocument},
		{FormatPDF, SourceDocument},
		{FormatText, SourceDocument},
		{FormatCode, SourceCode},
	}
	for _, tt := range tests {
		t.Run(string(tt.format), func(t *testing.T) {
			if got := tt.format.sourceType(); got != tt.want {
				t.Errorf("sourceType() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPlainTextParser_Parse(t *testing.T) {
	content, meta, err := PlainTextParser{}.Parse([]byte("# Title\n\nSome body text."))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if content != "# Title\n\nSome body text." {
		t.Errorf("content = %q, want unchanged input", content)
	}
	if meta != nil {
		t.Errorf("metadata = %v, want nil", meta)
	}
}

// buildTestPDF constructs a minimal, well-formed single-page PDF containing
// text so PDFParser can be exercised without a binary test fixture.
func buildTestPDF(text string) []byte {
	var buf bytes.Buffer
	offsets := make([]int, 6)
	buf.WriteString("%PDF-1.4\n")

	writeObj := func(num int, body string) {
		offsets[num] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", num, body)
	}

	writeObj(1, "<< /Type /Catalog /Pages 2 0 R >>")
	writeObj(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>")
	writeObj(3, "<< /Type /Page /Parent 2 0 R /Resources << /Font << /F1 4 0 R >> >> /MediaBox [0 0 300 300] /Contents 5 0 R >>")
	writeObj(4, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>")

	stream := fmt.Sprintf("BT /F1 24 Tf 10 250 Td (%s) Tj ET", text)
	writeObj(5, fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(stream), stream))

	xrefOffset := buf.Len()
	buf.WriteString("xref\n0 6\n")
	buf.WriteString("0000000000 65535 f \n")
	for i := 1; i <= 5; i++ {
		fmt.Fprintf(&buf, "%010d 00000 n \n", offsets[i])
	}
	buf.WriteString("trailer\n<< /Size 6 /Root 1 0 R >>\nstartxref\n")
	fmt.Fprintf(&buf, "%d\n", xrefOffset)
	buf.WriteString("%%EOF")

	return buf.Bytes()
}

func TestPDFParser_Parse(t *testing.T) {
	content, meta, err := PDFParser{}.Parse(buildTestPDF("Hello World"))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if content != "Hello World" {
		t.Errorf("content = %q, want %q", content, "Hello World")
	}
	if meta["pageCount"] != 1 {
		t.Errorf("metadata[pageCount] = %v, want 1", meta["pageCount"])
	}
}

func TestPDFParser_Parse_Invalid(t *testing.T) {
	_, _, err := PDFParser{}.Parse([]byte("not a pdf"))
	if !errors.HasCode(err, "KNOWLEDGE_PDF_UNREADABLE") {
		t.Fatalf("Parse() error = %v, want KNOWLEDGE_PDF_UNREADABLE", err)
	}
}

func TestNewKnowledgeIngestionPipeline_DefaultParsers(t *testing.T) {
	k := NewKnowledgeIngestionPipeline(NewEmbeddingPipeline(newStubMemory()))
	if _, ok := k.parsers[FormatMarkdown].(PlainTextParser); !ok {
		t.Errorf("markdown parser = %T, want PlainTextParser", k.parsers[FormatMarkdown])
	}
	if _, ok := k.parsers[FormatPDF].(PDFParser); !ok {
		t.Errorf("pdf parser = %T, want PDFParser", k.parsers[FormatPDF])
	}
}

type stubParser struct {
	content string
}

func (p stubParser) Parse([]byte) (string, map[string]any, error) {
	return p.content, map[string]any{"stub": true}, nil
}

func TestWithParser_Override(t *testing.T) {
	k := NewKnowledgeIngestionPipeline(NewEmbeddingPipeline(newStubMemory()),
		WithParser(FormatText, stubParser{content: "overridden"}))
	if _, ok := k.parsers[FormatText].(stubParser); !ok {
		t.Errorf("text parser = %T, want stubParser", k.parsers[FormatText])
	}
}

// TestKnowledgeIngestionPipeline_IngestFile_DocumentsIngestSuccessfully
// covers SPEC-0040's "documents ingest successfully" testing criterion
// across Markdown, Text, and Code files.
func TestKnowledgeIngestionPipeline_IngestFile_DocumentsIngestSuccessfully(t *testing.T) {
	tests := []struct {
		name       string
		filename   string
		content    string
		wantSource SourceType
	}{
		{"markdown", "notes.md", "# Notes\n\nSome markdown content.", SourceDocument},
		{"text", "notes.txt", "Some plain text content.", SourceDocument},
		{"code", "main.go", "package main\n\nfunc main() {}\n", SourceCode},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, tt.filename)
			if err := os.WriteFile(path, []byte(tt.content), 0o644); err != nil {
				t.Fatalf("WriteFile() error = %v", err)
			}

			mem := newStubMemory()
			k := NewKnowledgeIngestionPipeline(NewEmbeddingPipeline(mem))

			records, err := k.IngestFile(context.Background(), path)
			if err != nil {
				t.Fatalf("IngestFile() error = %v", err)
			}
			if len(records) == 0 {
				t.Fatal("IngestFile() produced no records")
			}

			stored, err := mem.Retrieve(context.Background(), records[0].ID)
			if err != nil {
				t.Fatalf("Retrieve() error = %v", err)
			}
			if stored.Metadata[metaSource] != string(tt.wantSource) {
				t.Errorf("metadata[%q] = %v, want %q", metaSource, stored.Metadata[metaSource], tt.wantSource)
			}
		})
	}
}

func TestKnowledgeIngestionPipeline_IngestFile_PDF(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.pdf")
	if err := os.WriteFile(path, buildTestPDF("Report Body"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	mem := newStubMemory()
	k := NewKnowledgeIngestionPipeline(NewEmbeddingPipeline(mem))

	records, err := k.IngestFile(context.Background(), path)
	if err != nil {
		t.Fatalf("IngestFile() error = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("len(records) = %d, want 1", len(records))
	}
	if records[0].Text != "Report Body" {
		t.Errorf("records[0].Text = %q, want %q", records[0].Text, "Report Body")
	}
	if records[0].Metadata["pageCount"] != 1 {
		t.Errorf("metadata[pageCount] = %v, want 1", records[0].Metadata["pageCount"])
	}
}

func TestKnowledgeIngestionPipeline_IngestFile_UnsupportedFormat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "image.png")
	if err := os.WriteFile(path, []byte{0x89, 0x50, 0x4e, 0x47}, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	k := NewKnowledgeIngestionPipeline(NewEmbeddingPipeline(newStubMemory()))
	_, err := k.IngestFile(context.Background(), path)
	if !errors.HasCode(err, "KNOWLEDGE_UNSUPPORTED_FORMAT") {
		t.Fatalf("IngestFile() error = %v, want KNOWLEDGE_UNSUPPORTED_FORMAT", err)
	}
}

func TestKnowledgeIngestionPipeline_IngestFile_MissingFile(t *testing.T) {
	k := NewKnowledgeIngestionPipeline(NewEmbeddingPipeline(newStubMemory()))
	_, err := k.IngestFile(context.Background(), filepath.Join(t.TempDir(), "missing.md"))
	if !errors.HasCode(err, "KNOWLEDGE_FILE_READ_FAILED") {
		t.Fatalf("IngestFile() error = %v, want KNOWLEDGE_FILE_READ_FAILED", err)
	}
}

func TestKnowledgeIngestionPipeline_IngestFile_EmptyContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.txt")
	if err := os.WriteFile(path, []byte("   \n\t "), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	k := NewKnowledgeIngestionPipeline(NewEmbeddingPipeline(newStubMemory()))
	_, err := k.IngestFile(context.Background(), path)
	if !errors.HasCode(err, "KNOWLEDGE_EMPTY_CONTENT") {
		t.Fatalf("IngestFile() error = %v, want KNOWLEDGE_EMPTY_CONTENT", err)
	}
}

// TestKnowledgeIngestionPipeline_IngestFile_MetadataAttached covers
// SPEC-0040's "metadata remains attached" testing criterion.
func TestKnowledgeIngestionPipeline_IngestFile_MetadataAttached(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "guide.md")
	if err := os.WriteFile(path, []byte("# Guide\n\nContent."), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	mem := newStubMemory()
	k := NewKnowledgeIngestionPipeline(NewEmbeddingPipeline(mem))

	records, err := k.IngestFile(context.Background(), path)
	if err != nil {
		t.Fatalf("IngestFile() error = %v", err)
	}

	stored, err := mem.Retrieve(context.Background(), records[0].ID)
	if err != nil {
		t.Fatalf("Retrieve() error = %v", err)
	}
	if stored.Metadata["path"] != path {
		t.Errorf("metadata[path] = %v, want %q", stored.Metadata["path"], path)
	}
	if stored.Metadata["filename"] != "guide.md" {
		t.Errorf("metadata[filename] = %v, want %q", stored.Metadata["filename"], "guide.md")
	}
	if stored.Metadata["format"] != string(FormatMarkdown) {
		t.Errorf("metadata[format] = %v, want %q", stored.Metadata["format"], FormatMarkdown)
	}
}

// TestKnowledgeIngestionPipeline_IngestFile_ContentIsSearchable covers
// SPEC-0040's "content is searchable" testing criterion: an ingested
// document is findable via Memory.Search afterward.
func TestKnowledgeIngestionPipeline_IngestFile_ContentIsSearchable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "searchable.md")
	if err := os.WriteFile(path, []byte("The quick brown fox jumps over the lazy dog."), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	mem := newStubMemory()
	k := NewKnowledgeIngestionPipeline(NewEmbeddingPipeline(mem))

	if _, err := k.IngestFile(context.Background(), path); err != nil {
		t.Fatalf("IngestFile() error = %v", err)
	}

	results, err := mem.Search(context.Background(), MemoryQuery{Type: MemoryTypeKnowledge, Query: "fox"})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if results[0].Content != "The quick brown fox jumps over the lazy dog." {
		t.Errorf("results[0].Content = %q, want ingested content", results[0].Content)
	}
}

// TestKnowledgeIngestionPipeline_IngestDirectory covers SPEC-0040's "Code
// repositories" and "Documentation" source types: walking a directory
// tree, ingesting every recognized file, skipping unsupported files and
// skipDirs entirely.
func TestKnowledgeIngestionPipeline_IngestDirectory(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"README.md":                       "# Project\n\nOverview text.",
		"main.go":                         "package main\n\nfunc main() {}\n",
		"notes.txt":                       "Plain notes.",
		"image.png":                       "\x89PNG-not-real-but-binary",
		filepath.Join("docs", "guide.md"): "# Guide\n\nDetails.",
	}
	for rel, content := range files {
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("MkdirAll() error = %v", err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
	}

	// A .git directory (with a tracked-looking file inside) must be skipped
	// entirely, even though the file itself has a recognized extension.
	gitDir := filepath.Join(root, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "config.txt"), []byte("should not be ingested"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	mem := newStubMemory()
	k := NewKnowledgeIngestionPipeline(NewEmbeddingPipeline(mem))

	records, err := k.IngestDirectory(context.Background(), root)
	if err != nil {
		t.Fatalf("IngestDirectory() error = %v", err)
	}
	// README.md, main.go, notes.txt, docs/guide.md: 4 ingested files, one
	// record each (all short enough to be a single chunk). image.png and
	// .git/config.txt must be excluded.
	if len(records) != 4 {
		t.Fatalf("len(records) = %d, want 4: %+v", len(records), records)
	}
	if len(mem.records) != 4 {
		t.Fatalf("len(mem.records) = %d, want 4 (image.png and .git contents must be skipped)", len(mem.records))
	}
	for _, rec := range mem.records {
		if rec.Metadata["path"] == filepath.Join(gitDir, "config.txt") {
			t.Error("ingested a file from a skipped .git directory")
		}
	}
}

func TestKnowledgeIngestionPipeline_IngestDirectory_PropagatesFileErrors(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "bad.pdf"), []byte("not a pdf"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	k := NewKnowledgeIngestionPipeline(NewEmbeddingPipeline(newStubMemory()))
	_, err := k.IngestDirectory(context.Background(), root)
	if !errors.HasCode(err, "KNOWLEDGE_PDF_UNREADABLE") {
		t.Fatalf("IngestDirectory() error = %v, want KNOWLEDGE_PDF_UNREADABLE", err)
	}
}
