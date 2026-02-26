// Package graph implements Neo4j adapters for the application.
package graph

import (
	"worker_server/core/port/out"
	"context"
	"fmt"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// =============================================================================
// Neo4j Personalization Store Adapter
// =============================================================================

// PersonalizationAdapter implements out.PersonalizationStore using Neo4j.
type PersonalizationAdapter struct {
	driver neo4j.DriverWithContext
	dbName string
}

// NewPersonalizationAdapter creates a new Neo4j personalization adapter.
func NewPersonalizationAdapter(driver neo4j.DriverWithContext, dbName string) *PersonalizationAdapter {
	return &PersonalizationAdapter{
		driver: driver,
		dbName: dbName,
	}
}

// EnsureIndexes creates necessary indexes and constraints.
func (a *PersonalizationAdapter) EnsureIndexes(ctx context.Context) error {
	session := a.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: a.dbName})
	defer session.Close(ctx)

	queries := []string{
		// User constraints and indexes
		`CREATE CONSTRAINT user_id_unique IF NOT EXISTS FOR (u:User) REQUIRE u.user_id IS UNIQUE`,
		`CREATE INDEX user_email_idx IF NOT EXISTS FOR (u:User) ON (u.email)`,

		// Trait indexes
		`CREATE INDEX trait_name_idx IF NOT EXISTS FOR (t:Trait) ON (t.name)`,

		// Phrase indexes
		`CREATE INDEX phrase_user_idx IF NOT EXISTS FOR (p:Phrase) ON (p.user_id)`,
		`CREATE INDEX phrase_text_idx IF NOT EXISTS FOR (p:Phrase) ON (p.text)`,

		// Signature indexes
		`CREATE INDEX signature_user_idx IF NOT EXISTS FOR (s:Signature) ON (s.user_id)`,

		// TonePreference indexes
		`CREATE INDEX tone_user_context_idx IF NOT EXISTS FOR (tp:TonePreference) ON (tp.user_id, tp.context)`,
	}

	for _, query := range queries {
		_, err := session.Run(ctx, query, nil)
		if err != nil {
			// Ignore if already exists
			continue
		}
	}

	return nil
}

// =============================================================================
// User Profile Operations
// =============================================================================

// GetUserProfile retrieves a user profile.
func (a *PersonalizationAdapter) GetUserProfile(ctx context.Context, userID string) (*out.UserProfile, error) {
	session := a.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: a.dbName})
	defer session.Close(ctx)

	query := `
		MATCH (u:User {user_id: $userID})
		RETURN u.email AS email, u.name AS name, u.job_title AS job_title,
			   u.company AS company, u.industry AS industry, u.timezone AS timezone
	`

	result, err := session.Run(ctx, query, map[string]interface{}{"userID": userID})
	if err != nil {
		return nil, fmt.Errorf("failed to get user profile: %w", err)
	}

	if result.Next(ctx) {
		record := result.Record()
		return &out.UserProfile{
			UserID:   userID,
			Email:    getStringValue(record, "email"),
			Name:     getStringValue(record, "name"),
			JobTitle: getStringValue(record, "job_title"),
			Company:  getStringValue(record, "company"),
			Industry: getStringValue(record, "industry"),
			Timezone: getStringValue(record, "timezone"),
		}, nil
	}

	return nil, nil
}

// UpdateUserProfile updates a user profile.
func (a *PersonalizationAdapter) UpdateUserProfile(ctx context.Context, userID string, profile *out.UserProfile) error {
	session := a.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: a.dbName})
	defer session.Close(ctx)

	query := `
		MERGE (u:User {user_id: $userID})
		SET u.email = $email,
			u.name = $name,
			u.job_title = $jobTitle,
			u.company = $company,
			u.industry = $industry,
			u.timezone = $timezone,
			u.updated_at = timestamp()
	`

	params := map[string]interface{}{
		"userID":   userID,
		"email":    profile.Email,
		"name":     profile.Name,
		"jobTitle": profile.JobTitle,
		"company":  profile.Company,
		"industry": profile.Industry,
		"timezone": profile.Timezone,
	}

	_, err := session.Run(ctx, query, params)
	if err != nil {
		return fmt.Errorf("failed to update user profile: %w", err)
	}

	return nil
}

// =============================================================================
// Trait Operations
// =============================================================================

// GetUserTraits retrieves user traits.
func (a *PersonalizationAdapter) GetUserTraits(ctx context.Context, userID string) ([]*out.UserTrait, error) {
	session := a.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: a.dbName})
	defer session.Close(ctx)

	query := `
		MATCH (u:User {user_id: $userID})-[:HAS_TRAIT]->(t:Trait)
		RETURN t.name AS name, t.score AS score
		ORDER BY t.score DESC
	`

	result, err := session.Run(ctx, query, map[string]interface{}{"userID": userID})
	if err != nil {
		return nil, fmt.Errorf("failed to get user traits: %w", err)
	}

	var traits []*out.UserTrait
	for result.Next(ctx) {
		record := result.Record()
		traits = append(traits, &out.UserTrait{
			Name:  getStringValue(record, "name"),
			Score: getFloatValue(record, "score"),
		})
	}

	return traits, nil
}

