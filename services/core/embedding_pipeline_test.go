// embedding_pipeline_test.go implements SPEC-0039 tests for
// EmbeddingPipeline: text is processed (chunking), embeddings are
// generated, and metadata is preserved (SPEC-0039's three testing
// criteria), plus FixedSizeChunker/SourceType/EmbeddingInput coverage.
package core

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"jarvis-pa/packages/errors"
)

func TestSourceType_IsValid(t *testing.T) {
	tests := []struct {
		name string
		s    SourceType
		want bool
	}{
		{"conversation", SourceConversation, true},
		{"document", SourceDocument, true},
		{"note", SourceNote, true},
		{"code", SourceCode, true},
		{"empty", SourceType(""), false},
		{"unknown", SourceType("bogus"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.s.IsValid(); got != tt.want {
				t.Errorf("IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSourceType_MemoryType(t *testing.T) {
	tests := []struct {
		s    SourceType
		want MemoryType
	}{
		{SourceConversation, MemoryTypeConversation},
		{SourceDocument, MemoryTypeKnowledge},
		{SourceNote, MemoryTypeKnowledge},
		{SourceCode, MemoryTypeKnowledge},
	}
	for _, tt := range tests {
		t.Run(string(tt.s), func(t *testing.T) {
			if got := tt.s.memoryType(); got != tt.want {
				t.Errorf("memoryType() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEmbeddingInput_Validate(t *testing.T) {
	tests := []struct {
		name     string
		in       EmbeddingInput
		wantCode string
	}{
		{"valid", EmbeddingInput{Source: SourceNote, Content: "hello"}, ""},
		{"missing source", EmbeddingInput{Content: "hello"}, "EMBEDDING_INPUT_MISSING_SOURCE"},
		{"invalid source", EmbeddingInput{Source: SourceType("bogus"), Content: "hello"}, "EMBEDDING_INPUT_INVALID_SOURCE"},
		{"missing content", EmbeddingInput{Source: SourceNote}, "EMBEDDING_INPUT_MISSING_CONTENT"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.in.Validate()
			if tt.wantCode == "" {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate() = nil, want error code %s", tt.wantCode)
			}
			if !errors.HasCode(err, tt.wantCode) {
				t.Errorf("Validate() error code = %v, want %s", err, tt.wantCode)
			}
		})
	}
}

func words(n int) string {
	w := make([]string, n)
	for i := range w {
		w[i] = "w"
	}
	return strings.Join(w, " ")
}

func TestFixedSizeChunker_Chunk_Empty(t *testing.T) {
	c := NewFixedSizeChunker()
	if got := c.Chunk(""); got != nil {
		t.Fatalf("Chunk(\"\") = %v, want nil", got)
	}
	if got := c.Chunk("   "); got != nil {
		t.Fatalf("Chunk(whitespace) = %v, want nil", got)
	}
}

func TestFixedSizeChunker_Chunk_ShorterThanSize(t *testing.T) {
	c := &FixedSizeChunker{Size: 10}
	chunks := c.Chunk("one two three")
	if len(chunks) != 1 {
		t.Fatalf("len(chunks) = %d, want 1", len(chunks))
	}
	if chunks[0].Text != "one two three" || chunks[0].Index != 0 {
		t.Errorf("chunks[0] = %+v, want {one two three 0}", chunks[0])
	}
}

func TestFixedSizeChunker_Chunk_NoOverlap(t *testing.T) {
	c := &FixedSizeChunker{Size: 3}
	chunks := c.Chunk(words(7))
	if len(chunks) != 3 {
		t.Fatalf("len(chunks) = %d, want 3", len(chunks))
	}
	for i, ch := range chunks {
		if ch.Index != i {
			t.Errorf("chunks[%d].Index = %d, want %d", i, ch.Index, i)
		}
	}
	wantWords := []int{3, 3, 1}
	for i, ch := range chunks {
		got := len(strings.Fields(ch.Text))
		if got != wantWords[i] {
			t.Errorf("chunks[%d] has %d words, want %d", i, got, wantWords[i])
		}
	}
}

func TestFixedSizeChunker_Chunk_Overlap(t *testing.T) {
	c := &FixedSizeChunker{Size: 4, Overlap: 2}
	text := "a b c d e f"
	chunks := c.Chunk(text)
	if len(chunks) != 2 {
		t.Fatalf("len(chunks) = %d, want 2: %+v", len(chunks), chunks)
	}
	if chunks[0].Text != "a b c d" {
		t.Errorf("chunks[0].Text = %q, want %q", chunks[0].Text, "a b c d")
	}
	if chunks[1].Text != "c d e f" {
		t.Errorf("chunks[1].Text = %q, want %q", chunks[1].Text, "c d e f")
	}
}

func TestFixedSizeChunker_Chunk_InvalidOverlapFallsBackToNone(t *testing.T) {
	c := &FixedSizeChunker{Size: 3, Overlap: 3}
	chunks := c.Chunk(words(6))
	if len(chunks) != 2 {
		t.Fatalf("len(chunks) = %d, want 2 (invalid overlap should be ignored)", len(chunks))
	}
}

func TestNewFixedSizeChunker_Defaults(t *testing.T) {
	c := NewFixedSizeChunker()
	if c.Size != defaultChunkWords {
		t.Errorf("Size = %d, want %d", c.Size, defaultChunkWords)
	}
	if c.Overlap != defaultChunkOverlapWords {
		t.Errorf("Overlap = %d, want %d", c.Overlap, defaultChunkOverlapWords)
	}
}

func TestNewEmbeddingPipeline_Defaults(t *testing.T) {
	p := NewEmbeddingPipeline(newStubMemory())
	if _, ok := p.chunker.(*FixedSizeChunker); !ok {
		t.Errorf("default chunker = %T, want *FixedSizeChunker", p.chunker)
	}
	if _, ok := p.embedder.(*HashEmbedder); !ok {
		t.Errorf("default embedder = %T, want *HashEmbedder", p.embedder)
	}
}

func TestEmbeddingPipeline_Process_InvalidInput(t *testing.T) {
	p := NewEmbeddingPipeline(newStubMemory())
	_, err := p.Process(context.Background(), EmbeddingInput{Content: "hello"})
	if !errors.HasCode(err, "EMBEDDING_INPUT_MISSING_SOURCE") {
		t.Fatalf("Process() error = %v, want EMBEDDING_INPUT_MISSING_SOURCE", err)
	}
}

func TestEmbeddingPipeline_Process_StoreError(t *testing.T) {
	mem := newStubMemory()
	mem.err = errors.New(errors.TypeUnavailable, "MEMORY_UNAVAILABLE", "core.test", "boom")
	p := NewEmbeddingPipeline(mem)

	_, err := p.Process(context.Background(), EmbeddingInput{Source: SourceNote, Content: "hello world"})
	if !errors.HasCode(err, "MEMORY_UNAVAILABLE") {
		t.Fatalf("Process() error = %v, want MEMORY_UNAVAILABLE to propagate", err)
	}
}

// TestEmbeddingPipeline_Process_TextIsProcessed covers SPEC-0039's "text is
// processed" testing criterion: content longer than one chunk is actually
// split, in order, and every chunk is submitted to Memory.
func TestEmbeddingPipeline_Process_TextIsProcessed(t *testing.T) {
	mem := newStubMemory()
	p := NewEmbeddingPipeline(mem, WithChunker(&FixedSizeChunker{Size: 3}))

	records, err := p.Process(context.Background(), EmbeddingInput{
		Source:  SourceDocument,
		Content: words(7),
	})
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("len(records) = %d, want 3", len(records))
	}
	for i, rec := range records {
		if rec.ChunkIndex != i {
			t.Errorf("records[%d].ChunkIndex = %d, want %d", i, rec.ChunkIndex, i)
		}
		if rec.ChunkCount != 3 {
			t.Errorf("records[%d].ChunkCount = %d, want 3", i, rec.ChunkCount)
		}
		stored, err := mem.Retrieve(context.Background(), rec.ID)
		if err != nil {
			t.Fatalf("Retrieve(%s) error = %v", rec.ID, err)
		}
		if stored.Content != rec.Text {
			t.Errorf("stored.Content = %q, want %q", stored.Content, rec.Text)
		}
	}
}

// TestEmbeddingPipeline_Process_EmbeddingsAreGenerated covers SPEC-0039's
// "embeddings are generated" testing criterion.
func TestEmbeddingPipeline_Process_EmbeddingsAreGenerated(t *testing.T) {
	mem := newStubMemory()
	p := NewEmbeddingPipeline(mem, WithChunker(&FixedSizeChunker{Size: 2}))

	records, err := p.Process(context.Background(), EmbeddingInput{
		Source:  SourceCode,
		Content: "alpha beta gamma delta",
	})
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("len(records) = %d, want 2", len(records))
	}
	if len(records[0].Embedding) == 0 {
		t.Fatal("records[0].Embedding is empty, want a generated vector")
	}
	if reflect.DeepEqual(records[0].Embedding, records[1].Embedding) {
		t.Error("distinct chunk content produced identical embeddings")
	}

	stored, err := mem.Retrieve(context.Background(), records[0].ID)
	if err != nil {
		t.Fatalf("Retrieve() error = %v", err)
	}
	gotEmbedding, ok := stored.Metadata[metaEmbedding].([]float64)
	if !ok {
		t.Fatalf("stored metadata[%q] = %v, want []float64", metaEmbedding, stored.Metadata[metaEmbedding])
	}
	if !reflect.DeepEqual(gotEmbedding, records[0].Embedding) {
		t.Errorf("stored embedding = %v, want %v", gotEmbedding, records[0].Embedding)
	}
}

// TestEmbeddingPipeline_Process_MetadataIsPreserved covers SPEC-0039's
// "metadata is preserved" testing criterion: caller-supplied metadata
// round-trips alongside the pipeline's own source/chunk metadata.
func TestEmbeddingPipeline_Process_MetadataIsPreserved(t *testing.T) {
	mem := newStubMemory()
	p := NewEmbeddingPipeline(mem)

	records, err := p.Process(context.Background(), EmbeddingInput{
		Source:  SourceDocument,
		Content: "a short document",
		Metadata: map[string]any{
			"title": "My Document",
			"path":  "/docs/my-document.md",
		},
	})
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("len(records) = %d, want 1", len(records))
	}

	stored, err := mem.Retrieve(context.Background(), records[0].ID)
	if err != nil {
		t.Fatalf("Retrieve() error = %v", err)
	}

	if stored.Metadata["title"] != "My Document" {
		t.Errorf("metadata[title] = %v, want %q", stored.Metadata["title"], "My Document")
	}
	if stored.Metadata["path"] != "/docs/my-document.md" {
		t.Errorf("metadata[path] = %v, want %q", stored.Metadata["path"], "/docs/my-document.md")
	}
	if stored.Metadata[metaSource] != string(SourceDocument) {
		t.Errorf("metadata[%q] = %v, want %q", metaSource, stored.Metadata[metaSource], SourceDocument)
	}
	if stored.Metadata[metaChunkIndex] != 0 {
		t.Errorf("metadata[%q] = %v, want 0", metaChunkIndex, stored.Metadata[metaChunkIndex])
	}
	if stored.Metadata[metaChunkCount] != 1 {
		t.Errorf("metadata[%q] = %v, want 1", metaChunkCount, stored.Metadata[metaChunkCount])
	}
}

// TestEmbeddingPipeline_Process_AllSources covers SPEC-0039's four required
// sources, verifying each routes to the expected SPEC-0034 MemoryType.
func TestEmbeddingPipeline_Process_AllSources(t *testing.T) {
	tests := []struct {
		source SourceType
		want   MemoryType
	}{
		{SourceConversation, MemoryTypeConversation},
		{SourceDocument, MemoryTypeKnowledge},
		{SourceNote, MemoryTypeKnowledge},
		{SourceCode, MemoryTypeKnowledge},
	}
	for _, tt := range tests {
		t.Run(string(tt.source), func(t *testing.T) {
			mem := newStubMemory()
			p := NewEmbeddingPipeline(mem)

			records, err := p.Process(context.Background(), EmbeddingInput{
				Source:  tt.source,
				Content: "some content to embed",
			})
			if err != nil {
				t.Fatalf("Process() error = %v", err)
			}
			stored, err := mem.Retrieve(context.Background(), records[0].ID)
			if err != nil {
				t.Fatalf("Retrieve() error = %v", err)
			}
			if stored.Type != tt.want {
				t.Errorf("stored.Type = %v, want %v", stored.Type, tt.want)
			}
		})
	}
}

func TestEmbeddingPipeline_Process_WithPipelineEmbedder(t *testing.T) {
	mem := newStubMemory()
	p := NewEmbeddingPipeline(mem, WithPipelineEmbedder(&HashEmbedder{Dims: 8}))

	records, err := p.Process(context.Background(), EmbeddingInput{Source: SourceNote, Content: "hello"})
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	if len(records[0].Embedding) != 8 {
		t.Errorf("len(Embedding) = %d, want 8", len(records[0].Embedding))
	}
}
