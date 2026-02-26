// Package persistence provides database adapters implementing outbound ports.
package persistence

import (
	"database/sql"
	"fmt"
	"time"

	"worker_server/core/domain"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// SenderProfileAdapter implements domain.SenderProfileRepository using PostgreSQL.
type SenderProfileAdapter struct {
	db *sqlx.DB
}

// NewSenderProfileAdapter creates a new SenderProfileAdapter.
func NewSenderProfileAdapter(db *sqlx.DB) *SenderProfileAdapter {
	return &SenderProfileAdapter{db: db}
}

// senderProfileRow represents the database row for sender profiles.
type senderProfileRow struct {
	ID                 int64          `db:"id"`
	UserID             uuid.UUID      `db:"user_id"`
	Email              string         `db:"email"`
	Domain             string         `db:"domain"`
	LearnedCategory    sql.NullString `db:"learned_category"`
	LearnedSubCategory sql.NullString `db:"learned_sub_category"`
	IsVIP              bool           `db:"is_vip"`
	IsMuted            bool           `db:"is_muted"`
	EmailCount         int            `db:"email_count"`
	ReadRate           float64        `db:"read_rate"`
	ReplyRate          float64        `db:"reply_rate"`
	DeleteRate         float64        `db:"delete_rate"`
	IsContact          bool           `db:"is_contact"`
	InteractionCount   int            `db:"interaction_count"`
	LastInteractedAt   sql.NullTime   `db:"last_interacted_at"`
	ImportanceScore    float64        `db:"importance_score"`
	ConfirmedLabels    []int64        `db:"confirmed_labels"`
	DisplayName        sql.NullString `db:"display_name"`
	AvatarURL          sql.NullString `db:"avatar_url"`
	FirstSeenAt        time.Time      `db:"first_seen_at"`
	LastSeenAt         time.Time      `db:"last_seen_at"`
	CreatedAt          time.Time      `db:"created_at"`
	UpdatedAt          time.Time      `db:"updated_at"`
}

func (r *senderProfileRow) toEntity() *domain.SenderProfile {
	profile := &domain.SenderProfile{
		ID:               r.ID,
		UserID:           r.UserID,
		Email:            r.Email,
		Domain:           r.Domain,
		IsVIP:            r.IsVIP,
		IsMuted:          r.IsMuted,
		EmailCount:       r.EmailCount,
		ReadRate:         r.ReadRate,
		ReplyRate:        r.ReplyRate,
		DeleteRate:       r.DeleteRate,
		IsContact:        r.IsContact,
		InteractionCount: r.InteractionCount,
		ImportanceScore:  r.ImportanceScore,
		ConfirmedLabels:  r.ConfirmedLabels,
		FirstSeenAt:      r.FirstSeenAt,
		LastSeenAt:       r.LastSeenAt,
		CreatedAt:        r.CreatedAt,
		UpdatedAt:        r.UpdatedAt,
	}

	if r.LastInteractedAt.Valid {
		profile.LastInteractedAt = &r.LastInteractedAt.Time
	}
	if r.LearnedCategory.Valid {
		cat := domain.EmailCategory(r.LearnedCategory.String)
		profile.LearnedCategory = &cat
	}
	if r.LearnedSubCategory.Valid {
		subCat := domain.EmailSubCategory(r.LearnedSubCategory.String)
		profile.LearnedSubCategory = &subCat
	}
	if r.DisplayName.Valid {
		profile.DisplayName = &r.DisplayName.String
	}
	if r.AvatarURL.Valid {
		profile.AvatarURL = &r.AvatarURL.String
	}

	return profile
}

// GetByID retrieves a sender profile by its ID.
func (a *SenderProfileAdapter) GetByID(id int64) (*domain.SenderProfile, error) {
	var row senderProfileRow
	query := `SELECT * FROM sender_profiles WHERE id = $1`

	if err := a.db.Get(&row, query, id); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get sender profile: %w", err)
	}

	return row.toEntity(), nil
}