// UpdateUserTrait updates a user trait.
func (a *PersonalizationAdapter) UpdateUserTrait(ctx context.Context, userID string, trait *out.UserTrait) error {
	session := a.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: a.dbName})
	defer session.Close(ctx)

	query := `
		MERGE (u:User {user_id: $userID})
		MERGE (t:Trait {name: $name, user_id: $userID})
		SET t.score = $score,
			t.updated_at = timestamp()
		MERGE (u)-[:HAS_TRAIT]->(t)
	`

	params := map[string]interface{}{
		"userID": userID,
		"name":   trait.Name,
		"score":  trait.Score,
	}

	_, err := session.Run(ctx, query, params)
	if err != nil {
		return fmt.Errorf("failed to update user trait: %w", err)
	}

	return nil
}

// =============================================================================
// Writing Style Operations
// =============================================================================

// GetWritingStyle retrieves user writing style.
func (a *PersonalizationAdapter) GetWritingStyle(ctx context.Context, userID string) (*out.WritingStyle, error) {
	session := a.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: a.dbName})
	defer session.Close(ctx)

	query := `
		MATCH (u:User {user_id: $userID})-[:HAS_WRITING_STYLE]->(ws:WritingStyle)
		RETURN ws.embedding AS embedding, ws.avg_sentence_length AS avg_sentence_length,
			   ws.formality_score AS formality_score, ws.emoji_frequency AS emoji_frequency,
			   ws.sample_count AS sample_count, ws.updated_at AS updated_at
	`

	result, err := session.Run(ctx, query, map[string]interface{}{"userID": userID})
	if err != nil {
		return nil, fmt.Errorf("failed to get writing style: %w", err)
	}

	if result.Next(ctx) {
		record := result.Record()

		var embedding []float32
		if emb, ok := record.Get("embedding"); ok && emb != nil {
			if embArr, ok := emb.([]interface{}); ok {
				embedding = make([]float32, len(embArr))
				for i, v := range embArr {
					if f, ok := v.(float64); ok {
						embedding[i] = float32(f)
					}
				}
			}
		}

		var updatedAt time.Time
		if ts, ok := record.Get("updated_at"); ok && ts != nil {
			if tsInt, ok := ts.(int64); ok {
				updatedAt = time.Unix(tsInt/1000, 0)
			}
		}

		return &out.WritingStyle{
			Embedding:         embedding,
			AvgSentenceLength: getIntValue(record, "avg_sentence_length"),
			FormalityScore:    getFloatValue(record, "formality_score"),
			EmojiFrequency:    getFloatValue(record, "emoji_frequency"),
			SampleCount:       getIntValue(record, "sample_count"),
			UpdatedAt:         updatedAt,
		}, nil
	}

	return nil, nil
}

// UpdateWritingStyle updates user writing style.
func (a *PersonalizationAdapter) UpdateWritingStyle(ctx context.Context, userID string, style *out.WritingStyle) error {
	session := a.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: a.dbName})
	defer session.Close(ctx)

	query := `
		MERGE (u:User {user_id: $userID})
		MERGE (u)-[:HAS_WRITING_STYLE]->(ws:WritingStyle {user_id: $userID})
		SET ws.embedding = $embedding,
			ws.avg_sentence_length = $avgSentenceLength,
			ws.formality_score = $formalityScore,
			ws.emoji_frequency = $emojiFrequency,
			ws.sample_count = $sampleCount,
			ws.updated_at = timestamp()
	`

	params := map[string]interface{}{
		"userID":            userID,
		"embedding":         style.Embedding,
		"avgSentenceLength": style.AvgSentenceLength,
		"formalityScore":    style.FormalityScore,
		"emojiFrequency":    style.EmojiFrequency,
		"sampleCount":       style.SampleCount,
	}

	_, err := session.Run(ctx, query, params)
	if err != nil {
		return fmt.Errorf("failed to update writing style: %w", err)
	}

	return nil
}

// =============================================================================
// Tone Preference Operations
// =============================================================================

// GetTonePreference retrieves tone preference for a context.
func (a *PersonalizationAdapter) GetTonePreference(ctx context.Context, userID, contextType string) (*out.TonePreference, error) {
	session := a.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: a.dbName})
	defer session.Close(ctx)

	query := `
		MATCH (u:User {user_id: $userID})-[:HAS_TONE_PREF]->(tp:TonePreference {context: $context})
		RETURN tp.style AS style, tp.formality AS formality
	`

	params := map[string]interface{}{
		"userID":  userID,
		"context": contextType,
	}

	result, err := session.Run(ctx, query, params)
	if err != nil {
		return nil, fmt.Errorf("failed to get tone preference: %w", err)
	}

	if result.Next(ctx) {
		record := result.Record()
		return &out.TonePreference{
			Context:   contextType,
			Style:     getStringValue(record, "style"),
			Formality: getFloatValue(record, "formality"),
		}, nil
	}

	return nil, nil
}

// UpdateTonePreference updates tone preference.
func (a *PersonalizationAdapter) UpdateTonePreference(ctx context.Context, userID string, pref *out.TonePreference) error {
	session := a.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: a.dbName})
	defer session.Close(ctx)

	query := `
		MERGE (u:User {user_id: $userID})
		MERGE (tp:TonePreference {user_id: $userID, context: $context})
		SET tp.style = $style,
			tp.formality = $formality,
			tp.updated_at = timestamp()
		MERGE (u)-[:HAS_TONE_PREF]->(tp)
	`

	params := map[string]interface{}{
		"userID":    userID,
		"context":   pref.Context,
		"style":     pref.Style,
		"formality": pref.Formality,
	}

	_, err := session.Run(ctx, query, params)
	if err != nil {
		return fmt.Errorf("failed to update tone preference: %w", err)
	}

	return nil
}

