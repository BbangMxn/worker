package search

// StrategyPlanner decides which search sources to use based on query analysis.
type StrategyPlanner struct {
	transformer *QueryTransformer
}

// NewStrategyPlanner creates a new strategy planner.
func NewStrategyPlanner(transformer *QueryTransformer) *StrategyPlanner {
	return &StrategyPlanner{
		transformer: transformer,
	}
}

// Plan creates a search execution plan based on the parsed query and strategy.
func (p *StrategyPlanner) Plan(parsed *ParsedQuery, req *SearchRequest) *SearchPlan {
	plan := &SearchPlan{
		Phase1TimeoutMs: 100,  // Fast phase: DB + Vector
		Phase2TimeoutMs: 2000, // Slow phase: Provider
		MergeStrategy:   MergeRRF,
	}

	// Determine which sources to use based on strategy
	switch req.Strategy {
	case StrategyFast:
		// DB only - fastest response
		plan.UseDB = true
		plan.UseVector = false
		plan.UseProvider = false
		plan.MergeStrategy = MergeDedup

	case StrategySemantic:
		// Vector only - semantic search
		plan.UseDB = false
		plan.UseVector = true
		plan.UseProvider = false
		plan.MergeStrategy = MergeDedup

	case StrategyProvider:
		// Provider API only
		plan.UseDB = false
		plan.UseVector = false
		plan.UseProvider = true
		plan.MergeStrategy = MergeDedup

	case StrategyComplete:
		// All sources
		plan.UseDB = true
		plan.UseVector = true
		plan.UseProvider = true
		plan.MergeStrategy = MergeRRF

	case StrategyBalanced:
		fallthrough
	default:
		// Balanced: DB + Vector, Provider as fallback
		p.planBalanced(plan, parsed)
	}

	// Build source-specific queries
	p.buildQueries(plan, parsed, req)

	return plan
}

// planBalanced determines sources for balanced strategy based on intent.
func (p *StrategyPlanner) planBalanced(plan *SearchPlan, parsed *ParsedQuery) {
	switch parsed.Intent {
	case IntentKeyword:
		// Simple keyword: DB is enough, Vector optional
		plan.UseDB = true
		plan.UseVector = false
		plan.UseProvider = false // fallback if needed

	case IntentSemantic:
		// Natural language: Vector primary, DB secondary
		plan.UseDB = true
		plan.UseVector = true
		plan.UseProvider = false // fallback if needed

	case IntentStructured:
		// Structured filters: DB + Provider (for accurate filter matching)
		plan.UseDB = true
		plan.UseVector = false
		plan.UseProvider = true // Provider handles filters better

	case IntentHybrid:
		// Mixed: Use all available
		plan.UseDB = true
		plan.UseVector = true
		plan.UseProvider = false // fallback if needed

	default:
		plan.UseDB = true
		plan.UseVector = false
		plan.UseProvider = false
	}

	// Complex queries benefit from more sources
	if parsed.Complexity >= 3 {
		plan.UseProvider = true
	}
}

// buildQueries creates source-specific queries.
func (p *StrategyPlanner) buildQueries(plan *SearchPlan, parsed *ParsedQuery, req *SearchRequest) {
	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}

	// DB Query
	if plan.UseDB {
		plan.DBQuery = &DBSearchQuery{
			Query:   p.transformer.ToDBQuery(parsed),
			Filters: req.Filters,
			Limit:   limit,
			Offset:  req.Offset,
			UseRank: len(parsed.Keywords) > 0,
		}
	}

	// Vector Query
	if plan.UseVector {
		plan.VectorQuery = &VectorSearchQuery{
			Text:     parsed.SemanticQuery,
			Filters:  req.Filters,
			Limit:    limit,
			MinScore: 0.5, // Minimum similarity threshold
		}
	}

	// Provider Query
	if plan.UseProvider {
		plan.ProviderQuery = p.transformer.BuildProviderSearchQuery(parsed, limit)
	}
}

// ShouldFallbackToProvider determines if Provider search is needed as fallback.
func (p *StrategyPlanner) ShouldFallbackToProvider(plan *SearchPlan, dbCount, vectorCount, limit int) bool {
	// Already using provider
	if plan.UseProvider {
		return false
	}

	// Results insufficient
	totalResults := dbCount + vectorCount
	if totalResults < limit/2 {
		return true
	}

	return false
}

// AdjustPlanForFallback modifies plan to include Provider search.
func (p *StrategyPlanner) AdjustPlanForFallback(plan *SearchPlan, parsed *ParsedQuery, limit int) {
	plan.UseProvider = true
	if plan.ProviderQuery == nil {
		plan.ProviderQuery = p.transformer.BuildProviderSearchQuery(parsed, limit)
	}
}
