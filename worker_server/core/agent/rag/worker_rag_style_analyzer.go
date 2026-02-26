// Package rag implements RAG (Retrieval Augmented Generation) components.
package rag

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	"worker_server/core/port/out"

	"github.com/google/uuid"
)

// =============================================================================
// Writing Style Analyzer
//
// 발송 이메일을 분석하여 사용자의 말투, 작문 스타일, 자주 사용하는 문구 등을 학습
// Neo4j에 그래프 형태로 저장하여 자동완성 및 AI 응답에 활용
// =============================================================================

// StyleAnalyzer analyzes user's writing style from sent emails.
type StyleAnalyzer struct {
	embedder    *Embedder
	personStore out.ExtendedPersonalizationStore
	vectorStore *VectorStore
}

// NewStyleAnalyzer creates a new style analyzer.
func NewStyleAnalyzer(
	embedder *Embedder,
	personStore out.ExtendedPersonalizationStore,
	vectorStore *VectorStore,
) *StyleAnalyzer {
	return &StyleAnalyzer{
		embedder:    embedder,
		personStore: personStore,
		vectorStore: vectorStore,
	}
}

// AnalysisInput represents input for style analysis.
type AnalysisInput struct {
	UserID         uuid.UUID
	EmailID        int64
	Subject        string
	Body           string
	RecipientEmail string
	RecipientName  string
	SentAt         time.Time
	IsReply        bool
	ThreadID       string
}

// StyleAnalysisResult represents the result of style analysis.
type StyleAnalysisResult struct {
	// Writing metrics
	AvgSentenceLength int     `json:"avg_sentence_length"`
	FormalityScore    float64 `json:"formality_score"`
	EmojiFrequency    float64 `json:"emoji_frequency"`

	// Detected patterns
	Greetings   []string `json:"greetings,omitempty"`
	Closings    []string `json:"closings,omitempty"`
	Transitions []string `json:"transitions,omitempty"`

	// Detected phrases
	FrequentPhrases []string `json:"frequent_phrases,omitempty"`

	// Estimated tone
	ToneEstimate string `json:"tone_estimate"` // formal, casual, friendly, professional
}

// AnalyzeSentEmail analyzes a sent email and updates user profile.
func (a *StyleAnalyzer) AnalyzeSentEmail(ctx context.Context, input *AnalysisInput) (*StyleAnalysisResult, error) {
	if input.Body == "" {
		return nil, nil
	}

	userID := input.UserID.String()
	result := &StyleAnalysisResult{}

	// 1. Analyze writing metrics
	result.AvgSentenceLength = calculateAvgSentenceLength(input.Body)
	result.FormalityScore = calculateFormalityScore(input.Body)
	result.EmojiFrequency = calculateEmojiFrequency(input.Body)
	result.ToneEstimate = estimateTone(result.FormalityScore, input.Body)

	// 2. Extract patterns (greetings, closings, transitions)
	result.Greetings = extractGreetings(input.Body)
	result.Closings = extractClosings(input.Body)
	result.Transitions = extractTransitions(input.Body)

	// 3. Extract frequent phrases
	result.FrequentPhrases = extractFrequentPhrases(input.Body)

	// 4. Update Neo4j - Writing Style
	if err := a.updateWritingStyle(ctx, userID, result); err != nil {
		return result, fmt.Errorf("failed to update writing style: %w", err)
	}

	// 5. Update Neo4j - Contact Relationship
	if input.RecipientEmail != "" {
		if err := a.updateContactRelationship(ctx, userID, input); err != nil {
			// Non-fatal error
			fmt.Printf("warning: failed to update contact relationship: %v\n", err)
		}
	}

	// 6. Update Neo4j - Communication Patterns
	if err := a.updateCommunicationPatterns(ctx, userID, result); err != nil {
		// Non-fatal error
		fmt.Printf("warning: failed to update communication patterns: %v\n", err)
	}

	// 7. Update Neo4j - Frequent Phrases
	if err := a.updateFrequentPhrases(ctx, userID, result.FrequentPhrases); err != nil {
		// Non-fatal error
		fmt.Printf("warning: failed to update frequent phrases: %v\n", err)
	}

	return result, nil
}