// =============================================================================
// Frequent Phrases Operations
// =============================================================================

// GetFrequentPhrases retrieves frequently used phrases.
func (a *PersonalizationAdapter) GetFrequentPhrases(ctx context.Context, userID string, limit int) ([]*out.FrequentPhrase, error) {
	session := a.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: a.dbName})
	defer session.Close(ctx)

	query := `
		MATCH (u:User {user_id: $userID})-[:USES_PHRASE]->(p:Phrase)
		RETURN p.text AS text, p.count AS count, p.category AS category, p.last_used AS last_used
		ORDER BY p.count DESC
		LIMIT $limit
	`

	params := map[string]interface{}{
		"userID": userID,
		"limit":  limit,
	}

	result, err := session.Run(ctx, query, params)
	if err != nil {
		return nil, fmt.Errorf("failed to get frequent phrases: %w", err)
	}

	var phrases []*out.FrequentPhrase
	for result.Next(ctx) {
		record := result.Record()

		var lastUsed time.Time
		if ts, ok := record.Get("last_used"); ok && ts != nil {
			if tsInt, ok := ts.(int64); ok {
				lastUsed = time.Unix(tsInt/1000, 0)
			}
		}

		phrases = append(phrases, &out.FrequentPhrase{
			Text:     getStringValue(record, "text"),
			Count:    getIntValue(record, "count"),
			Category: getStringValue(record, "category"),
			LastUsed: lastUsed,
		})
	}

	return phrases, nil
}

// AddPhrase adds a new phrase.
func (a *PersonalizationAdapter) AddPhrase(ctx context.Context, userID string, phrase *out.FrequentPhrase) error {
	session := a.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: a.dbName})
	defer session.Close(ctx)

	query := `
		MERGE (u:User {user_id: $userID})
		MERGE (p:Phrase {user_id: $userID, text: $text})
		SET p.count = $count,
			p.category = $category,
			p.last_used = timestamp()
		MERGE (u)-[:USES_PHRASE]->(p)
	`

	params := map[string]interface{}{
		"userID":   userID,
		"text":     phrase.Text,
		"count":    phrase.Count,
		"category": phrase.Category,
	}

	_, err := session.Run(ctx, query, params)
	if err != nil {
		return fmt.Errorf("failed to add phrase: %w", err)
	}

	return nil
}

// IncrementPhraseCount increments phrase usage count.
func (a *PersonalizationAdapter) IncrementPhraseCount(ctx context.Context, userID, phraseText string) error {
	session := a.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: a.dbName})
	defer session.Close(ctx)

	query := `
		MATCH (u:User {user_id: $userID})-[:USES_PHRASE]->(p:Phrase {text: $text})
		SET p.count = p.count + 1,
			p.last_used = timestamp()
	`

	params := map[string]interface{}{
		"userID": userID,
		"text":   phraseText,
	}

	_, err := session.Run(ctx, query, params)
	if err != nil {
		return fmt.Errorf("failed to increment phrase count: %w", err)
	}

	return nil
}

// =============================================================================
// Signature Operations
// =============================================================================

// GetSignatures retrieves user signatures.
func (a *PersonalizationAdapter) GetSignatures(ctx context.Context, userID string) ([]*out.Signature, error) {
	session := a.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: a.dbName})
	defer session.Close(ctx)

	query := `
		MATCH (u:User {user_id: $userID})-[:HAS_SIGNATURE]->(s:Signature)
		RETURN s.id AS id, s.text AS text, s.is_default AS is_default
		ORDER BY s.is_default DESC
	`

	result, err := session.Run(ctx, query, map[string]interface{}{"userID": userID})
	if err != nil {
		return nil, fmt.Errorf("failed to get signatures: %w", err)
	}

	var signatures []*out.Signature
	for result.Next(ctx) {
		record := result.Record()
		signatures = append(signatures, &out.Signature{
			ID:        getStringValue(record, "id"),
			Text:      getStringValue(record, "text"),
			IsDefault: getBoolValue(record, "is_default"),
		})
	}

	return signatures, nil
}

// SetDefaultSignature sets the default signature.
func (a *PersonalizationAdapter) SetDefaultSignature(ctx context.Context, userID string, signatureID string) error {
	session := a.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: a.dbName})
	defer session.Close(ctx)

	// First, unset all defaults
	query1 := `
		MATCH (u:User {user_id: $userID})-[:HAS_SIGNATURE]->(s:Signature)
		SET s.is_default = false
	`

	_, err := session.Run(ctx, query1, map[string]interface{}{"userID": userID})
	if err != nil {
		return fmt.Errorf("failed to unset defaults: %w", err)
	}

	// Then, set the new default
	query2 := `
		MATCH (u:User {user_id: $userID})-[:HAS_SIGNATURE]->(s:Signature {id: $signatureID})
		SET s.is_default = true
	`

	params := map[string]interface{}{
		"userID":      userID,
		"signatureID": signatureID,
	}

	_, err = session.Run(ctx, query2, params)
	if err != nil {
		return fmt.Errorf("failed to set default signature: %w", err)
	}

	return nil
}

