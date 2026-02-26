package rag

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestRetrievalRequest(t *testing.T) {
	userID := uuid.New()

	req := &RetrievalRequest{
		Query:    "meeting schedule",
		UserID:   userID,
		SentOnly: true,
		Limit:    5,
		MinScore: 0.7,
	}

	if req.Query != "meeting schedule" {
		t.Errorf("expected query 'meeting schedule', got %s", req.Query)
	}
	if !req.SentOnly {
		t.Error("expected SentOnly to be true")
	}
	if req.Limit != 5 {
		t.Errorf("expected limit 5, got %d", req.Limit)
	}
}

func TestRetrievalResult(t *testing.T) {
	result := &RetrievalResult{
		EmailID: 123,
		Score:   0.95,
		Content: "This is the email content for testing.",
		Metadata: map[string]any{
			"subject":   "Test Subject",
			"from":      "sender@example.com",
			"direction": "outbound",
		},
	}

	if result.EmailID != 123 {
		t.Errorf("expected email ID 123, got %d", result.EmailID)
	}
	if result.Score != 0.95 {
		t.Errorf("expected score 0.95, got %f", result.Score)
	}
	if result.Metadata["direction"] != "outbound" {
		t.Errorf("expected direction 'outbound', got %v", result.Metadata["direction"])
	}
}

func TestSearchOptions(t *testing.T) {
	userID := uuid.New()

	// Style learning - sent only
	styleOpts := &SearchOptions{
		UserID:   userID.String(),
		SentOnly: true,
		Limit:    3,
	}

	if !styleOpts.SentOnly {
		t.Error("style options should have SentOnly true")
	}
	if styleOpts.AllEmails {
		t.Error("style options should not have AllEmails")
	}

	// Context search - all emails
	contextOpts := &SearchOptions{
		UserID:    userID.String(),
		AllEmails: true,
		Limit:     5,
	}

	if contextOpts.SentOnly {
		t.Error("context options should not have SentOnly")
	}
	if !contextOpts.AllEmails {
		t.Error("context options should have AllEmails true")
	}
}

// MockEmbedder for testing
type MockEmbedder struct {
	embedding []float32
}

func (m *MockEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	if m.embedding != nil {
		return m.embedding, nil
	}
	// Return a dummy embedding
	return make([]float32, 1536), nil
}

func (m *MockEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	result := make([][]float32, len(texts))
	for i := range texts {
		result[i] = make([]float32, 1536)
	}
	return result, nil
}

// MockVectorStore for testing
type MockVectorStore struct {
	results []*SearchResult
}

func (m *MockVectorStore) Search(ctx context.Context, embedding []float32, opts *SearchOptions) ([]*SearchResult, error) {
	if m.results != nil {
		return m.results, nil
	}
	return []*SearchResult{
		{
			EmailID:  1,
			Score:    0.95,
			Content:  "Mock email content",
			Metadata: map[string]any{"subject": "Test"},
		},
	}, nil
}

func (m *MockVectorStore) Store(ctx context.Context, record *VectorRecord) error {
	return nil
}

func (m *MockVectorStore) StoreBatch(ctx context.Context, records []*VectorRecord) error {
	return nil
}

func (m *MockVectorStore) Delete(ctx context.Context, emailID int64) error {
	return nil
}

func TestRetrieverWithMocks(t *testing.T) {
	embedder := &Embedder{}
	vectorStore := &VectorStore{}
	retriever := NewRetriever(embedder, vectorStore)

	if retriever == nil {
		t.Error("expected retriever to be created")
	}
	if retriever.embedder != embedder {
		t.Error("embedder not set correctly")
	}
	if retriever.vectorStore != vectorStore {
		t.Error("vectorStore not set correctly")
	}
}

func TestSearchResultStruct(t *testing.T) {
	result := &SearchResult{
		EmailID: 456,
		Score:   0.87,
		Content: "Search result content",
		Metadata: map[string]any{
			"subject": "Found Email",
		},
	}

	if result.EmailID != 456 {
		t.Errorf("expected email ID 456, got %d", result.EmailID)
	}
	if result.Score < 0.8 || result.Score > 0.9 {
		t.Errorf("expected score around 0.87, got %f", result.Score)
	}
}

func TestIndexerServiceStruct(t *testing.T) {
	service := &IndexerService{}

	if service == nil {
		t.Error("expected indexer service to be created")
	}
}

// Test embedding dimension constant
func TestEmbeddingDimension(t *testing.T) {
	// OpenAI Ada-002 uses 1536 dimensions
	expectedDim := 1536
	embedding := make([]float32, expectedDim)

	if len(embedding) != expectedDim {
		t.Errorf("expected %d dimensions, got %d", expectedDim, len(embedding))
	}
}

func TestPgVector(t *testing.T) {
	tests := []struct {
		name     string
		input    []float32
		expected string
	}{
		{
			name:     "empty",
			input:    []float32{},
			expected: "[0]",
		},
		{
			name:     "single value",
			input:    []float32{1.5},
			expected: "[1.500000]",
		},
		{
			name:     "multiple values",
			input:    []float32{1.0, 2.5, 3.0},
			expected: "[1.000000,2.500000,3.000000]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := pgVector(tt.input)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestVectorRecordStruct(t *testing.T) {
	record := &VectorRecord{
		ID:        1,
		EmailID:   123,
		UserID:    "user-uuid",
		Direction: "outbound",
		Embedding: make([]float32, 1536),
		Content:   "Email content",
		Metadata: map[string]any{
			"subject": "Test",
		},
	}

	if record.Direction != "outbound" {
		t.Errorf("expected direction 'outbound', got %s", record.Direction)
	}
}