// updateWritingStyle updates the user's writing style in Neo4j.
func (a *StyleAnalyzer) updateWritingStyle(ctx context.Context, userID string, result *StyleAnalysisResult) error {
	// Get existing style
	existing, _ := a.personStore.GetWritingStyle(ctx, userID)

	var newStyle *out.WritingStyle
	if existing != nil {
		// Incremental update with weighted average
		count := existing.SampleCount + 1
		newStyle = &out.WritingStyle{
			AvgSentenceLength: (existing.AvgSentenceLength*existing.SampleCount + result.AvgSentenceLength) / count,
			FormalityScore:    (existing.FormalityScore*float64(existing.SampleCount) + result.FormalityScore) / float64(count),
			EmojiFrequency:    (existing.EmojiFrequency*float64(existing.SampleCount) + result.EmojiFrequency) / float64(count),
			SampleCount:       count,
			UpdatedAt:         time.Now(),
		}
	} else {
		newStyle = &out.WritingStyle{
			AvgSentenceLength: result.AvgSentenceLength,
			FormalityScore:    result.FormalityScore,
			EmojiFrequency:    result.EmojiFrequency,
			SampleCount:       1,
			UpdatedAt:         time.Now(),
		}
	}

	return a.personStore.UpdateWritingStyle(ctx, userID, newStyle)
}

// updateContactRelationship updates the relationship with a contact.
// Tracks relationship changes over time (e.g., colleague -> boss after promotion)
func (a *StyleAnalyzer) updateContactRelationship(ctx context.Context, userID string, input *AnalysisInput) error {
	// Get existing relationship
	existing, _ := a.personStore.GetContactRelationship(ctx, userID, input.RecipientEmail)

	currentFormality := calculateFormalityScore(input.Body)
	currentTone := estimateTone(currentFormality, input.Body)
	inferredRelation := inferRelationType(input.RecipientEmail, input.Body)

	var rel *out.ContactRelationship
	if existing != nil {
		// Update existing relationship
		rel = existing
		rel.EmailsSent++
		rel.LastContact = input.SentAt
		rel.LastActivityDate = input.SentAt
		rel.IsActive = true
		rel.InactivityDays = 0

		// Recalculate importance score (frequency + recency)
		totalEmails := float64(rel.EmailsSent + rel.EmailsReceived)
		recency := 1.0 / (1.0 + time.Since(rel.LastContact).Hours()/24/30) // decay over months
		rel.ImportanceScore = (totalEmails/100.0)*0.5 + recency*0.5
		if rel.ImportanceScore > 1.0 {
			rel.ImportanceScore = 1.0
		}

		// Mark as frequent if > 10 emails
		rel.IsFrequent = totalEmails > 10

		// === Detect relationship type change ===
		if inferredRelation != "" && inferredRelation != rel.RelationType && inferredRelation != "colleague" {
			// Check if this is a significant change (confidence threshold)
			changeConfidence := calculateRelationChangeConfidence(rel.RelationType, inferredRelation, input.Body)
			if changeConfidence > 0.7 {
				// Record the change in history
				change := out.RelationChange{
					FromType:   rel.RelationType,
					ToType:     inferredRelation,
					ChangedAt:  input.SentAt,
					Confidence: changeConfidence,
					Reason:     "inferred",
				}
				rel.RelationHistory = append(rel.RelationHistory, change)
				rel.RelationType = inferredRelation
			}
		}

		// === Track formality/tone evolution ===
		if rel.FormalityLevel > 0 {
			// Calculate formality trend
			formalityDiff := currentFormality - rel.FormalityLevel
			if formalityDiff > 0.1 {
				rel.FormalityTrend = "increasing"
			} else if formalityDiff < -0.1 {
				rel.FormalityTrend = "decreasing"
			} else {
				rel.FormalityTrend = "stable"
			}

			// Exponential moving average for formality (smooths out noise)
			alpha := 0.3 // weight for new value
			rel.FormalityLevel = alpha*currentFormality + (1-alpha)*rel.FormalityLevel

			// Track tone change rate
			if formalityDiff < 0 {
				rel.ToneChangeRate = -formalityDiff
			} else {
				rel.ToneChangeRate = formalityDiff
			}
		} else {
			rel.FormalityLevel = currentFormality
			rel.FormalityTrend = "stable"
		}

		rel.ToneUsed = currentTone

	} else {
		// Create new relationship
		rel = &out.ContactRelationship{
			ContactEmail:     input.RecipientEmail,
			ContactName:      input.RecipientName,
			RelationType:     inferredRelation,
			RelationHistory:  []out.RelationChange{},
			EmailsSent:       1,
			EmailsReceived:   0,
			FirstContact:     input.SentAt,
			LastContact:      input.SentAt,
			ToneUsed:         currentTone,
			FormalityLevel:   currentFormality,
			FormalityTrend:   "stable",
			ImportanceScore:  0.1,
			IsFrequent:       false,
			IsImportant:      false,
			IsActive:         true,
			LastActivityDate: input.SentAt,
		}
	}

	return a.personStore.UpsertContactRelationship(ctx, userID, rel)
}

