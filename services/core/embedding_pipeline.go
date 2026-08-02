// embedding_pipeline.go implements SPEC-0039: the Embedding Pipeline.
// EmbeddingPipeline is the pipeline responsible for converting raw source
// content into searchable embeddings stored as SPEC-0034 MemoryRecords: it
// chunks content, generates an embedding per chunk via an Embedder
// (SPEC-0038's interface - HashEmbedder by default, or a model-backed
// Embedder such as OllamaEmbedder), attaches source/chunk metadata, and
// submits each chunk to a Memory. SPEC-0039's own "embedding generation"
// step is independent of whatever a downstream MemoryStorageProvider (e.g.
// VectorStore) recomputes internally for its own indexing - this pipeline's
// computed vector is attached to the record's metadata so it's available to
// callers without depending on which storage engine backs Memory.
package core

import (
	"context"
	"strings"

	"jarvis-pa/packages/errors"
)

// defaultChunkWords and defaultChunkOverlapWords size FixedSizeChunker when
// none is specified.
const (
	defaultChunkWords        = 200
	defaultChunkOverlapWords = 0
)

// SourceType classifies where an EmbeddingInput's content came from, per
// SPEC-0039's four required sources.
type SourceType string

const (
	SourceConversation SourceType = "conversation"
	SourceDocument     SourceType = "document"
	SourceNote         SourceType = "note"
	SourceCode         SourceType = "code"
)

// IsValid reports whether s is one of the source types SPEC-0039 defines.
func (s SourceType) IsValid() bool {
	switch s {
	case SourceConversation, SourceDocument, SourceNote, SourceCode:
		return true
	default:
		return false
	}
}

// memoryType maps s to the SPEC-0034 MemoryType its chunks are stored
// under. Conversations map to MemoryTypeConversation, matching SPEC-0036's
// own use of that type; documents/notes/code all map to MemoryTypeKnowledge,
// the general-content MemoryType SPEC-0034 defines but no concrete feature
// has used until now.
func (s SourceType) memoryType() MemoryType {
	if s == SourceConversation {
		return MemoryTypeConversation
	}
	return MemoryTypeKnowledge
}

// EmbeddingInput is the input to EmbeddingPipeline.Process: the source kind
// and raw content to chunk and embed, plus any caller-supplied metadata to
// attach to every resulting chunk (e.g. a document title or file path).
type EmbeddingInput struct {
	Source   SourceType
	Content  string
	Metadata map[string]any
}

// Validate reports whether in has the minimum fields EmbeddingPipeline needs
// to process it: a known Source and non-empty Content. It returns a
// packages/errors error typed TypeInvalidInput naming the first missing or
// invalid field, or nil if in is valid.
func (in EmbeddingInput) Validate() error {
	if in.Source == "" {
		return errors.New(errors.TypeInvalidInput, "EMBEDDING_INPUT_MISSING_SOURCE", "core.embeddingpipeline",
			"embedding input is missing a Source")
	}
	if !in.Source.IsValid() {
		return errors.New(errors.TypeInvalidInput, "EMBEDDING_INPUT_INVALID_SOURCE", "core.embeddingpipeline",
			"embedding input has an unknown Source").With("source", string(in.Source))
	}
	if in.Content == "" {
		return errors.New(errors.TypeInvalidInput, "EMBEDDING_INPUT_MISSING_CONTENT", "core.embeddingpipeline",
			"embedding input is missing Content").With("source", string(in.Source))
	}
	return nil
}

// Chunk is one piece of text FixedSizeChunker (or any Chunker) splits
// content into, along with its position among its siblings.
type Chunk struct {
	Text  string
	Index int
}

// Chunker splits text into ordered Chunks for separate embedding.
type Chunker interface {
	Chunk(text string) []Chunk
}

// FixedSizeChunker is a dependency-free default Chunker: it splits text
// into fixed-size, whitespace-delimited word windows, optionally
// overlapping consecutive windows by Overlap words so meaning that spans a
// chunk boundary isn't lost entirely to either side.
type FixedSizeChunker struct {
	// Size is the number of words per chunk. Non-positive falls back to
	// defaultChunkWords.
	Size int
	// Overlap is the number of trailing words from one chunk repeated at
	// the start of the next. Must be less than Size to make progress;
	// invalid values (negative, or >= Size) fall back to no overlap.
	Overlap int
}

// NewFixedSizeChunker creates a FixedSizeChunker using defaultChunkWords and
// no overlap.
func NewFixedSizeChunker() *FixedSizeChunker {
	return &FixedSizeChunker{Size: defaultChunkWords, Overlap: defaultChunkOverlapWords}
}

