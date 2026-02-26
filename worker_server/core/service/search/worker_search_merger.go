package search

import (
	"sort"
)

// ResultMerger merges and ranks results from multiple search sources.
type ResultMerger struct{}

// NewResultMerger creates a new result merger.
func NewResultMerger() *ResultMerger {
	return &ResultMerger{}
}

// Merge combines results from multiple sources using the specified strategy.
func (m *ResultMerger) Merge(response *SearchResponse, strategy MergeStrategy, limit int) {
	if len(response.Results) == 0 {
		return
	}

	// First, deduplicate by ProviderID
	response.Results = m.deduplicate(response.Results)

	// Then apply merge strategy
	switch strategy {
	case MergeRRF:
		m.applyRRF(response)
	case MergeScore:
		m.applyScoreMerge(response)
	case MergeDedup:
		// Already deduplicated, just sort by date
		m.sortByDate(response.Results)
	default:
		m.applyRRF(response)
	}

	// Apply limit
	if limit > 0 && len(response.Results) > limit {
		response.Results = response.Results[:limit]
		response.HasMore = true
	}

	response.Total = len(response.Results)
}

// deduplicate removes duplicate results based on ProviderID or EmailID.
func (m *ResultMerger) deduplicate(results []*SearchResult) []*SearchResult {
	seen := make(map[string]bool)
	seenIDs := make(map[int64]bool)
	unique := make([]*SearchResult, 0, len(results))

	for _, r := range results {
		// Check by ProviderID first (more reliable)
		if r.ProviderID != "" {
			if seen[r.ProviderID] {
				continue
			}
			seen[r.ProviderID] = true
		} else if r.EmailID != 0 {
			// Fallback to EmailID
			if seenIDs[r.EmailID] {
				continue
			}
			seenIDs[r.EmailID] = true
		}

		unique = append(unique, r)
	}

	return unique
}

// applyRRF applies Reciprocal Rank Fusion scoring.
// RRF score = sum(1 / (k + rank_i)) for each source
// k is typically 60 to balance between sources.
func (m *ResultMerger) applyRRF(response *SearchResponse) {
	const k = 60.0

	// Group results by source and assign ranks
	sourceRanks := make(map[SearchSource]map[string]int) // source -> providerID -> rank
	for source := range map[SearchSource]bool{SourceDB: true, SourceVector: true, SourceProvider: true} {
		sourceRanks[source] = make(map[string]int)
	}

	// Separate results by source
	bySource := make(map[SearchSource][]*SearchResult)
	for _, r := range response.Results {
		bySource[r.Source] = append(bySource[r.Source], r)
	}

	// Sort each source's results by their native score
	for source, results := range bySource {
		// Sort by score descending
		sort.Slice(results, func(i, j int) bool {
			return results[i].Score > results[j].Score
		})

		// Assign ranks
		for rank, r := range results {
			key := r.ProviderID
			if key == "" {
				key = string(rune(r.EmailID))
			}
			sourceRanks[source][key] = rank + 1 // 1-indexed
		}
	}

	// Calculate RRF scores
	for _, r := range response.Results {
		key := r.ProviderID
		if key == "" {
			key = string(rune(r.EmailID))
		}

		rrfScore := 0.0
		for source, ranks := range sourceRanks {
			if rank, ok := ranks[key]; ok {
				rrfScore += 1.0 / (k + float64(rank))
			} else {
				// Not in this source, use very low rank
				rrfScore += 1.0 / (k + 1000.0)
			}
			_ = source
		}

		r.Score = rrfScore
	}

	// Sort by RRF score descending
	sort.Slice(response.Results, func(i, j int) bool {
		return response.Results[i].Score > response.Results[j].Score
	})
}

// applyScoreMerge applies weighted score combination.
func (m *ResultMerger) applyScoreMerge(response *SearchResponse) {
	// Weights for each source
	weights := map[SearchSource]float64{
		SourceDB:       0.3,
		SourceVector:   0.5,
		SourceProvider: 0.2,
	}

	// Normalize scores within each source
	maxScores := make(map[SearchSource]float64)
	for _, r := range response.Results {
		if r.Score > maxScores[r.Source] {
			maxScores[r.Source] = r.Score
		}
	}

	// Apply weighted scores
	for _, r := range response.Results {
		normalizedScore := r.Score
		if maxScores[r.Source] > 0 {
			normalizedScore = r.Score / maxScores[r.Source]
		}
		r.Score = normalizedScore * weights[r.Source]
	}

	// Sort by weighted score descending
	sort.Slice(response.Results, func(i, j int) bool {
		return response.Results[i].Score > response.Results[j].Score
	})
}

// sortByDate sorts results by date descending.
func (m *ResultMerger) sortByDate(results []*SearchResult) {
	sort.Slice(results, func(i, j int) bool {
		return results[i].Date.After(results[j].Date)
	})
}

// BoostRecentResults gives a score boost to recent emails.
func (m *ResultMerger) BoostRecentResults(results []*SearchResult, boostFactor float64) {
	if len(results) == 0 {
		return
	}

	// Find the most recent date
	var latestDate = results[0].Date
	for _, r := range results {
		if r.Date.After(latestDate) {
			latestDate = r.Date
		}
	}

	// Apply recency boost
	for _, r := range results {
		daysDiff := latestDate.Sub(r.Date).Hours() / 24
		recencyBoost := 1.0 / (1.0 + daysDiff/30.0) // Decay over 30 days
		r.Score = r.Score * (1.0 + boostFactor*recencyBoost)
	}
}

// FilterByMinScore removes results below a minimum score threshold.
func (m *ResultMerger) FilterByMinScore(results []*SearchResult, minScore float64) []*SearchResult {
	filtered := make([]*SearchResult, 0, len(results))
	for _, r := range results {
		if r.Score >= minScore {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

// EnrichFromDB fills in missing fields from DB results.
func (m *ResultMerger) EnrichFromDB(results []*SearchResult, dbResults []*SearchResult) {
	// Create lookup map from DB results
	dbLookup := make(map[string]*SearchResult)
	for _, r := range dbResults {
		if r.ProviderID != "" {
			dbLookup[r.ProviderID] = r
		}
	}

	// Enrich other results
	for _, r := range results {
		if r.Source == SourceDB {
			continue
		}

		if dbResult, ok := dbLookup[r.ProviderID]; ok {
			// Fill in missing fields
			if r.Subject == "" {
				r.Subject = dbResult.Subject
			}
			if r.Snippet == "" {
				r.Snippet = dbResult.Snippet
			}
			if r.From == "" {
				r.From = dbResult.From
			}
			if r.Date.IsZero() {
				r.Date = dbResult.Date
			}
			if r.Folder == "" {
				r.Folder = dbResult.Folder
			}
			// Always use DB's read/attachment status as source of truth
			r.IsRead = dbResult.IsRead
			r.HasAttach = dbResult.HasAttach
			r.EmailID = dbResult.EmailID
		}
	}
}