// calculateRelationChangeConfidence calculates confidence score for relationship type change.
func calculateRelationChangeConfidence(oldType, newType, body string) float64 {
	bodyLower := strings.ToLower(body)
	confidence := 0.5 // base confidence

	// High confidence indicators for specific transitions
	transitionIndicators := map[string]map[string][]string{
		"colleague": {
			"boss":   {"promoted", "new manager", "팀장님", "부장님", "승진", "congratulations on"},
			"client": {"new account", "partnership", "계약", "proposal"},
		},
		"client": {
			"colleague": {"joined", "welcome to the team", "입사", "합류", "new role"},
		},
		"boss": {
			"colleague": {"stepping down", "new role", "이동", "전배", "transition"},
		},
		"vendor": {
			"colleague": {"joined our team", "welcome aboard", "합류"},
		},
	}

	if indicators, ok := transitionIndicators[oldType]; ok {
		if keywords, ok := indicators[newType]; ok {
			for _, kw := range keywords {
				if strings.Contains(bodyLower, kw) {
					confidence += 0.2
				}
			}
		}
	}

	// Check for explicit role mentions in signature or body
	signatureIndicators := map[string][]string{
		"boss":        {"manager", "director", "head of", "vp", "ceo", "cto", "팀장", "부장", "이사", "대표", "chief"},
		"client":      {"account manager", "sales", "business development"},
		"colleague":   {"team", "department", "팀원"},
		"subordinate": {"intern", "junior", "assistant", "인턴", "사원", "신입"},
	}

	if keywords, ok := signatureIndicators[newType]; ok {
		for _, kw := range keywords {
			if strings.Contains(bodyLower, kw) {
				confidence += 0.1
			}
		}
	}

	// Cap at 1.0
	if confidence > 1.0 {
		confidence = 1.0
	}

	return confidence
}

// updateCommunicationPatterns updates detected communication patterns.
func (a *StyleAnalyzer) updateCommunicationPatterns(ctx context.Context, userID string, result *StyleAnalysisResult) error {
	now := time.Now()

	// Update greeting patterns
	for _, greeting := range result.Greetings {
		pattern := &out.CommunicationPattern{
			PatternID:   generatePatternID(userID, "greeting", greeting),
			UserID:      userID,
			PatternType: "greeting",
			Text:        greeting,
			Context:     determineContext(result.FormalityScore),
			UsageCount:  1,
			LastUsed:    now,
			Confidence:  0.8,
		}
		if err := a.personStore.UpsertCommunicationPattern(ctx, userID, pattern); err != nil {
			return err
		}
	}

	// Update closing patterns
	for _, closing := range result.Closings {
		pattern := &out.CommunicationPattern{
			PatternID:   generatePatternID(userID, "closing", closing),
			UserID:      userID,
			PatternType: "closing",
			Text:        closing,
			Context:     determineContext(result.FormalityScore),
			UsageCount:  1,
			LastUsed:    now,
			Confidence:  0.8,
		}
		if err := a.personStore.UpsertCommunicationPattern(ctx, userID, pattern); err != nil {
			return err
		}
	}

	return nil
}

// updateFrequentPhrases updates frequently used phrases.
func (a *StyleAnalyzer) updateFrequentPhrases(ctx context.Context, userID string, phrases []string) error {
	for _, phrase := range phrases {
		if len(phrase) < 5 {
			continue
		}

		p := &out.FrequentPhrase{
			Text:     phrase,
			Count:    1,
			Category: categorizePhrase(phrase),
			LastUsed: time.Now(),
		}
		if err := a.personStore.AddPhrase(ctx, userID, p); err != nil {
			return err
		}
	}
	return nil
}

// =============================================================================
// Analysis Helper Functions
// =============================================================================

func calculateAvgSentenceLength(text string) int {
	sentences := splitSentences(text)
	if len(sentences) == 0 {
		return 0
	}

	totalWords := 0
	for _, s := range sentences {
		totalWords += len(strings.Fields(s))
	}

	return totalWords / len(sentences)
}

