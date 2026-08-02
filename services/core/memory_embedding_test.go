package core

import "testing"

// TestHashEmbedder_EmbedIsDeterministic is SPEC-0038's "Memories are
// embedded" testing criterion: embedding the same content twice yields the
// same vector.
func TestHashEmbedder_EmbedIsDeterministic(t *testing.T) {
	e := NewHashEmbedder()
	a := e.Embed("jarvis memory storage")
	b := e.Embed("jarvis memory storage")
	if len(a) != len(b) {
		t.Fatalf("Embed() lengths differ: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("Embed() not deterministic at index %d: %v vs %v", i, a[i], b[i])
		}
	}
}

func TestHashEmbedder_EmbedDimensions(t *testing.T) {
	e := &HashEmbedder{Dims: 8}
	vec := e.Embed("some content here")
	if len(vec) != 8 {
		t.Errorf("Embed() length = %d, want 8 (configured Dims)", len(vec))
	}
}

// TestHashEmbedder_ZeroDimsDefaultsToDefaultEmbeddingDims covers the
// zero-value HashEmbedder{} (Dims unset), which must fall back to
// defaultEmbeddingDims rather than producing an empty or panicking vector.
func TestHashEmbedder_ZeroDimsDefaultsToDefaultEmbeddingDims(t *testing.T) {
	e := &HashEmbedder{}
	vec := e.Embed("some content here")
	if len(vec) != defaultEmbeddingDims {
		t.Errorf("Embed() length = %d, want %d (default Dims)", len(vec), defaultEmbeddingDims)
	}
}

func TestHashEmbedder_EmptyTextIsZeroVector(t *testing.T) {
	e := NewHashEmbedder()
	vec := e.Embed("")
	for i, v := range vec {
		if v != 0 {
			t.Fatalf("Embed(\"\")[%d] = %v, want 0", i, v)
		}
	}
}

func TestHashEmbedder_DifferentContentProducesDifferentVectors(t *testing.T) {
	e := NewHashEmbedder()
	a := e.Embed("alpha beta gamma")
	b := e.Embed("completely unrelated words here")
	if cosineSimilarity(a, b) >= 1 {
		t.Errorf("cosineSimilarity() of unrelated content = %v, want < 1", cosineSimilarity(a, b))
	}
}

func TestCosineSimilarity_IdenticalVectorsIsOne(t *testing.T) {
	v := []float64{1, 2, 3}
	if got := cosineSimilarity(v, v); got < 0.999999 || got > 1.000001 {
		t.Errorf("cosineSimilarity(v, v) = %v, want ~1", got)
	}
}

func TestCosineSimilarity_OrthogonalVectorsIsZero(t *testing.T) {
	a := []float64{1, 0}
	b := []float64{0, 1}
	if got := cosineSimilarity(a, b); got != 0 {
		t.Errorf("cosineSimilarity(orthogonal) = %v, want 0", got)
	}
}

func TestCosineSimilarity_ZeroVectorIsZeroNotNaN(t *testing.T) {
	zero := []float64{0, 0, 0}
	other := []float64{1, 2, 3}
	if got := cosineSimilarity(zero, other); got != 0 {
		t.Errorf("cosineSimilarity(zero vector) = %v, want 0", got)
	}
}