// =============================================================================
// Helper Functions
// =============================================================================
// Helper Functions (shared across graph package)
// =============================================================================

func getStringValue(record *neo4j.Record, key string) string {
	if val, ok := record.Get(key); ok && val != nil {
		if s, ok := val.(string); ok {
			return s
		}
	}
	return ""
}

func getIntValue(record *neo4j.Record, key string) int {
	if val, ok := record.Get(key); ok && val != nil {
		switch v := val.(type) {
		case int64:
			return int(v)
		case int:
			return v
		case float64:
			return int(v)
		}
	}
	return 0
}

func getBoolValue(record *neo4j.Record, key string) bool {
	if val, ok := record.Get(key); ok && val != nil {
		if b, ok := val.(bool); ok {
			return b
		}
	}
	return false
}

// =============================================================================
// Extended Profile Operations
// =============================================================================

// GetExtendedProfile retrieves comprehensive user profile.
func (a *PersonalizationAdapter) GetExtendedProfile(ctx context.Context, userID string) (*out.ExtendedUserProfile, error) {
	session := a.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: a.dbName})
	defer session.Close(ctx)

	query := `
		MATCH (u:User {user_id: $userID})
		RETURN u.email AS email, u.name AS name, u.nickname AS nickname,
			   u.age_range AS age_range, u.gender AS gender, u.location AS location,
			   u.timezone AS timezone, u.language AS language, u.languages AS languages,
			   u.job_title AS job_title, u.department AS department,
			   u.company AS company, u.industry AS industry,
			   u.seniority AS seniority, u.skills AS skills,
			   u.preferred_tone AS preferred_tone, u.response_speed AS response_speed,
			   u.preferred_length AS preferred_length, u.emoji_usage AS emoji_usage,
			   u.formality_default AS formality_default,
			   u.profile_completeness AS profile_completeness,
			   u.source_count AS source_count, u.updated_at AS updated_at
	`

	result, err := session.Run(ctx, query, map[string]interface{}{"userID": userID})
	if err != nil {
		return nil, fmt.Errorf("failed to get extended profile: %w", err)
	}

	if result.Next(ctx) {
		record := result.Record()
		profile := &out.ExtendedUserProfile{
			UserID:              userID,
			Email:               getStringValue(record, "email"),
			Name:                getStringValue(record, "name"),
			Nickname:            getStringValue(record, "nickname"),
			AgeRange:            getStringValue(record, "age_range"),
			Gender:              getStringValue(record, "gender"),
			Location:            getStringValue(record, "location"),
			Timezone:            getStringValue(record, "timezone"),
			Language:            getStringValue(record, "language"),
			JobTitle:            getStringValue(record, "job_title"),
			Department:          getStringValue(record, "department"),
			Company:             getStringValue(record, "company"),
			Industry:            getStringValue(record, "industry"),
			Seniority:           getStringValue(record, "seniority"),
			PreferredTone:       getStringValue(record, "preferred_tone"),
			ResponseSpeed:       getStringValue(record, "response_speed"),
			PreferredLength:     getStringValue(record, "preferred_length"),
			EmojiUsage:          getFloatValue(record, "emoji_usage"),
			FormalityDefault:    getFloatValue(record, "formality_default"),
			ProfileCompleteness: getFloatValue(record, "profile_completeness"),
			SourceCount:         getIntValue(record, "source_count"),
		}

		// Parse string arrays
		profile.Languages = getStringArrayValue(record, "languages")
		profile.Skills = getStringArrayValue(record, "skills")

		// Parse timestamp
		if ts, ok := record.Get("updated_at"); ok && ts != nil {
			if tsInt, ok := ts.(int64); ok {
				profile.LastUpdated = time.Unix(tsInt/1000, 0)
			}
		}

		return profile, nil
	}

	return nil, nil
}

// UpdateExtendedProfile updates comprehensive user profile.
func (a *PersonalizationAdapter) UpdateExtendedProfile(ctx context.Context, userID string, profile *out.ExtendedUserProfile) error {
	session := a.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: a.dbName})
	defer session.Close(ctx)

	query := `
		MERGE (u:User {user_id: $userID})
		SET u.email = $email,
			u.name = $name,
			u.nickname = $nickname,
			u.age_range = $ageRange,
			u.gender = $gender,
			u.location = $location,
			u.timezone = $timezone,
			u.language = $language,
			u.languages = $languages,
			u.job_title = $jobTitle,
			u.department = $department,
			u.company = $company,
			u.industry = $industry,
			u.seniority = $seniority,
			u.skills = $skills,
			u.preferred_tone = $preferredTone,
			u.response_speed = $responseSpeed,
			u.preferred_length = $preferredLength,
			u.emoji_usage = $emojiUsage,
			u.formality_default = $formalityDefault,
			u.profile_completeness = $profileCompleteness,
			u.source_count = $sourceCount,
			u.updated_at = timestamp()
	`

	params := map[string]interface{}{
		"userID":              userID,
		"email":               profile.Email,
		"name":                profile.Name,
		"nickname":            profile.Nickname,
		"ageRange":            profile.AgeRange,
		"gender":              profile.Gender,
		"location":            profile.Location,
		"timezone":            profile.Timezone,
		"language":            profile.Language,
		"languages":           profile.Languages,
		"jobTitle":            profile.JobTitle,
		"department":          profile.Department,
		"company":             profile.Company,
		"industry":            profile.Industry,
		"seniority":           profile.Seniority,
		"skills":              profile.Skills,
		"preferredTone":       profile.PreferredTone,
		"responseSpeed":       profile.ResponseSpeed,
		"preferredLength":     profile.PreferredLength,
		"emojiUsage":          profile.EmojiUsage,
		"formalityDefault":    profile.FormalityDefault,
		"profileCompleteness": profile.ProfileCompleteness,
		"sourceCount":         profile.SourceCount,
	}

	_, err := session.Run(ctx, query, params)
	if err != nil {
		return fmt.Errorf("failed to update extended profile: %w", err)
	}

	return nil
}