func splitSentences(text string) []string {
	// Simple sentence splitting
	re := regexp.MustCompile(`[.!?]+\s+`)
	sentences := re.Split(text, -1)

	var result []string
	for _, s := range sentences {
		s = strings.TrimSpace(s)
		if len(s) > 0 {
			result = append(result, s)
		}
	}
	return result
}

func calculateFormalityScore(text string) float64 {
	text = strings.ToLower(text)
	score := 0.5 // neutral

	// Formal indicators
	formalWords := []string{
		"respectfully", "sincerely", "regards", "dear", "please",
		"kindly", "would", "could", "appreciate", "regarding",
		"attached", "pursuant", "hereby", "therefore", "however",
		"드립니다", "감사합니다", "부탁드립니다", "말씀", "여쭙",
	}

	// Informal indicators
	informalWords := []string{
		"hey", "hi", "yeah", "yep", "gonna", "wanna", "gotta",
		"thanks", "cool", "awesome", "great", "ok", "okay",
		"ㅎㅎ", "ㅋㅋ", "ㅠㅠ", "ㅜㅜ", "~", "!", "요ㅎ",
	}

	for _, word := range formalWords {
		if strings.Contains(text, word) {
			score += 0.05
		}
	}

	for _, word := range informalWords {
		if strings.Contains(text, word) {
			score -= 0.05
		}
	}

	// Clamp to 0-1
	if score < 0 {
		score = 0
	}
	if score > 1 {
		score = 1
	}

	return score
}

func calculateEmojiFrequency(text string) float64 {
	emojiCount := 0
	totalChars := 0

	for _, r := range text {
		if unicode.IsGraphic(r) {
			totalChars++
			// Check for emoji ranges
			if isEmoji(r) {
				emojiCount++
			}
		}
	}

	// Also count Korean emoticons
	emoticons := []string{"ㅎㅎ", "ㅋㅋ", "ㅠㅠ", "ㅜㅜ", "^^", ":)", ":(", ";)", ":D"}
	for _, e := range emoticons {
		emojiCount += strings.Count(text, e)
	}

	if totalChars == 0 {
		return 0
	}

	freq := float64(emojiCount) / float64(totalChars) * 100
	if freq > 1 {
		freq = 1
	}
	return freq
}

func isEmoji(r rune) bool {
	return (r >= 0x1F600 && r <= 0x1F64F) || // Emoticons
		(r >= 0x1F300 && r <= 0x1F5FF) || // Misc Symbols and Pictographs
		(r >= 0x1F680 && r <= 0x1F6FF) || // Transport and Map
		(r >= 0x1F700 && r <= 0x1F77F) || // Alchemical Symbols
		(r >= 0x1F780 && r <= 0x1F7FF) || // Geometric Shapes Extended
		(r >= 0x1F800 && r <= 0x1F8FF) || // Supplemental Arrows-C
		(r >= 0x1F900 && r <= 0x1F9FF) || // Supplemental Symbols and Pictographs
		(r >= 0x1FA00 && r <= 0x1FA6F) || // Chess Symbols
		(r >= 0x1FA70 && r <= 0x1FAFF) || // Symbols and Pictographs Extended-A
		(r >= 0x2600 && r <= 0x26FF) || // Misc symbols
		(r >= 0x2700 && r <= 0x27BF) // Dingbats
}

func estimateTone(formalityScore float64, text string) string {
	if formalityScore >= 0.7 {
		return "formal"
	} else if formalityScore >= 0.5 {
		// Check for friendly indicators
		friendlyWords := []string{"hope", "looking forward", "great", "happy", "기대", "좋은", "감사"}
		for _, w := range friendlyWords {
			if strings.Contains(strings.ToLower(text), w) {
				return "friendly"
			}
		}
		return "professional"
	} else {
		return "casual"
	}
}

func extractGreetings(text string) []string {
	lines := strings.Split(text, "\n")
	var greetings []string

	// Check first few lines for greetings
	for i := 0; i < min(3, len(lines)); i++ {
		line := strings.TrimSpace(lines[i])
		if len(line) == 0 {
			continue
		}

		// Common greeting patterns
		greetingPatterns := []string{
			"dear", "hi", "hello", "good morning", "good afternoon", "good evening",
			"안녕하세요", "안녕하십니까", "안녕", "좋은 아침", "OOO님", "님,",
		}

		lineLower := strings.ToLower(line)
		for _, pattern := range greetingPatterns {
			if strings.Contains(lineLower, pattern) || strings.HasPrefix(lineLower, pattern) {
				greetings = append(greetings, line)
				break
			}
		}
	}

	return greetings
}

