// knowledge_ingestion.go implements SPEC-0040: the Knowledge Ingestion
// Pipeline. It supplies the Input and Parser stages SPEC-0040's own pipeline
// diagram (Input -> Parser -> Chunking -> Embedding -> Storage) puts ahead
// of SPEC-0039's EmbeddingPipeline, which already implements Chunking,
// Embedding, and Storage: KnowledgeIngestionPipeline reads a file (or an
// entire directory tree, for code repositories and documentation sets),
// parses it into plain text per its detected IngestFormat, and hands that
// text to an EmbeddingPipeline as an EmbeddingInput.
package core

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/ledongthuc/pdf"

	"jarvis-pa/packages/errors"
)

// IngestFormat classifies a file's content so KnowledgeIngestionPipeline
// knows which Parser to use, per SPEC-0040's required source types.
type IngestFormat string

const (
	FormatMarkdown IngestFormat = "markdown"
	FormatPDF      IngestFormat = "pdf"
	FormatText     IngestFormat = "text"
	FormatCode     IngestFormat = "code"
)

// codeExtensions is the allowlist of file extensions IngestDirectory treats
// as source code when walking a repository. It is deliberately an allowlist
// rather than a blacklist: a repository tree also contains binaries,
// images, and other non-text files that must never be read as UTF-8 text.
var codeExtensions = map[string]bool{
	".go": true, ".py": true, ".js": true, ".jsx": true, ".ts": true, ".tsx": true,
	".java": true, ".c": true, ".h": true, ".cpp": true, ".hpp": true, ".cs": true,
	".rb": true, ".rs": true, ".php": true, ".swift": true, ".kt": true, ".scala": true,
	".sh": true, ".ps1": true, ".sql": true, ".html": true, ".css": true, ".scss": true,
	".json": true, ".yaml": true, ".yml": true, ".toml": true, ".xml": true, ".proto": true,
}

// DetectFormat returns the IngestFormat filename's extension maps to, or ""
// if the extension is unrecognized (e.g. a binary file that should be
// skipped during directory ingestion).
func DetectFormat(filename string) IngestFormat {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".md", ".markdown":
		return FormatMarkdown
	case ".pdf":
		return FormatPDF
	case ".txt":
		return FormatText
	default:
		if codeExtensions[strings.ToLower(filepath.Ext(filename))] {
			return FormatCode
		}
		return ""
	}
}

// sourceType maps f to the SPEC-0039 SourceType its parsed content is
// embedded under: code files are tagged SourceCode; every other supported
// format (markdown, PDF, text) is tagged SourceDocument.
func (f IngestFormat) sourceType() SourceType {
	if f == FormatCode {
		return SourceCode
	}
	return SourceDocument
}

// Parser extracts plain text (and any parser-derived metadata, such as a
// PDF's page count) from a file's raw bytes.
type Parser interface {
	Parse(raw []byte) (content string, metadata map[string]any, err error)
}

// PlainTextParser is the Parser for formats whose bytes are already plain
// text with nothing to decode: Markdown, Text, and Code source files all
// use it unchanged.
type PlainTextParser struct{}

// Parse implements Parser.
func (PlainTextParser) Parse(raw []byte) (string, map[string]any, error) {
	return string(raw), nil, nil
}

// PDFParser is the Parser for PDF files: it extracts the document's text
// content via github.com/ledongthuc/pdf, a pure-Go reader with no external
// process or cgo dependency, keeping ingestion local-first per ADR-0008.
type PDFParser struct{}

// Parse implements Parser.
func (PDFParser) Parse(raw []byte) (string, map[string]any, error) {
	reader, err := pdf.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return "", nil, errors.Wrap(err, errors.TypeInvalidInput, "KNOWLEDGE_PDF_UNREADABLE", "core.knowledgeingestion",
			"pdf content could not be read")
	}

	textReader, err := reader.GetPlainText()
	if err != nil {
		return "", nil, errors.Wrap(err, errors.TypeInvalidInput, "KNOWLEDGE_PDF_TEXT_EXTRACTION_FAILED", "core.knowledgeingestion",
			"pdf text could not be extracted")
	}

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(textReader); err != nil {
		return "", nil, errors.Wrap(err, errors.TypeInvalidInput, "KNOWLEDGE_PDF_TEXT_EXTRACTION_FAILED", "core.knowledgeingestion",
			"pdf text could not be extracted")
	}

	return strings.TrimSpace(buf.String()), map[string]any{"pageCount": reader.NumPage()}, nil
}