// =============================================================================
// Contact Relationship Operations (Graph Edges)
// =============================================================================

// GetContactRelationships retrieves all contact relationships for a user.
func (a *PersonalizationAdapter) GetContactRelationships(ctx context.Context, userID string, limit int) ([]*out.ContactRelationship, error) {
	session := a.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: a.dbName})
	defer session.Close(ctx)

	query := `
		MATCH (u:User {user_id: $userID})-[r:COMMUNICATES_WITH]->(c:Contact)
		RETURN c.email AS contact_email, c.name AS contact_name,
			   r.relation_type AS relation_type,
			   r.emails_sent AS emails_sent, r.emails_received AS emails_received,
			   r.last_contact AS last_contact, r.first_contact AS first_contact,
			   r.tone_used AS tone_used, r.formality_level AS formality_level,
			   r.avg_reply_time AS avg_reply_time,
			   r.importance_score AS importance_score,
			   r.is_frequent AS is_frequent, r.is_important AS is_important
		ORDER BY r.importance_score DESC, r.last_contact DESC
		LIMIT $limit
	`

	result, err := session.Run(ctx, query, map[string]interface{}{
		"userID": userID,
		"limit":  limit,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get contact relationships: %w", err)
	}

	return parseContactRelationships(ctx, result)
}

// GetContactRelationship retrieves a specific contact relationship.
func (a *PersonalizationAdapter) GetContactRelationship(ctx context.Context, userID, contactEmail string) (*out.ContactRelationship, error) {
	session := a.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: a.dbName})
	defer session.Close(ctx)

	query := `
		MATCH (u:User {user_id: $userID})-[r:COMMUNICATES_WITH]->(c:Contact {email: $contactEmail})
		RETURN c.email AS contact_email, c.name AS contact_name,
			   r.relation_type AS relation_type,
			   r.emails_sent AS emails_sent, r.emails_received AS emails_received,
			   r.last_contact AS last_contact, r.first_contact AS first_contact,
			   r.tone_used AS tone_used, r.formality_level AS formality_level,
			   r.avg_reply_time AS avg_reply_time,
			   r.importance_score AS importance_score,
			   r.is_frequent AS is_frequent, r.is_important AS is_important
	`

	result, err := session.Run(ctx, query, map[string]interface{}{
		"userID":       userID,
		"contactEmail": contactEmail,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get contact relationship: %w", err)
	}

	rels, err := parseContactRelationships(ctx, result)
	if err != nil {
		return nil, err
	}
	if len(rels) > 0 {
		return rels[0], nil
	}
	return nil, nil
}

// UpsertContactRelationship creates or updates a contact relationship.
func (a *PersonalizationAdapter) UpsertContactRelationship(ctx context.Context, userID string, rel *out.ContactRelationship) error {
	session := a.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: a.dbName})
	defer session.Close(ctx)

	query := `
		MERGE (u:User {user_id: $userID})
		MERGE (c:Contact {email: $contactEmail})
		SET c.name = $contactName
		MERGE (u)-[r:COMMUNICATES_WITH]->(c)
		SET r.relation_type = $relationType,
			r.emails_sent = $emailsSent,
			r.emails_received = $emailsReceived,
			r.last_contact = $lastContact,
			r.first_contact = COALESCE(r.first_contact, $firstContact),
			r.tone_used = $toneUsed,
			r.formality_level = $formalityLevel,
			r.avg_reply_time = $avgReplyTime,
			r.importance_score = $importanceScore,
			r.is_frequent = $isFrequent,
			r.is_important = $isImportant,
			r.updated_at = timestamp()
	`

	params := map[string]interface{}{
		"userID":          userID,
		"contactEmail":    rel.ContactEmail,
		"contactName":     rel.ContactName,
		"relationType":    rel.RelationType,
		"emailsSent":      rel.EmailsSent,
		"emailsReceived":  rel.EmailsReceived,
		"lastContact":     rel.LastContact.Unix(),
		"firstContact":    rel.FirstContact.Unix(),
		"toneUsed":        rel.ToneUsed,
		"formalityLevel":  rel.FormalityLevel,
		"avgReplyTime":    rel.AvgReplyTime,
		"importanceScore": rel.ImportanceScore,
		"isFrequent":      rel.IsFrequent,
		"isImportant":     rel.IsImportant,
	}

	_, err := session.Run(ctx, query, params)
	if err != nil {
		return fmt.Errorf("failed to upsert contact relationship: %w", err)
	}

	return nil
}