func extractClosings(text string) []string {
	lines := strings.Split(text, "\n")
	var closings []string

	// Check last few lines for closings
	for i := max(0, len(lines)-5); i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if len(line) == 0 {
			continue
		}

		// Common closing patterns
		closingPatterns := []string{
			"best regards", "regards", "sincerely", "thanks", "thank you",
			"best", "cheers", "yours", "warm regards",
			"감사합니다", "감사드립니다", "수고하세요", "좋은 하루", "드림",
		}

		lineLower := strings.ToLower(line)
		for _, pattern := range closingPatterns {
			if strings.Contains(lineLower, pattern) {
				closings = append(closings, line)
				break
			}
		}
	}

	return closings
}

func extractTransitions(text string) []string {
	// Common transition phrases
	transitionPatterns := []string{
		"however", "therefore", "additionally", "furthermore", "moreover",
		"in addition", "on the other hand", "as a result", "consequently",
		"그러나", "따라서", "또한", "그리고", "하지만", "그래서", "이에",
	}

	var transitions []string
	textLower := strings.ToLower(text)

	for _, pattern := range transitionPatterns {
		if strings.Contains(textLower, pattern) {
			transitions = append(transitions, pattern)
		}
	}

	return transitions
}

func extractFrequentPhrases(text string) []string {
	// Simple n-gram extraction (2-4 words)
	words := strings.Fields(text)
	phraseCount := make(map[string]int)

	for n := 2; n <= 4; n++ {
		for i := 0; i <= len(words)-n; i++ {
			phrase := strings.Join(words[i:i+n], " ")
			phrase = strings.ToLower(phrase)
			// Filter out very short or very long phrases
			if len(phrase) >= 8 && len(phrase) <= 50 {
				phraseCount[phrase]++
			}
		}
	}

	// Return phrases that appear more than once or are significant
	var phrases []string
	for phrase, count := range phraseCount {
		if count >= 1 && isSignificantPhrase(phrase) {
			phrases = append(phrases, phrase)
		}
	}

	// Limit to top 5
	if len(phrases) > 5 {
		phrases = phrases[:5]
	}

	return phrases
}

func isSignificantPhrase(phrase string) bool {
	// Filter out common stop-word phrases
	stopPhrases := []string{
		"the", "and", "this", "that", "with", "for", "are", "was", "were",
		"is", "be", "to", "of", "in", "it", "you", "i",
	}

	phraseLower := strings.ToLower(phrase)
	for _, stop := range stopPhrases {
		if strings.HasPrefix(phraseLower, stop+" ") || strings.HasSuffix(phraseLower, " "+stop) {
			return false
		}
	}

	return true
}

func inferRelationType(email, body string) string {
	emailLower := strings.ToLower(email)
	bodyLower := strings.ToLower(body)

	// Infer from email domain
	if strings.Contains(emailLower, "client") || strings.Contains(emailLower, "customer") {
		return "client"
	}
	if strings.Contains(emailLower, "vendor") || strings.Contains(emailLower, "supplier") {
		return "vendor"
	}

	// Infer from body content
	if strings.Contains(bodyLower, "boss") || strings.Contains(bodyLower, "manager") ||
		strings.Contains(bodyLower, "팀장") || strings.Contains(bodyLower, "부장") {
		return "boss"
	}

	// Default to colleague
	return "colleague"
}

func determineContext(formalityScore float64) string {
	if formalityScore >= 0.7 {
		return "formal"
	} else if formalityScore >= 0.4 {
		return "general"
	}
	return "casual"
}

func generatePatternID(userID, patternType, text string) string {
	// Simple hash-like ID
	hash := 0
	for _, c := range text {
		hash = hash*31 + int(c)
	}
	if hash < 0 {
		hash = -hash
	}
	return fmt.Sprintf("%s_%s_%d", userID[:8], patternType, hash%100000)
}

func categorizePhrase(phrase string) string {
	phraseLower := strings.ToLower(phrase)

	// Categorize based on content
	if strings.Contains(phraseLower, "thank") || strings.Contains(phraseLower, "감사") {
		return "gratitude"
	}
	if strings.Contains(phraseLower, "please") || strings.Contains(phraseLower, "부탁") {
		return "request"
	}
	if strings.Contains(phraseLower, "sorry") || strings.Contains(phraseLower, "죄송") {
		return "apology"
	}
	if strings.Contains(phraseLower, "question") || strings.Contains(phraseLower, "문의") {
		return "inquiry"
	}

	return "general"
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