// GetByEmail retrieves a sender profile by user ID and email.
func (a *SenderProfileAdapter) GetByEmail(userID uuid.UUID, email string) (*domain.SenderProfile, error) {
	var row senderProfileRow
	query := `SELECT * FROM sender_profiles WHERE user_id = $1 AND email = $2`

	if err := a.db.Get(&row, query, userID, email); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get sender profile: %w", err)
	}

	return row.toEntity(), nil
}

// GetByDomain retrieves all sender profiles for a domain.
func (a *SenderProfileAdapter) GetByDomain(userID uuid.UUID, domainName string) ([]*domain.SenderProfile, error) {
	var rows []senderProfileRow
	query := `SELECT * FROM sender_profiles WHERE user_id = $1 AND domain = $2 ORDER BY email_count DESC`

	if err := a.db.Select(&rows, query, userID, domainName); err != nil {
		return nil, fmt.Errorf("failed to get sender profiles by domain: %w", err)
	}

	profiles := make([]*domain.SenderProfile, len(rows))
	for i, row := range rows {
		profiles[i] = row.toEntity()
	}

	return profiles, nil
}

// GetByUserID retrieves all sender profiles for a user.
func (a *SenderProfileAdapter) GetByUserID(userID uuid.UUID, limit, offset int) ([]*domain.SenderProfile, error) {
	var rows []senderProfileRow
	query := `SELECT * FROM sender_profiles WHERE user_id = $1 ORDER BY email_count DESC LIMIT $2 OFFSET $3`

	if err := a.db.Select(&rows, query, userID, limit, offset); err != nil {
		return nil, fmt.Errorf("failed to list sender profiles: %w", err)
	}

	profiles := make([]*domain.SenderProfile, len(rows))
	for i, row := range rows {
		profiles[i] = row.toEntity()
	}

	return profiles, nil
}

// Create creates a new sender profile.
func (a *SenderProfileAdapter) Create(profile *domain.SenderProfile) error {
	query := `
		INSERT INTO sender_profiles (
			user_id, email, domain, learned_category, learned_sub_category,
			is_vip, is_muted, email_count, read_rate, reply_rate,
			display_name, avatar_url, first_seen_at, last_seen_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		RETURNING id, created_at, updated_at`

	var learnedCat sql.NullString
	if profile.LearnedCategory != nil {
		learnedCat = sql.NullString{String: string(*profile.LearnedCategory), Valid: true}
	}

	var learnedSubCat sql.NullString
	if profile.LearnedSubCategory != nil {
		learnedSubCat = sql.NullString{String: string(*profile.LearnedSubCategory), Valid: true}
	}

	var displayName sql.NullString
	if profile.DisplayName != nil {
		displayName = sql.NullString{String: *profile.DisplayName, Valid: true}
	}

	var avatarURL sql.NullString
	if profile.AvatarURL != nil {
		avatarURL = sql.NullString{String: *profile.AvatarURL, Valid: true}
	}

	err := a.db.QueryRow(
		query,
		profile.UserID,
		profile.Email,
		profile.Domain,
		learnedCat,
		learnedSubCat,
		profile.IsVIP,
		profile.IsMuted,
		profile.EmailCount,
		profile.ReadRate,
		profile.ReplyRate,
		displayName,
		avatarURL,
		profile.FirstSeenAt,
		profile.LastSeenAt,
	).Scan(&profile.ID, &profile.CreatedAt, &profile.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create sender profile: %w", err)
	}

	return nil
}