// GetFrequentContacts retrieves frequently contacted contacts.
func (a *PersonalizationAdapter) GetFrequentContacts(ctx context.Context, userID string, limit int) ([]*out.ContactRelationship, error) {
	session := a.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: a.dbName})
	defer session.Close(ctx)

	query := `
		MATCH (u:User {user_id: $userID})-[r:COMMUNICATES_WITH]->(c:Contact)
		WHERE r.is_frequent = true
		RETURN c.email AS contact_email, c.name AS contact_name,
			   r.relation_type AS relation_type,
			   r.emails_sent AS emails_sent, r.emails_received AS emails_received,
			   r.last_contact AS last_contact, r.first_contact AS first_contact,
			   r.tone_used AS tone_used, r.formality_level AS formality_level,
			   r.avg_reply_time AS avg_reply_time,
			   r.importance_score AS importance_score,
			   r.is_frequent AS is_frequent, r.is_important AS is_important
		ORDER BY (r.emails_sent + r.emails_received) DESC
		LIMIT $limit
	`

	result, err := session.Run(ctx, query, map[string]interface{}{
		"userID": userID,
		"limit":  limit,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get frequent contacts: %w", err)
	}

	return parseContactRelationships(ctx, result)
}

// GetImportantContacts retrieves important contacts.
func (a *PersonalizationAdapter) GetImportantContacts(ctx context.Context, userID string, limit int) ([]*out.ContactRelationship, error) {
	session := a.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: a.dbName})
	defer session.Close(ctx)

	query := `
		MATCH (u:User {user_id: $userID})-[r:COMMUNICATES_WITH]->(c:Contact)
		WHERE r.is_important = true
		RETURN c.email AS contact_email, c.name AS contact_name,
			   r.relation_type AS relation_type,
			   r.emails_sent AS emails_sent, r.emails_received AS emails_received,
			   r.last_contact AS last_contact, r.first_contact AS first_contact,
			   r.tone_used AS tone_used, r.formality_level AS formality_level,
			   r.avg_reply_time AS avg_reply_time,
			   r.importance_score AS importance_score,
			   r.is_frequent AS is_frequent, r.is_important AS is_important
		ORDER BY r.importance_score DESC
		LIMIT $limit
	`

	result, err := session.Run(ctx, query, map[string]interface{}{
		"userID": userID,
		"limit":  limit,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get important contacts: %w", err)
	}

	return parseContactRelationships(ctx, result)
}

// =============================================================================
// Communication Pattern Operations
// =============================================================================

// GetCommunicationPatterns retrieves communication patterns by type.
func (a *PersonalizationAdapter) GetCommunicationPatterns(ctx context.Context, userID string, patternType string, limit int) ([]*out.CommunicationPattern, error) {
	session := a.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: a.dbName})
	defer session.Close(ctx)

	query := `
		MATCH (u:User {user_id: $userID})-[:HAS_PATTERN]->(p:CommunicationPattern)
		WHERE p.pattern_type = $patternType
		RETURN p.pattern_id AS pattern_id, p.pattern_type AS pattern_type,
			   p.text AS text, p.variants AS variants, p.context AS context,
			   p.recipient AS recipient, p.usage_count AS usage_count,
			   p.last_used AS last_used, p.confidence AS confidence
		ORDER BY p.usage_count DESC
		LIMIT $limit
	`

	result, err := session.Run(ctx, query, map[string]interface{}{
		"userID":      userID,
		"patternType": patternType,
		"limit":       limit,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get communication patterns: %w", err)
	}

	return parseCommunicationPatterns(ctx, userID, result)
}

// UpsertCommunicationPattern creates or updates a communication pattern.
func (a *PersonalizationAdapter) UpsertCommunicationPattern(ctx context.Context, userID string, pattern *out.CommunicationPattern) error {
	session := a.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: a.dbName})
	defer session.Close(ctx)

	query := `
		MERGE (u:User {user_id: $userID})
		MERGE (p:CommunicationPattern {pattern_id: $patternID, user_id: $userID})
		SET p.pattern_type = $patternType,
			p.text = $text,
			p.variants = $variants,
			p.context = $context,
			p.recipient = $recipient,
			p.usage_count = $usageCount,
			p.last_used = $lastUsed,
			p.confidence = $confidence,
			p.updated_at = timestamp()
		MERGE (u)-[:HAS_PATTERN]->(p)
	`

	params := map[string]interface{}{
		"userID":      userID,
		"patternID":   pattern.PatternID,
		"patternType": pattern.PatternType,
		"text":        pattern.Text,
		"variants":    pattern.Variants,
		"context":     pattern.Context,
		"recipient":   pattern.Recipient,
		"usageCount":  pattern.UsageCount,
		"lastUsed":    pattern.LastUsed.Unix(),
		"confidence":  pattern.Confidence,
	}

	_, err := session.Run(ctx, query, params)
	if err != nil {
		return fmt.Errorf("failed to upsert communication pattern: %w", err)
	}

	return nil
}

// GetPatternsByContext retrieves patterns by context (formal, casual, etc.).
func (a *PersonalizationAdapter) GetPatternsByContext(ctx context.Context, userID, contextType string, limit int) ([]*out.CommunicationPattern, error) {
	session := a.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: a.dbName})
	defer session.Close(ctx)

	query := `
		MATCH (u:User {user_id: $userID})-[:HAS_PATTERN]->(p:CommunicationPattern)
		WHERE p.context = $context
		RETURN p.pattern_id AS pattern_id, p.pattern_type AS pattern_type,
			   p.text AS text, p.variants AS variants, p.context AS context,
			   p.recipient AS recipient, p.usage_count AS usage_count,
			   p.last_used AS last_used, p.confidence AS confidence
		ORDER BY p.usage_count DESC
		LIMIT $limit
	`

	result, err := session.Run(ctx, query, map[string]interface{}{
		"userID":  userID,
		"context": contextType,
		"limit":   limit,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get patterns by context: %w", err)
	}

	return parseCommunicationPatterns(ctx, userID, result)
}

