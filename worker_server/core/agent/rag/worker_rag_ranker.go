package rag

import (
	"sort"
	"time"
)

type Ranker struct {
	// Configuration for ranking weights
	vectorWeight    float64
	recencyWeight   float64
	relevanceWeight float64
}

func NewRanker() *Ranker {
	return &Ranker{
		vectorWeight:    0.7,
		recencyWeight:   0.2,
		relevanceWeight: 0.1,
	}
}

type RankableResult struct {
	*RetrievalResult
	RecencyScore   float64
	RelevanceScore float64
	FinalScore     float64
}

func (r *Ranker) Rank(results []*RetrievalResult) []*RankableResult {
	if len(results) == 0 {
		return nil
	}

	ranked := make([]*RankableResult, len(results))

	for i, result := range results {
		ranked[i] = &RankableResult{
			RetrievalResult: result,
			RecencyScore:    r.calculateRecencyScore(result.Metadata),
			RelevanceScore:  r.calculateRelevanceScore(result),
		}

		// Calculate final score
		ranked[i].FinalScore = r.vectorWeight*result.Score +
			r.recencyWeight*ranked[i].RecencyScore +
			r.relevanceWeight*ranked[i].RelevanceScore
	}

	// Sort by final score descending
	sort.Slice(ranked, func(i, j int) bool {
		return ranked[i].FinalScore > ranked[j].FinalScore
	})

	return ranked
}

// calculateRecencyScore calculates a score based on how recent the email is
// Returns 0.0 to 1.0, where 1.0 is most recent
func (r *Ranker) calculateRecencyScore(metadata map[string]any) float64 {
	if metadata == nil {
		return 0.5 // Default if no metadata
	}

	// Try to get date from metadata
	var emailDate time.Time

	if dateStr, ok := metadata["date"].(string); ok {
		parsed, err := time.Parse(time.RFC3339, dateStr)
		if err == nil {
			emailDate = parsed
		}
	} else if dateTime, ok := metadata["date"].(time.Time); ok {
		emailDate = dateTime
	} else if receivedAt, ok := metadata["received_at"].(string); ok {
		parsed, err := time.Parse(time.RFC3339, receivedAt)
		if err == nil {
			emailDate = parsed
		}
	}

	if emailDate.IsZero() {
		return 0.5 // Default if no date found
	}

	// Calculate days ago
	daysAgo := time.Since(emailDate).Hours() / 24

	// Decay function: score decreases as email gets older
	// - 0 days ago: 1.0
	// - 7 days ago: ~0.8
	// - 30 days ago: ~0.5
	// - 90 days ago: ~0.25
	// - 365 days ago: ~0.1
	if daysAgo <= 0 {
		return 1.0
	}

	// Exponential decay with half-life of ~30 days
	score := 1.0 / (1.0 + (daysAgo / 30.0))

	// Clamp to 0.1 minimum
	if score < 0.1 {
		return 0.1
	}
	return score
}

// calculateRelevanceScore calculates additional relevance factors
// beyond vector similarity
func (r *Ranker) calculateRelevanceScore(result *RetrievalResult) float64 {
	if result.Metadata == nil {
		return 0.5
	}

	score := 0.5 // Base score

	// Boost if from important folder (inbox > sent > others)
	if folder, ok := result.Metadata["folder"].(string); ok {
		switch folder {
		case "inbox":
			score += 0.2
		case "sent":
			score += 0.15
		case "important", "starred":
			score += 0.25
		}
	}

	// Boost if has subject match (indicates higher relevance)
	if subject, ok := result.Metadata["subject"].(string); ok {
		if len(subject) > 0 {
			score += 0.1
		}
	}

	// Boost based on priority if available
	if priority, ok := result.Metadata["priority"].(int); ok {
		switch priority {
		case 1: // Urgent
			score += 0.2
		case 2: // High
			score += 0.1
		}
	}

	// Clamp to 0.0-1.0
	if score > 1.0 {
		return 1.0
	}
	if score < 0.0 {
		return 0.0
	}
	return score
}

// ReRank applies additional ranking based on context
// Currently a placeholder for future cross-encoder implementation
func (r *Ranker) ReRank(results []*RankableResult, context string) []*RankableResult {
	// Future enhancement: Use cross-encoder model to re-rank based on context
	// For now, return as-is since vector similarity + recency/relevance scoring
	// provides good enough results for most use cases
	return results
}

// WithWeights allows customizing ranking weights
func (r *Ranker) WithWeights(vector, recency, relevance float64) *Ranker {
	total := vector + recency + relevance
	if total > 0 {
		r.vectorWeight = vector / total
		r.recencyWeight = recency / total
		r.relevanceWeight = relevance / total
	}
	return r
}