// Chunk implements Chunker.
func (c *FixedSizeChunker) Chunk(text string) []Chunk {
	size := c.Size
	if size <= 0 {
		size = defaultChunkWords
	}
	overlap := c.Overlap
	if overlap < 0 || overlap >= size {
		overlap = 0
	}

	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}

	step := size - overlap
	var chunks []Chunk
	for start := 0; start < len(words); start += step {
		end := start + size
		if end > len(words) {
			end = len(words)
		}
		chunks = append(chunks, Chunk{
			Text:  strings.Join(words[start:end], " "),
			Index: len(chunks),
		})
		if end == len(words) {
			break
		}
	}
	return chunks
}

// metadata keys EmbeddingPipeline encodes onto a MemoryRecord.Metadata map,
// alongside the caller's own metadata. Only EmbeddingPipeline reads or
// writes these keys; they are not part of the Memory contract.
const (
	metaSource     = "source"
	metaChunkIndex = "chunkIndex"
	metaChunkCount = "chunkCount"
	metaEmbedding  = "embedding"
)

// EmbeddingRecord is one embedded, stored chunk resulting from
// EmbeddingPipeline.Process: its assigned Memory ID, position, text,
// generated embedding vector, and the full metadata attached to its
// MemoryRecord.
type EmbeddingRecord struct {
	ID         string
	Source     SourceType
	ChunkIndex int
	ChunkCount int
	Text       string
	Embedding  []float64
	Metadata   map[string]any
}

// EmbeddingPipeline implements SPEC-0039: chunking source content,
// generating an embedding per chunk, attaching metadata, and submitting
// each chunk to a Memory. It is safe for concurrent use so long as its
// configured Chunker, Embedder, and Memory are.
type EmbeddingPipeline struct {
	chunker  Chunker
	embedder Embedder
	memory   Memory
}

// EmbeddingPipelineOption configures an EmbeddingPipeline created by
// NewEmbeddingPipeline.
type EmbeddingPipelineOption func(*EmbeddingPipeline)

// WithChunker overrides the Chunker an EmbeddingPipeline uses to split
// content. The default is a FixedSizeChunker.
func WithChunker(c Chunker) EmbeddingPipelineOption {
	return func(p *EmbeddingPipeline) { p.chunker = c }
}

// WithPipelineEmbedder overrides the Embedder an EmbeddingPipeline uses to
// generate each chunk's embedding. The default is a HashEmbedder. Named
// distinctly from VectorStore's WithEmbedder (a different option type) to
// avoid a same-package naming collision.
func WithPipelineEmbedder(e Embedder) EmbeddingPipelineOption {
	return func(p *EmbeddingPipeline) { p.embedder = e }
}

// NewEmbeddingPipeline creates an EmbeddingPipeline backed by memory,
// defaulting to a FixedSizeChunker and a HashEmbedder unless overridden.
// memory must not be nil.
func NewEmbeddingPipeline(memory Memory, opts ...EmbeddingPipelineOption) *EmbeddingPipeline {
	p := &EmbeddingPipeline{
		chunker:  NewFixedSizeChunker(),
		embedder: NewHashEmbedder(),
		memory:   memory,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Process validates in, splits its Content into chunks, generates an
// embedding for each chunk, attaches source/chunk-position metadata plus
// in.Metadata, and stores each chunk as its own MemoryRecord. It returns one
// EmbeddingRecord per stored chunk, in chunk order.
func (p *EmbeddingPipeline) Process(ctx context.Context, in EmbeddingInput) ([]EmbeddingRecord, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}

	chunks := p.chunker.Chunk(in.Content)
	if len(chunks) == 0 {
		return nil, errors.New(errors.TypeInvalidInput, "EMBEDDING_INPUT_NO_CHUNKS", "core.embeddingpipeline",
			"embedding input produced no chunks").With("source", string(in.Source))
	}

	records := make([]EmbeddingRecord, 0, len(chunks))
	for _, chunk := range chunks {
		embedding := p.embedder.Embed(chunk.Text)

		metadata := make(map[string]any, len(in.Metadata)+4)
		for k, v := range in.Metadata {
			metadata[k] = v
		}
		metadata[metaSource] = string(in.Source)
		metadata[metaChunkIndex] = chunk.Index
		metadata[metaChunkCount] = len(chunks)
		metadata[metaEmbedding] = embedding

		id, err := p.memory.Store(ctx, MemoryRecord{
			Type:     in.Source.memoryType(),
			Content:  chunk.Text,
			Metadata: metadata,
		})
		if err != nil {
			return nil, err
		}

		records = append(records, EmbeddingRecord{
			ID:         id,
			Source:     in.Source,
			ChunkIndex: chunk.Index,
			ChunkCount: len(chunks),
			Text:       chunk.Text,
			Embedding:  embedding,
			Metadata:   metadata,
		})
	}

	return records, nil
}
