// memory_embedding.go implements the embedding side of SPEC-0038 (Vector
// Memory Engine): Embedder is the abstraction VectorStore uses to turn a
// MemoryRecord's Content into a vector for similarity search, and
// HashEmbedder is a deterministic, dependency-free default implementation
// (the hashing trick) so vector similarity works without a model. SPEC-0039
// (Embedding Pipeline) is expected to supply a model-backed Embedder behind
// this same interface.
package core

import (
	"hash/fnv"
	"math"
	"strings"
)

// defaultEmbeddingDims is the vector length HashEmbedder uses when none is
// specified.
const defaultEmbeddingDims = 256

// Embedder converts text into a fixed-length embedding vector for similarity
// search.
type Embedder interface {
	Embed(text string) []float64
}

// HashEmbedder is a deterministic Embedder: it hashes each word of the input
// into one of Dims buckets (the hashing trick) and accumulates a
// term-frequency vector, so text sharing more words produces vectors with
// higher cosine similarity - without requiring an external embedding model.
type HashEmbedder struct {
	Dims int
}

// NewHashEmbedder creates a HashEmbedder using defaultEmbeddingDims.
func NewHashEmbedder() *HashEmbedder {
	return &HashEmbedder{Dims: defaultEmbeddingDims}
}

// Embed implements Embedder.
func (e *HashEmbedder) Embed(text string) []float64 {
	dims := e.Dims
	if dims <= 0 {
		dims = defaultEmbeddingDims
	}
	vec := make([]float64, dims)
	for _, word := range strings.Fields(strings.ToLower(text)) {
		vec[hashBucket(word, dims)]++
	}
	return vec
}

// hashBucket deterministically maps word into [0, dims).
func hashBucket(word string, dims int) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(word))
	return int(h.Sum32() % uint32(dims))
}

// cosineSimilarity returns the cosine similarity of a and b, in [-1, 1] for
// non-zero vectors, or 0 if either vector has zero magnitude (an empty or
// all-zero embedding has no defined direction).
func cosineSimilarity(a, b []float64) float64 {
	var dot, normA, normB float64
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}