// Update updates a sender profile.
func (a *SenderProfileAdapter) Update(profile *domain.SenderProfile) error {
	query := `
		UPDATE sender_profiles
		SET learned_category = $2, learned_sub_category = $3,
		    is_vip = $4, is_muted = $5,
		    display_name = $6, avatar_url = $7,
		    updated_at = NOW()
		WHERE id = $1`

	var learnedCat sql.NullString
	if profile.LearnedCategory != nil {
		learnedCat = sql.NullString{String: string(*profile.LearnedCategory), Valid: true}
	}

	var learnedSubCat sql.NullString
	if profile.LearnedSubCategory != nil {
		learnedSubCat = sql.NullString{String: string(*profile.LearnedSubCategory), Valid: true}
	}

	var displayName sql.NullString
	if profile.DisplayName != nil {
		displayName = sql.NullString{String: *profile.DisplayName, Valid: true}
	}

	var avatarURL sql.NullString
	if profile.AvatarURL != nil {
		avatarURL = sql.NullString{String: *profile.AvatarURL, Valid: true}
	}

	result, err := a.db.Exec(query, profile.ID, learnedCat, learnedSubCat, profile.IsVIP, profile.IsMuted, displayName, avatarURL)
	if err != nil {
		return fmt.Errorf("failed to update sender profile: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("sender profile not found: %d", profile.ID)
	}

	return nil
}

// Delete deletes a sender profile.
func (a *SenderProfileAdapter) Delete(id int64) error {
	query := `DELETE FROM sender_profiles WHERE id = $1`

	result, err := a.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete sender profile: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("sender profile not found: %d", id)
	}

	return nil
}

// GetVIPSenders retrieves all VIP senders for a user.
func (a *SenderProfileAdapter) GetVIPSenders(userID uuid.UUID) ([]*domain.SenderProfile, error) {
	var rows []senderProfileRow
	query := `SELECT * FROM sender_profiles WHERE user_id = $1 AND is_vip = TRUE ORDER BY email_count DESC`

	if err := a.db.Select(&rows, query, userID); err != nil {
		return nil, fmt.Errorf("failed to get VIP senders: %w", err)
	}

	profiles := make([]*domain.SenderProfile, len(rows))
	for i, row := range rows {
		profiles[i] = row.toEntity()
	}

	return profiles, nil
}

// GetMutedSenders retrieves all muted senders for a user.
func (a *SenderProfileAdapter) GetMutedSenders(userID uuid.UUID) ([]*domain.SenderProfile, error) {
	var rows []senderProfileRow
	query := `SELECT * FROM sender_profiles WHERE user_id = $1 AND is_muted = TRUE ORDER BY email_count DESC`

	if err := a.db.Select(&rows, query, userID); err != nil {
		return nil, fmt.Errorf("failed to get muted senders: %w", err)
	}

	profiles := make([]*domain.SenderProfile, len(rows))
	for i, row := range rows {
		profiles[i] = row.toEntity()
	}

	return profiles, nil
}

// IncrementEmailCount increments the email count for a sender profile.
func (a *SenderProfileAdapter) IncrementEmailCount(id int64) error {
	query := `UPDATE sender_profiles SET email_count = email_count + 1, updated_at = NOW() WHERE id = $1`

	_, err := a.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to increment email count: %w", err)
	}

	return nil
}

// UpdateReadRate updates the read rate for a sender profile.
func (a *SenderProfileAdapter) UpdateReadRate(id int64, newRate float64) error {
	query := `UPDATE sender_profiles SET read_rate = $2, updated_at = NOW() WHERE id = $1`

	_, err := a.db.Exec(query, id, newRate)
	if err != nil {
		return fmt.Errorf("failed to update read rate: %w", err)
	}

	return nil
}

// UpdateReplyRate updates the reply rate for a sender profile.
func (a *SenderProfileAdapter) UpdateReplyRate(id int64, newRate float64) error {
	query := `UPDATE sender_profiles SET reply_rate = $2, updated_at = NOW() WHERE id = $1`

	_, err := a.db.Exec(query, id, newRate)
	if err != nil {
		return fmt.Errorf("failed to update reply rate: %w", err)
	}

	return nil
}

// UpdateLastSeen updates the last seen time for a sender profile.
func (a *SenderProfileAdapter) UpdateLastSeen(id int64, lastSeenAt time.Time) error {
	query := `UPDATE sender_profiles SET last_seen_at = $2, updated_at = NOW() WHERE id = $1`

	_, err := a.db.Exec(query, id, lastSeenAt)
	if err != nil {
		return fmt.Errorf("failed to update last seen: %w", err)
	}

	return nil
}