// IncrementPatternUsage increments the usage count of a pattern.
func (a *PersonalizationAdapter) IncrementPatternUsage(ctx context.Context, userID, patternID string) error {
	session := a.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: a.dbName})
	defer session.Close(ctx)

	query := `
		MATCH (p:CommunicationPattern {pattern_id: $patternID, user_id: $userID})
		SET p.usage_count = p.usage_count + 1,
			p.last_used = timestamp()
	`

	_, err := session.Run(ctx, query, map[string]interface{}{
		"userID":    userID,
		"patternID": patternID,
	})
	if err != nil {
		return fmt.Errorf("failed to increment pattern usage: %w", err)
	}

	return nil
}

// =============================================================================
// Topic Expertise Operations
// =============================================================================

// GetTopicExpertise retrieves user's topic expertise.
func (a *PersonalizationAdapter) GetTopicExpertise(ctx context.Context, userID string, limit int) ([]*out.TopicExpertise, error) {
	session := a.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: a.dbName})
	defer session.Close(ctx)

	query := `
		MATCH (u:User {user_id: $userID})-[:HAS_EXPERTISE]->(t:TopicExpertise)
		RETURN t.topic AS topic, t.expertise_level AS expertise_level,
			   t.mention_count AS mention_count, t.last_mentioned AS last_mentioned,
			   t.related_keywords AS related_keywords
		ORDER BY t.expertise_level DESC, t.mention_count DESC
		LIMIT $limit
	`

	result, err := session.Run(ctx, query, map[string]interface{}{
		"userID": userID,
		"limit":  limit,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get topic expertise: %w", err)
	}

	var topics []*out.TopicExpertise
	for result.Next(ctx) {
		record := result.Record()

		var lastMentioned time.Time
		if ts, ok := record.Get("last_mentioned"); ok && ts != nil {
			if tsInt, ok := ts.(int64); ok {
				lastMentioned = time.Unix(tsInt, 0)
			}
		}

		topic := &out.TopicExpertise{
			Topic:           getStringValue(record, "topic"),
			ExpertiseLevel:  getFloatValue(record, "expertise_level"),
			MentionCount:    getIntValue(record, "mention_count"),
			LastMentioned:   lastMentioned,
			RelatedKeywords: getStringArrayValue(record, "related_keywords"),
		}
		topics = append(topics, topic)
	}

	return topics, nil
}

// UpsertTopicExpertise creates or updates topic expertise.
func (a *PersonalizationAdapter) UpsertTopicExpertise(ctx context.Context, userID string, topic *out.TopicExpertise) error {
	session := a.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: a.dbName})
	defer session.Close(ctx)

	query := `
		MERGE (u:User {user_id: $userID})
		MERGE (t:TopicExpertise {topic: $topic, user_id: $userID})
		SET t.expertise_level = $expertiseLevel,
			t.mention_count = $mentionCount,
			t.last_mentioned = $lastMentioned,
			t.related_keywords = $relatedKeywords,
			t.updated_at = timestamp()
		MERGE (u)-[:HAS_EXPERTISE]->(t)
	`

	params := map[string]interface{}{
		"userID":          userID,
		"topic":           topic.Topic,
		"expertiseLevel":  topic.ExpertiseLevel,
		"mentionCount":    topic.MentionCount,
		"lastMentioned":   topic.LastMentioned.Unix(),
		"relatedKeywords": topic.RelatedKeywords,
	}

	_, err := session.Run(ctx, query, params)
	if err != nil {
		return fmt.Errorf("failed to upsert topic expertise: %w", err)
	}

	return nil
}

// =============================================================================
// Autocomplete Context
// =============================================================================

// GetAutocompleteContext retrieves context for AI autocomplete.
func (a *PersonalizationAdapter) GetAutocompleteContext(ctx context.Context, userID string, recipientEmail string, inputPrefix string) (*out.AutocompleteContext, error) {
	result := &out.AutocompleteContext{}

	// Get extended profile
	profile, err := a.GetExtendedProfile(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get profile: %w", err)
	}
	result.UserProfile = profile

	// Get contact relationship if recipient is specified
	if recipientEmail != "" {
		contact, err := a.GetContactRelationship(ctx, userID, recipientEmail)
		if err == nil && contact != nil {
			result.ContactInfo = contact

			// Get tone preference based on contact type
			if contact.RelationType != "" {
				tone, err := a.GetTonePreference(ctx, userID, contact.RelationType)
				if err == nil && tone != nil {
					result.TonePreference = tone
				}
			}
		}
	}

	// Get writing style
	style, err := a.GetWritingStyle(ctx, userID)
	if err == nil && style != nil {
		result.WritingStyle = style
	}

	// Get relevant phrases
	phrases, err := a.GetFrequentPhrases(ctx, userID, 10)
	if err == nil {
		result.RelevantPhrases = phrases
	}

	// Get patterns based on context
	contextType := "general"
	if result.ContactInfo != nil {
		switch result.ContactInfo.RelationType {
		case "boss", "client":
			contextType = "formal"
		case "colleague", "friend":
			contextType = "casual"
		}
	}

	patterns, err := a.GetPatternsByContext(ctx, userID, contextType, 5)
	if err == nil {
		result.Patterns = patterns
	}

	return result, nil
}