// skipDirs names directories IngestDirectory never descends into: version
// control metadata and dependency/build trees that are large, not source
// content, and often not valid UTF-8 text.
var skipDirs = map[string]bool{
	".git": true, "node_modules": true, "vendor": true, ".venv": true, "__pycache__": true,
}

// KnowledgeIngestionPipeline implements SPEC-0040's Input and Parser
// stages: given a file path (or a directory to walk), it detects the
// file's IngestFormat, parses it into plain text, attaches file metadata,
// and hands the result to an EmbeddingPipeline for chunking, embedding, and
// storage.
type KnowledgeIngestionPipeline struct {
	embeddingPipeline *EmbeddingPipeline
	parsers           map[IngestFormat]Parser
}

// KnowledgeIngestionPipelineOption configures a KnowledgeIngestionPipeline
// created by NewKnowledgeIngestionPipeline.
type KnowledgeIngestionPipelineOption func(*KnowledgeIngestionPipeline)

// WithParser overrides the Parser a KnowledgeIngestionPipeline uses for
// format.
func WithParser(format IngestFormat, p Parser) KnowledgeIngestionPipelineOption {
	return func(k *KnowledgeIngestionPipeline) { k.parsers[format] = p }
}

// NewKnowledgeIngestionPipeline creates a KnowledgeIngestionPipeline that
// submits parsed content to embeddingPipeline, defaulting to
// PlainTextParser for Markdown/Text/Code and PDFParser for PDF unless
// overridden. embeddingPipeline must not be nil.
func NewKnowledgeIngestionPipeline(embeddingPipeline *EmbeddingPipeline, opts ...KnowledgeIngestionPipelineOption) *KnowledgeIngestionPipeline {
	k := &KnowledgeIngestionPipeline{
		embeddingPipeline: embeddingPipeline,
		parsers: map[IngestFormat]Parser{
			FormatMarkdown: PlainTextParser{},
			FormatText:     PlainTextParser{},
			FormatCode:     PlainTextParser{},
			FormatPDF:      PDFParser{},
		},
	}
	for _, opt := range opts {
		opt(k)
	}
	return k
}

// IngestFile reads path, parses it according to its detected IngestFormat,
// and submits the result to the pipeline's EmbeddingPipeline, returning one
// EmbeddingRecord per stored chunk. It returns a packages/errors error
// typed TypeInvalidInput if path's format is unsupported or its parsed
// content is empty.
func (k *KnowledgeIngestionPipeline) IngestFile(ctx context.Context, path string) ([]EmbeddingRecord, error) {
	format := DetectFormat(path)
	if format == "" {
		return nil, errors.New(errors.TypeInvalidInput, "KNOWLEDGE_UNSUPPORTED_FORMAT", "core.knowledgeingestion",
			"file has an unsupported format for ingestion").With("path", path)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Wrap(err, errors.TypeInvalidInput, "KNOWLEDGE_FILE_READ_FAILED", "core.knowledgeingestion",
			"file could not be read").With("path", path)
	}

	content, parserMeta, err := k.parsers[format].Parse(raw)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(content) == "" {
		return nil, errors.New(errors.TypeInvalidInput, "KNOWLEDGE_EMPTY_CONTENT", "core.knowledgeingestion",
			"parsed content is empty").With("path", path)
	}

	metadata := make(map[string]any, len(parserMeta)+3)
	for key, value := range parserMeta {
		metadata[key] = value
	}
	metadata["path"] = path
	metadata["filename"] = filepath.Base(path)
	metadata["format"] = string(format)

	return k.embeddingPipeline.Process(ctx, EmbeddingInput{
		Source:   format.sourceType(),
		Content:  content,
		Metadata: metadata,
	})
}

// IngestDirectory walks root recursively, calling IngestFile for every file
// whose format DetectFormat recognizes (skipping anything it doesn't, and
// skipDirs entirely), and returns the concatenation of every resulting
// EmbeddingRecord in the order files were visited. This is SPEC-0040's
// entry point for ingesting a code repository or a documentation tree
// rather than a single file.
func (k *KnowledgeIngestionPipeline) IngestDirectory(ctx context.Context, root string) ([]EmbeddingRecord, error) {
	var all []EmbeddingRecord
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if DetectFormat(path) == "" {
			return nil
		}
		records, err := k.IngestFile(ctx, path)
		if err != nil {
			return err
		}
		all = append(all, records...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return all, nil
}