// GetTopSenders retrieves senders with importance score >= minScore.
func (a *SenderProfileAdapter) GetTopSenders(userID uuid.UUID, minScore float64, limit int) ([]*domain.SenderProfile, error) {
	var rows []senderProfileRow
	query := `SELECT * FROM sender_profiles
		WHERE user_id = $1 AND importance_score >= $2
		ORDER BY importance_score DESC LIMIT $3`

	if err := a.db.Select(&rows, query, userID, minScore, limit); err != nil {
		return nil, fmt.Errorf("failed to get top senders: %w", err)
	}

	profiles := make([]*domain.SenderProfile, len(rows))
	for i, row := range rows {
		profiles[i] = row.toEntity()
	}

	return profiles, nil
}

// GetContactSenders retrieves all senders who are also contacts.
func (a *SenderProfileAdapter) GetContactSenders(userID uuid.UUID) ([]*domain.SenderProfile, error) {
	var rows []senderProfileRow
	query := `SELECT * FROM sender_profiles WHERE user_id = $1 AND is_contact = TRUE ORDER BY email_count DESC`

	if err := a.db.Select(&rows, query, userID); err != nil {
		return nil, fmt.Errorf("failed to get contact senders: %w", err)
	}

	profiles := make([]*domain.SenderProfile, len(rows))
	for i, row := range rows {
		profiles[i] = row.toEntity()
	}

	return profiles, nil
}

// UpdateDeleteRate updates the delete rate for a sender profile.
func (a *SenderProfileAdapter) UpdateDeleteRate(id int64, newRate float64) error {
	query := `UPDATE sender_profiles SET delete_rate = $2, updated_at = NOW() WHERE id = $1`

	_, err := a.db.Exec(query, id, newRate)
	if err != nil {
		return fmt.Errorf("failed to update delete rate: %w", err)
	}

	return nil
}

// UpdateIsContact updates the is_contact flag for a sender profile.
func (a *SenderProfileAdapter) UpdateIsContact(id int64, isContact bool) error {
	query := `UPDATE sender_profiles SET is_contact = $2, updated_at = NOW() WHERE id = $1`

	_, err := a.db.Exec(query, id, isContact)
	if err != nil {
		return fmt.Errorf("failed to update is_contact: %w", err)
	}

	return nil
}

// IncrementInteractionCount increments the interaction count for a sender profile.
func (a *SenderProfileAdapter) IncrementInteractionCount(id int64) error {
	query := `UPDATE sender_profiles SET interaction_count = interaction_count + 1, updated_at = NOW() WHERE id = $1`

	_, err := a.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to increment interaction count: %w", err)
	}

	return nil
}

// UpdateLastInteraction updates the last interaction time for a sender profile.
func (a *SenderProfileAdapter) UpdateLastInteraction(id int64, at time.Time) error {
	query := `UPDATE sender_profiles SET last_interacted_at = $2, updated_at = NOW() WHERE id = $1`

	_, err := a.db.Exec(query, id, at)
	if err != nil {
		return fmt.Errorf("failed to update last interaction: %w", err)
	}

	return nil
}

// AddConfirmedLabel adds a label ID to the confirmed_labels array.
func (a *SenderProfileAdapter) AddConfirmedLabel(id int64, labelID int64) error {
	query := `UPDATE sender_profiles
		SET confirmed_labels = array_append(confirmed_labels, $2), updated_at = NOW()
		WHERE id = $1 AND NOT ($2 = ANY(confirmed_labels))`

	_, err := a.db.Exec(query, id, labelID)
	if err != nil {
		return fmt.Errorf("failed to add confirmed label: %w", err)
	}

	return nil
}

// RemoveConfirmedLabel removes a label ID from the confirmed_labels array.
func (a *SenderProfileAdapter) RemoveConfirmedLabel(id int64, labelID int64) error {
	query := `UPDATE sender_profiles
		SET confirmed_labels = array_remove(confirmed_labels, $2), updated_at = NOW()
		WHERE id = $1`

	_, err := a.db.Exec(query, id, labelID)
	if err != nil {
		return fmt.Errorf("failed to remove confirmed label: %w", err)
	}

	return nil
}