// =============================================================================
// Extended Indexes
// =============================================================================

// EnsureExtendedIndexes creates indexes for extended functionality.
func (a *PersonalizationAdapter) EnsureExtendedIndexes(ctx context.Context) error {
	session := a.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: a.dbName})
	defer session.Close(ctx)

	queries := []string{
		// Contact indexes
		`CREATE CONSTRAINT contact_email_unique IF NOT EXISTS FOR (c:Contact) REQUIRE c.email IS UNIQUE`,
		`CREATE INDEX contact_name_idx IF NOT EXISTS FOR (c:Contact) ON (c.name)`,

		// Communication pattern indexes
		`CREATE INDEX comm_pattern_user_idx IF NOT EXISTS FOR (p:CommunicationPattern) ON (p.user_id)`,
		`CREATE INDEX comm_pattern_type_idx IF NOT EXISTS FOR (p:CommunicationPattern) ON (p.pattern_type)`,
		`CREATE INDEX comm_pattern_context_idx IF NOT EXISTS FOR (p:CommunicationPattern) ON (p.context)`,

		// Topic expertise indexes
		`CREATE INDEX topic_user_idx IF NOT EXISTS FOR (t:TopicExpertise) ON (t.user_id)`,
		`CREATE INDEX topic_level_idx IF NOT EXISTS FOR (t:TopicExpertise) ON (t.expertise_level)`,
	}

	for _, query := range queries {
		_, err := session.Run(ctx, query, nil)
		if err != nil {
			// Ignore if already exists
			continue
		}
	}

	return nil
}

// =============================================================================
// Helper Functions
// =============================================================================

func getStringArrayValue(record *neo4j.Record, key string) []string {
	if val, ok := record.Get(key); ok && val != nil {
		if arr, ok := val.([]interface{}); ok {
			result := make([]string, 0, len(arr))
			for _, v := range arr {
				if s, ok := v.(string); ok {
					result = append(result, s)
				}
			}
			return result
		}
	}
	return nil
}

func getFloatValue(record *neo4j.Record, key string) float64 {
	if val, ok := record.Get(key); ok && val != nil {
		if f, ok := val.(float64); ok {
			return f
		}
	}
	return 0
}

func parseContactRelationships(ctx context.Context, result neo4j.ResultWithContext) ([]*out.ContactRelationship, error) {
	var rels []*out.ContactRelationship
	for result.Next(ctx) {
		record := result.Record()

		var lastContact, firstContact time.Time
		if ts, ok := record.Get("last_contact"); ok && ts != nil {
			if tsInt, ok := ts.(int64); ok {
				lastContact = time.Unix(tsInt, 0)
			}
		}
		if ts, ok := record.Get("first_contact"); ok && ts != nil {
			if tsInt, ok := ts.(int64); ok {
				firstContact = time.Unix(tsInt, 0)
			}
		}

		rel := &out.ContactRelationship{
			ContactEmail:    getStringValue(record, "contact_email"),
			ContactName:     getStringValue(record, "contact_name"),
			RelationType:    getStringValue(record, "relation_type"),
			EmailsSent:      getIntValue(record, "emails_sent"),
			EmailsReceived:  getIntValue(record, "emails_received"),
			LastContact:     lastContact,
			FirstContact:    firstContact,
			ToneUsed:        getStringValue(record, "tone_used"),
			FormalityLevel:  getFloatValue(record, "formality_level"),
			AvgReplyTime:    getIntValue(record, "avg_reply_time"),
			ImportanceScore: getFloatValue(record, "importance_score"),
			IsFrequent:      getBoolValue(record, "is_frequent"),
			IsImportant:     getBoolValue(record, "is_important"),
		}
		rels = append(rels, rel)
	}
	return rels, nil
}

func parseCommunicationPatterns(ctx context.Context, userID string, result neo4j.ResultWithContext) ([]*out.CommunicationPattern, error) {
	var patterns []*out.CommunicationPattern
	for result.Next(ctx) {
		record := result.Record()

		var lastUsed time.Time
		if ts, ok := record.Get("last_used"); ok && ts != nil {
			if tsInt, ok := ts.(int64); ok {
				lastUsed = time.Unix(tsInt, 0)
			}
		}

		pattern := &out.CommunicationPattern{
			PatternID:   getStringValue(record, "pattern_id"),
			UserID:      userID,
			PatternType: getStringValue(record, "pattern_type"),
			Text:        getStringValue(record, "text"),
			Variants:    getStringArrayValue(record, "variants"),
			Context:     getStringValue(record, "context"),
			Recipient:   getStringValue(record, "recipient"),
			UsageCount:  getIntValue(record, "usage_count"),
			LastUsed:    lastUsed,
			Confidence:  getFloatValue(record, "confidence"),
		}
		patterns = append(patterns, pattern)
	}
	return patterns, nil
}

// =============================================================================
// Interface Compliance
// =============================================================================

var _ out.PersonalizationStore = (*PersonalizationAdapter)(nil)
var _ out.ExtendedPersonalizationStore = (*PersonalizationAdapter)(nil)