// KnownDomainAdapter implements domain.KnownDomainRepository using PostgreSQL.
type KnownDomainAdapter struct {
	db *sqlx.DB
}

// NewKnownDomainAdapter creates a new KnownDomainAdapter.
func NewKnownDomainAdapter(db *sqlx.DB) *KnownDomainAdapter {
	return &KnownDomainAdapter{db: db}
}

// knownDomainRow represents the database row for known domains.
type knownDomainRow struct {
	ID          int            `db:"id"`
	Domain      string         `db:"domain"`
	Category    string         `db:"category"`
	SubCategory sql.NullString `db:"sub_category"`
	Confidence  float64        `db:"confidence"`
	Source      string         `db:"source"`
	CreatedAt   time.Time      `db:"created_at"`
	UpdatedAt   time.Time      `db:"updated_at"`
}

func (r *knownDomainRow) toEntity() *domain.KnownDomain {
	kd := &domain.KnownDomain{
		ID:         r.ID,
		Domain:     r.Domain,
		Category:   domain.EmailCategory(r.Category),
		Confidence: r.Confidence,
		Source:     r.Source,
		CreatedAt:  r.CreatedAt,
		UpdatedAt:  r.UpdatedAt,
	}

	if r.SubCategory.Valid {
		subCat := domain.EmailSubCategory(r.SubCategory.String)
		kd.SubCategory = &subCat
	}

	return kd
}

// GetByDomain retrieves a known domain by its domain name.
func (a *KnownDomainAdapter) GetByDomain(domainName string) (*domain.KnownDomain, error) {
	var row knownDomainRow
	query := `SELECT * FROM known_domains WHERE domain = $1`

	if err := a.db.Get(&row, query, domainName); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get known domain: %w", err)
	}

	return row.toEntity(), nil
}

// List retrieves all known domains.
func (a *KnownDomainAdapter) List() ([]*domain.KnownDomain, error) {
	var rows []knownDomainRow
	query := `SELECT * FROM known_domains ORDER BY domain ASC`

	if err := a.db.Select(&rows, query); err != nil {
		return nil, fmt.Errorf("failed to list known domains: %w", err)
	}

	domains := make([]*domain.KnownDomain, len(rows))
	for i, row := range rows {
		domains[i] = row.toEntity()
	}

	return domains, nil
}

// Create creates a new known domain.
func (a *KnownDomainAdapter) Create(kd *domain.KnownDomain) error {
	query := `
		INSERT INTO known_domains (domain, category, sub_category, confidence, source)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at, updated_at`

	var subCategory sql.NullString
	if kd.SubCategory != nil {
		subCategory = sql.NullString{String: string(*kd.SubCategory), Valid: true}
	}

	err := a.db.QueryRow(
		query,
		kd.Domain,
		string(kd.Category),
		subCategory,
		kd.Confidence,
		kd.Source,
	).Scan(&kd.ID, &kd.CreatedAt, &kd.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create known domain: %w", err)
	}

	return nil
}

// Update updates a known domain.
func (a *KnownDomainAdapter) Update(kd *domain.KnownDomain) error {
	query := `
		UPDATE known_domains
		SET category = $2, sub_category = $3, confidence = $4, source = $5, updated_at = NOW()
		WHERE id = $1`

	var subCategory sql.NullString
	if kd.SubCategory != nil {
		subCategory = sql.NullString{String: string(*kd.SubCategory), Valid: true}
	}

	result, err := a.db.Exec(query, kd.ID, string(kd.Category), subCategory, kd.Confidence, kd.Source)
	if err != nil {
		return fmt.Errorf("failed to update known domain: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("known domain not found: %d", kd.ID)
	}

	return nil
}

// Delete deletes a known domain.
func (a *KnownDomainAdapter) Delete(id int) error {
	query := `DELETE FROM known_domains WHERE id = $1`

	result, err := a.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete known domain: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("known domain not found: %d", id)
	}

	return nil
}
