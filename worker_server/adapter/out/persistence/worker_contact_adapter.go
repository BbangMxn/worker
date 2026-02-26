// Package persistence provides database adapters implementing outbound ports.
package persistence

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"worker_server/core/domain"
	"worker_server/core/port/out"
	"worker_server/pkg/crypto"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

// ContactAdapter implements out.ContactRepository using PostgreSQL.
type ContactAdapter struct {
	db *sqlx.DB
}

// NewContactAdapter creates a new ContactAdapter.
func NewContactAdapter(db *sqlx.DB) *ContactAdapter {
	return &ContactAdapter{db: db}
}

// encryptField encrypts a sensitive field
func (a *ContactAdapter) encryptField(value string) string {
	if value == "" {
		return ""
	}
	encrypted, err := crypto.Encrypt(value)
	if err != nil {
		return value // fallback to plain text
	}
	return encrypted
}

// decryptField decrypts a sensitive field
func (a *ContactAdapter) decryptField(value string) string {
	if value == "" {
		return ""
	}
	if !crypto.IsEncrypted(value) {
		return value // not encrypted (legacy data)
	}
	decrypted, err := crypto.Decrypt(value)
	if err != nil {
		return value // fallback
	}
	return decrypted
}

// decryptContactEntity decrypts sensitive fields in a contact entity
func (a *ContactAdapter) decryptContactEntity(entity *out.ContactEntity) {
	if entity == nil {
		return
	}
	entity.Phone = a.decryptField(entity.Phone)
	entity.Notes = a.decryptField(entity.Notes)
}

// decryptDomainContact decrypts sensitive fields in a domain contact
func (a *ContactAdapter) decryptDomainContact(contact *domain.Contact) {
	if contact == nil {
		return
	}
	contact.Phone = a.decryptField(contact.Phone)
	contact.Notes = a.decryptField(contact.Notes)
}

// contactRow represents the database row for contacts.
type contactRow struct {
	ID                   int64          `db:"id"`
	UserID               uuid.UUID      `db:"user_id"`
	Provider             sql.NullString `db:"provider"`
	ProviderID           sql.NullString `db:"provider_id"`
	Name                 string         `db:"name"`
	Email                sql.NullString `db:"email"`
	Phone                sql.NullString `db:"phone"`
	PhotoURL             sql.NullString `db:"photo_url"`
	Company              sql.NullString `db:"company"`
	JobTitle             sql.NullString `db:"job_title"`
	Department           sql.NullString `db:"department"`
	Notes                sql.NullString `db:"notes"`
	Tags                 pq.StringArray `db:"tags"`
	Groups               pq.StringArray `db:"groups"`
	RelationshipScore    int16          `db:"relationship_score"`
	InteractionCount     int            `db:"interaction_count"`
	InteractionFrequency sql.NullString `db:"interaction_frequency"`
	LastContactDate      sql.NullTime   `db:"last_contact_date"`
	LastInteractionAt    sql.NullTime   `db:"last_interaction_at"`
	IsFavorite           bool           `db:"is_favorite"`
	CreatedAt            time.Time      `db:"created_at"`
	UpdatedAt            time.Time      `db:"updated_at"`
	SyncedAt             sql.NullTime   `db:"synced_at"`
}

func (r *contactRow) toEntity() *out.ContactEntity {
	entity := &out.ContactEntity{
		ID:                r.ID,
		UserID:            r.UserID,
		Name:              r.Name,
		Tags:              r.Tags,
		Groups:            r.Groups,
		RelationshipScore: r.RelationshipScore,
		InteractionCount:  r.InteractionCount,
		IsFavorite:        r.IsFavorite,
		CreatedAt:         r.CreatedAt,
		UpdatedAt:         r.UpdatedAt,
	}

	if r.Provider.Valid {
		entity.Provider = r.Provider.String
	}
	if r.ProviderID.Valid {
		entity.ProviderID = r.ProviderID.String
	}
	if r.Email.Valid {
		entity.Email = r.Email.String
	}
	if r.Phone.Valid {
		entity.Phone = r.Phone.String
	}
	if r.PhotoURL.Valid {
		entity.PhotoURL = r.PhotoURL.String
	}
	if r.Company.Valid {
		entity.Company = r.Company.String
	}
	if r.JobTitle.Valid {
		entity.JobTitle = r.JobTitle.String
	}
	if r.Department.Valid {
		entity.Department = r.Department.String
	}
	if r.Notes.Valid {
		entity.Notes = r.Notes.String
	}
	if r.InteractionFrequency.Valid {
		entity.InteractionFrequency = r.InteractionFrequency.String
	}
	if r.LastContactDate.Valid {
		entity.LastContactDate = &r.LastContactDate.Time
	}
	if r.LastInteractionAt.Valid {
		entity.LastInteractionAt = &r.LastInteractionAt.Time
	}
	if r.SyncedAt.Valid {
		entity.SyncedAt = &r.SyncedAt.Time
	}

	return entity
}

func (r *contactRow) toDomain() *domain.Contact {
	contact := &domain.Contact{
		ID:                r.ID,
		UserID:            r.UserID,
		Name:              r.Name,
		Tags:              r.Tags,
		Groups:            r.Groups,
		RelationshipScore: r.RelationshipScore,
		InteractionCount:  r.InteractionCount,
		IsFavorite:        r.IsFavorite,
		CreatedAt:         r.CreatedAt,
		UpdatedAt:         r.UpdatedAt,
	}

	if r.Provider.Valid {
		contact.Provider = r.Provider.String
	}
	if r.ProviderID.Valid {
		contact.ProviderID = r.ProviderID.String
	}
	if r.Email.Valid {
		contact.Email = r.Email.String
	}
	if r.Phone.Valid {
		contact.Phone = r.Phone.String
	}
	if r.PhotoURL.Valid {
		contact.PhotoURL = r.PhotoURL.String
	}
	if r.Company.Valid {
		contact.Company = r.Company.String
	}
	if r.JobTitle.Valid {
		contact.JobTitle = r.JobTitle.String
	}
	if r.Department.Valid {
		contact.Department = r.Department.String
	}
	if r.Notes.Valid {
		contact.Notes = r.Notes.String
	}
	if r.InteractionFrequency.Valid {
		contact.InteractionFrequency = r.InteractionFrequency.String
	}
	if r.LastContactDate.Valid {
		contact.LastContactDate = &r.LastContactDate.Time
	}
	if r.LastInteractionAt.Valid {
		contact.LastInteractionAt = &r.LastInteractionAt.Time
	}
	if r.SyncedAt.Valid {
		contact.SyncedAt = &r.SyncedAt.Time
	}

	return contact
}

// Create creates a new contact.
func (a *ContactAdapter) Create(ctx context.Context, contact *out.ContactEntity) error {
	// Encrypt sensitive fields
	encryptedPhone := a.encryptField(contact.Phone)
	encryptedNotes := a.encryptField(contact.Notes)

	query := `
		INSERT INTO contacts (
			user_id, provider, provider_id, name, email, phone, photo_url,
			company, job_title, department, notes, tags, is_favorite
		) VALUES (
			$1, NULLIF($2, ''), NULLIF($3, ''), $4, NULLIF($5, ''), NULLIF($6, ''),
			NULLIF($7, ''), NULLIF($8, ''), NULLIF($9, ''), NULLIF($10, ''),
			NULLIF($11, ''), $12, $13
		)
		RETURNING id, created_at, updated_at
	`

	return a.db.QueryRowxContext(ctx, query,
		contact.UserID,
		contact.Provider,
		contact.ProviderID,
		contact.Name,
		contact.Email,
		encryptedPhone,
		contact.PhotoURL,
		contact.Company,
		contact.JobTitle,
		contact.Department,
		encryptedNotes,
		pq.Array(contact.Tags),
		contact.IsFavorite,
	).Scan(&contact.ID, &contact.CreatedAt, &contact.UpdatedAt)
}

// Update updates a contact.
func (a *ContactAdapter) Update(ctx context.Context, contact *out.ContactEntity) error {
	// Encrypt sensitive fields
	encryptedPhone := a.encryptField(contact.Phone)
	encryptedNotes := a.encryptField(contact.Notes)

	query := `
		UPDATE contacts SET
			name = $1, email = NULLIF($2, ''), phone = NULLIF($3, ''),
			photo_url = NULLIF($4, ''), company = NULLIF($5, ''),
			job_title = NULLIF($6, ''), department = NULLIF($7, ''),
			notes = NULLIF($8, ''), tags = $9, is_favorite = $10,
			updated_at = NOW()
		WHERE id = $11 AND user_id = $12
	`

	result, err := a.db.ExecContext(ctx, query,
		contact.Name, contact.Email, encryptedPhone,
		contact.PhotoURL, contact.Company, contact.JobTitle,
		contact.Department, encryptedNotes, pq.Array(contact.Tags),
		contact.IsFavorite, contact.ID, contact.UserID,
	)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("contact not found")
	}
	return nil
}

// Delete deletes a contact.
func (a *ContactAdapter) Delete(ctx context.Context, userID uuid.UUID, id int64) error {
	query := `DELETE FROM contacts WHERE id = $1 AND user_id = $2`

	result, err := a.db.ExecContext(ctx, query, id, userID)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("contact not found")
	}
	return nil
}

// GetByID gets a contact by ID.
func (a *ContactAdapter) GetByID(ctx context.Context, userID uuid.UUID, id int64) (*out.ContactEntity, error) {
	query := `SELECT * FROM contacts WHERE id = $1 AND user_id = $2`

	var row contactRow
	err := a.db.QueryRowxContext(ctx, query, id, userID).StructScan(&row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("contact not found")
		}
		return nil, err
	}

	entity := row.toEntity()
	a.decryptContactEntity(entity)
	return entity, nil
}

// GetByEmail gets a contact by email.
func (a *ContactAdapter) GetByEmail(ctx context.Context, userID uuid.UUID, email string) (*out.ContactEntity, error) {
	query := `SELECT * FROM contacts WHERE email = $1 AND user_id = $2`

	var row contactRow
	err := a.db.QueryRowxContext(ctx, query, email, userID).StructScan(&row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	entity := row.toEntity()
	a.decryptContactEntity(entity)
	return entity, nil
}

// List lists contacts with filters.
func (a *ContactAdapter) List(ctx context.Context, userID uuid.UUID, query *out.ContactListQuery) ([]*out.ContactEntity, int, error) {
	if query == nil {
		query = &out.ContactListQuery{}
	}
	if query.Limit <= 0 || query.Limit > 100 {
		query.Limit = 50
	}
	if query.OrderBy == "" {
		query.OrderBy = "name"
	}
	if query.Order == "" {
		query.Order = "asc"
	}

	baseQuery := `FROM contacts WHERE user_id = $1`
	args := []interface{}{userID}
	argIdx := 2

	if query.Search != "" {
		baseQuery += fmt.Sprintf(` AND (name ILIKE $%d OR email ILIKE $%d OR company ILIKE $%d)`, argIdx, argIdx, argIdx)
		args = append(args, "%"+query.Search+"%")
		argIdx++
	}

	if query.Company != "" {
		baseQuery += fmt.Sprintf(` AND company ILIKE $%d`, argIdx)
		args = append(args, "%"+query.Company+"%")
		argIdx++
	}

	if len(query.Tags) > 0 {
		baseQuery += fmt.Sprintf(` AND tags && $%d`, argIdx)
		args = append(args, pq.Array(query.Tags))
		argIdx++
	}

	if query.Favorites {
		baseQuery += ` AND is_favorite = true`
	}

	if query.Provider != "" {
		baseQuery += fmt.Sprintf(` AND provider = $%d`, argIdx)
		args = append(args, query.Provider)
		argIdx++
	}

	// Count
	var total int
	countQuery := `SELECT COUNT(*) ` + baseQuery
	if err := a.db.QueryRowxContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Validate order by
	validOrderBy := map[string]bool{
		"name": true, "email": true, "company": true,
		"relationship_score": true, "last_interaction_at": true,
		"created_at": true, "updated_at": true,
	}
	if !validOrderBy[query.OrderBy] {
		query.OrderBy = "name"
	}
	if query.Order != "asc" && query.Order != "desc" {
		query.Order = "asc"
	}

	selectQuery := fmt.Sprintf(`SELECT * %s ORDER BY %s %s LIMIT $%d OFFSET $%d`,
		baseQuery, query.OrderBy, query.Order, argIdx, argIdx+1)
	args = append(args, query.Limit, query.Offset)

	rows, err := a.db.QueryxContext(ctx, selectQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var contacts []*out.ContactEntity
	for rows.Next() {
		var row contactRow
		if err := rows.StructScan(&row); err != nil {
			return nil, 0, err
		}
		entity := row.toEntity()
		a.decryptContactEntity(entity)
		contacts = append(contacts, entity)
	}

	return contacts, total, nil
}

// UpdateInteraction updates interaction stats.
func (a *ContactAdapter) UpdateInteraction(ctx context.Context, userID uuid.UUID, contactID int64) error {
	query := `
		UPDATE contacts
		SET interaction_count = interaction_count + 1,
			last_interaction_at = NOW(),
			updated_at = NOW()
		WHERE id = $1 AND user_id = $2
	`
	_, err := a.db.ExecContext(ctx, query, contactID, userID)
	return err
}

// UpdateRelationshipScore updates relationship score.
func (a *ContactAdapter) UpdateRelationshipScore(ctx context.Context, userID uuid.UUID, contactID int64, score int16) error {
	query := `
		UPDATE contacts
		SET relationship_score = $1, updated_at = NOW()
		WHERE id = $2 AND user_id = $3
	`
	_, err := a.db.ExecContext(ctx, query, score, contactID, userID)
	return err
}

// Upsert upserts a contact from provider.
func (a *ContactAdapter) Upsert(ctx context.Context, contact *out.ContactEntity) error {
	// Encrypt sensitive fields
	encryptedPhone := a.encryptField(contact.Phone)

	query := `
		INSERT INTO contacts (
			user_id, provider, provider_id, name, email, phone, photo_url,
			company, job_title, department, synced_at
		) VALUES (
			$1, $2, $3, $4, NULLIF($5, ''), NULLIF($6, ''), NULLIF($7, ''),
			NULLIF($8, ''), NULLIF($9, ''), NULLIF($10, ''), NOW()
		)
		ON CONFLICT (user_id, provider, provider_id) DO UPDATE SET
			name = EXCLUDED.name,
			email = COALESCE(EXCLUDED.email, contacts.email),
			phone = COALESCE(EXCLUDED.phone, contacts.phone),
			photo_url = COALESCE(EXCLUDED.photo_url, contacts.photo_url),
			company = COALESCE(EXCLUDED.company, contacts.company),
			job_title = COALESCE(EXCLUDED.job_title, contacts.job_title),
			department = COALESCE(EXCLUDED.department, contacts.department),
			synced_at = NOW(),
			updated_at = NOW()
		RETURNING id, created_at, updated_at
	`

	return a.db.QueryRowxContext(ctx, query,
		contact.UserID,
		contact.Provider,
		contact.ProviderID,
		contact.Name,
		contact.Email,
		encryptedPhone,
		contact.PhotoURL,
		contact.Company,
		contact.JobTitle,
		contact.Department,
	).Scan(&contact.ID, &contact.CreatedAt, &contact.UpdatedAt)
}

// =============================================================================
// MailContactRepository Implementation
// =============================================================================

// GetContactByEmail gets contact info by email for mail enrichment.
func (a *ContactAdapter) GetContactByEmail(ctx context.Context, userID uuid.UUID, email string) (*out.MailContactInfo, error) {
	query := `
		SELECT id, name, email, photo_url, company, job_title, is_favorite,
			   relationship_score >= 80 AS is_vip
		FROM contacts
		WHERE email = $1 AND user_id = $2
	`

	var info struct {
		ID         int64          `db:"id"`
		Name       string         `db:"name"`
		Email      sql.NullString `db:"email"`
		PhotoURL   sql.NullString `db:"photo_url"`
		Company    sql.NullString `db:"company"`
		JobTitle   sql.NullString `db:"job_title"`
		IsFavorite bool           `db:"is_favorite"`
		IsVIP      bool           `db:"is_vip"`
	}

	err := a.db.QueryRowxContext(ctx, query, email, userID).StructScan(&info)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	result := &out.MailContactInfo{
		ContactID:  info.ID,
		Name:       info.Name,
		IsFavorite: info.IsFavorite,
		IsVIP:      info.IsVIP,
	}

	if info.Email.Valid {
		result.Email = info.Email.String
	}
	if info.PhotoURL.Valid {
		result.PhotoURL = info.PhotoURL.String
	}
	if info.Company.Valid {
		result.Company = info.Company.String
	}
	if info.JobTitle.Valid {
		result.JobTitle = info.JobTitle.String
	}

	return result, nil
}

// BulkGetContactsByEmail gets multiple contacts by email for mail enrichment.
func (a *ContactAdapter) BulkGetContactsByEmail(ctx context.Context, userID uuid.UUID, emails []string) (map[string]*out.MailContactInfo, error) {
	if len(emails) == 0 {
		return make(map[string]*out.MailContactInfo), nil
	}

	query := `
		SELECT id, name, email, photo_url, company, job_title, is_favorite,
			   relationship_score >= 80 AS is_vip
		FROM contacts
		WHERE email = ANY($1) AND user_id = $2
	`

	rows, err := a.db.QueryxContext(ctx, query, pq.Array(emails), userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]*out.MailContactInfo)
	for rows.Next() {
		var info struct {
			ID         int64          `db:"id"`
			Name       string         `db:"name"`
			Email      sql.NullString `db:"email"`
			PhotoURL   sql.NullString `db:"photo_url"`
			Company    sql.NullString `db:"company"`
			JobTitle   sql.NullString `db:"job_title"`
			IsFavorite bool           `db:"is_favorite"`
			IsVIP      bool           `db:"is_vip"`
		}

		if err := rows.StructScan(&info); err != nil {
			return nil, err
		}

		if info.Email.Valid {
			result[info.Email.String] = &out.MailContactInfo{
				ContactID:  info.ID,
				Name:       info.Name,
				Email:      info.Email.String,
				PhotoURL:   info.PhotoURL.String,
				Company:    info.Company.String,
				JobTitle:   info.JobTitle.String,
				IsFavorite: info.IsFavorite,
				IsVIP:      info.IsVIP,
			}
		}
	}

	return result, nil
}

// LinkMailToContact links a mail to a contact.
func (a *ContactAdapter) LinkMailToContact(ctx context.Context, mailID int64, contactID int64) error {
	query := `UPDATE emails SET contact_id = $1 WHERE id = $2`
	_, err := a.db.ExecContext(ctx, query, contactID, mailID)
	return err
}

// UpdateContactInteraction updates contact interaction when mail is sent/received.
func (a *ContactAdapter) UpdateContactInteraction(ctx context.Context, userID uuid.UUID, email string) error {
	query := `
		UPDATE contacts
		SET interaction_count = interaction_count + 1,
			last_interaction_at = NOW(),
			updated_at = NOW()
		WHERE email = $1 AND user_id = $2
	`
	_, err := a.db.ExecContext(ctx, query, email, userID)
	return err
}

// Ensure ContactAdapter implements out.ContactRepository
var _ out.ContactRepository = (*ContactAdapter)(nil)

// Ensure ContactAdapter implements out.MailContactRepository
var _ out.MailContactRepository = (*ContactAdapter)(nil)

// =============================================================================
// domain.ContactRepository Implementation (for service layer)
// =============================================================================

// DomainGetByID implements domain.ContactRepository.GetByID
func (a *ContactAdapter) DomainGetByID(id int64) (*domain.Contact, error) {
	query := `SELECT * FROM contacts WHERE id = $1`

	var row contactRow
	err := a.db.QueryRowx(query, id).StructScan(&row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("contact not found")
		}
		return nil, err
	}

	contact := row.toDomain()
	a.decryptDomainContact(contact)
	return contact, nil
}

// DomainGetByEmail implements domain.ContactRepository.GetByEmail
func (a *ContactAdapter) DomainGetByEmail(userID uuid.UUID, email string) (*domain.Contact, error) {
	query := `SELECT * FROM contacts WHERE email = $1 AND user_id = $2`

	var row contactRow
	err := a.db.QueryRowx(query, email, userID).StructScan(&row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	contact := row.toDomain()
	a.decryptDomainContact(contact)
	return contact, nil
}

// DomainList implements domain.ContactRepository.List
func (a *ContactAdapter) DomainList(filter *domain.ContactFilter) ([]*domain.Contact, int, error) {
	if filter == nil {
		filter = &domain.ContactFilter{}
	}
	if filter.Limit <= 0 || filter.Limit > 100 {
		filter.Limit = 50
	}

	baseQuery := `FROM contacts WHERE user_id = $1`
	args := []interface{}{filter.UserID}
	argIdx := 2

	if filter.Search != nil && *filter.Search != "" {
		baseQuery += fmt.Sprintf(` AND (name ILIKE $%d OR email ILIKE $%d OR company ILIKE $%d)`, argIdx, argIdx, argIdx)
		args = append(args, "%"+*filter.Search+"%")
		argIdx++
	}

	if filter.Company != nil && *filter.Company != "" {
		baseQuery += fmt.Sprintf(` AND company ILIKE $%d`, argIdx)
		args = append(args, "%"+*filter.Company+"%")
		argIdx++
	}

	if len(filter.Tags) > 0 {
		baseQuery += fmt.Sprintf(` AND tags && $%d`, argIdx)
		args = append(args, pq.Array(filter.Tags))
		argIdx++
	}

	// Count
	var total int
	countQuery := `SELECT COUNT(*) ` + baseQuery
	if err := a.db.QueryRowx(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	selectQuery := fmt.Sprintf(`SELECT * %s ORDER BY name ASC LIMIT $%d OFFSET $%d`,
		baseQuery, argIdx, argIdx+1)
	args = append(args, filter.Limit, filter.Offset)

	rows, err := a.db.Queryx(selectQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var contacts []*domain.Contact
	for rows.Next() {
		var row contactRow
		if err := rows.StructScan(&row); err != nil {
			return nil, 0, err
		}
		c := row.toDomain()
		a.decryptDomainContact(c)
		contacts = append(contacts, c)
	}

	return contacts, total, nil
}

// DomainCreate implements domain.ContactRepository.Create
func (a *ContactAdapter) DomainCreate(contact *domain.Contact) error {
	// Encrypt sensitive fields
	encryptedPhone := a.encryptField(contact.Phone)
	encryptedNotes := a.encryptField(contact.Notes)

	query := `
		INSERT INTO contacts (
			user_id, provider, provider_id, name, email, phone, photo_url,
			company, job_title, department, notes, tags, is_favorite
		) VALUES (
			$1, NULLIF($2, ''), NULLIF($3, ''), $4, NULLIF($5, ''), NULLIF($6, ''),
			NULLIF($7, ''), NULLIF($8, ''), NULLIF($9, ''), NULLIF($10, ''),
			NULLIF($11, ''), $12, $13
		)
		RETURNING id, created_at, updated_at
	`

	return a.db.QueryRowx(query,
		contact.UserID,
		contact.Provider,
		contact.ProviderID,
		contact.Name,
		contact.Email,
		encryptedPhone,
		contact.PhotoURL,
		contact.Company,
		contact.JobTitle,
		contact.Department,
		encryptedNotes,
		pq.Array(contact.Tags),
		contact.IsFavorite,
	).Scan(&contact.ID, &contact.CreatedAt, &contact.UpdatedAt)
}

// DomainUpdate implements domain.ContactRepository.Update
func (a *ContactAdapter) DomainUpdate(contact *domain.Contact) error {
	// Encrypt sensitive fields
	encryptedPhone := a.encryptField(contact.Phone)
	encryptedNotes := a.encryptField(contact.Notes)

	query := `
		UPDATE contacts SET
			name = $1, email = NULLIF($2, ''), phone = NULLIF($3, ''),
			photo_url = NULLIF($4, ''), company = NULLIF($5, ''),
			job_title = NULLIF($6, ''), department = NULLIF($7, ''),
			notes = NULLIF($8, ''), tags = $9, is_favorite = $10,
			updated_at = NOW()
		WHERE id = $11
	`

	result, err := a.db.Exec(query,
		contact.Name, contact.Email, encryptedPhone,
		contact.PhotoURL, contact.Company, contact.JobTitle,
		contact.Department, encryptedNotes, pq.Array(contact.Tags),
		contact.IsFavorite, contact.ID,
	)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("contact not found")
	}
	return nil
}

// DomainDelete implements domain.ContactRepository.Delete
func (a *ContactAdapter) DomainDelete(id int64) error {
	query := `DELETE FROM contacts WHERE id = $1`

	result, err := a.db.Exec(query, id)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("contact not found")
	}
	return nil
}

// DomainGetCompanyByID implements domain.ContactRepository.GetCompanyByID
func (a *ContactAdapter) DomainGetCompanyByID(id int64) (*domain.Company, error) {
	query := `SELECT * FROM companies WHERE id = $1`

	var row struct {
		ID          int64          `db:"id"`
		UserID      uuid.UUID      `db:"user_id"`
		Name        string         `db:"name"`
		Domain      sql.NullString `db:"domain"`
		Industry    sql.NullString `db:"industry"`
		Size        sql.NullString `db:"size"`
		Website     sql.NullString `db:"website"`
		Description sql.NullString `db:"description"`
		LogoURL     sql.NullString `db:"logo_url"`
		CreatedAt   time.Time      `db:"created_at"`
		UpdatedAt   time.Time      `db:"updated_at"`
	}

	err := a.db.QueryRowx(query, id).StructScan(&row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("company not found")
		}
		return nil, err
	}

	company := &domain.Company{
		ID:        row.ID,
		UserID:    row.UserID,
		Name:      row.Name,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}

	if row.Domain.Valid {
		company.Domain = &row.Domain.String
	}
	if row.Industry.Valid {
		company.Industry = &row.Industry.String
	}
	if row.Size.Valid {
		company.Size = &row.Size.String
	}
	if row.Website.Valid {
		company.Website = &row.Website.String
	}
	if row.Description.Valid {
		company.Description = &row.Description.String
	}
	if row.LogoURL.Valid {
		company.LogoURL = &row.LogoURL.String
	}

	return company, nil
}

// DomainGetCompanyByDomain implements domain.ContactRepository.GetCompanyByDomain
func (a *ContactAdapter) DomainGetCompanyByDomain(userID uuid.UUID, domainName string) (*domain.Company, error) {
	query := `SELECT * FROM companies WHERE domain = $1 AND user_id = $2`

	var row struct {
		ID          int64          `db:"id"`
		UserID      uuid.UUID      `db:"user_id"`
		Name        string         `db:"name"`
		Domain      sql.NullString `db:"domain"`
		Industry    sql.NullString `db:"industry"`
		Size        sql.NullString `db:"size"`
		Website     sql.NullString `db:"website"`
		Description sql.NullString `db:"description"`
		LogoURL     sql.NullString `db:"logo_url"`
		CreatedAt   time.Time      `db:"created_at"`
		UpdatedAt   time.Time      `db:"updated_at"`
	}

	err := a.db.QueryRowx(query, domainName, userID).StructScan(&row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	company := &domain.Company{
		ID:        row.ID,
		UserID:    row.UserID,
		Name:      row.Name,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}

	if row.Domain.Valid {
		company.Domain = &row.Domain.String
	}
	if row.Industry.Valid {
		company.Industry = &row.Industry.String
	}
	if row.Size.Valid {
		company.Size = &row.Size.String
	}
	if row.Website.Valid {
		company.Website = &row.Website.String
	}
	if row.Description.Valid {
		company.Description = &row.Description.String
	}
	if row.LogoURL.Valid {
		company.LogoURL = &row.LogoURL.String
	}

	return company, nil
}

// DomainListCompanies implements domain.ContactRepository.ListCompanies
func (a *ContactAdapter) DomainListCompanies(userID uuid.UUID, limit, offset int) ([]*domain.Company, int, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	// Count
	var total int
	if err := a.db.QueryRowx(`SELECT COUNT(*) FROM companies WHERE user_id = $1`, userID).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `SELECT * FROM companies WHERE user_id = $1 ORDER BY name ASC LIMIT $2 OFFSET $3`
	rows, err := a.db.Queryx(query, userID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var companies []*domain.Company
	for rows.Next() {
		var row struct {
			ID          int64          `db:"id"`
			UserID      uuid.UUID      `db:"user_id"`
			Name        string         `db:"name"`
			Domain      sql.NullString `db:"domain"`
			Industry    sql.NullString `db:"industry"`
			Size        sql.NullString `db:"size"`
			Website     sql.NullString `db:"website"`
			Description sql.NullString `db:"description"`
			LogoURL     sql.NullString `db:"logo_url"`
			CreatedAt   time.Time      `db:"created_at"`
			UpdatedAt   time.Time      `db:"updated_at"`
		}

		if err := rows.StructScan(&row); err != nil {
			return nil, 0, err
		}

		company := &domain.Company{
			ID:        row.ID,
			UserID:    row.UserID,
			Name:      row.Name,
			CreatedAt: row.CreatedAt,
			UpdatedAt: row.UpdatedAt,
		}

		if row.Domain.Valid {
			company.Domain = &row.Domain.String
		}
		if row.Industry.Valid {
			company.Industry = &row.Industry.String
		}
		if row.Size.Valid {
			company.Size = &row.Size.String
		}
		if row.Website.Valid {
			company.Website = &row.Website.String
		}
		if row.Description.Valid {
			company.Description = &row.Description.String
		}
		if row.LogoURL.Valid {
			company.LogoURL = &row.LogoURL.String
		}

		companies = append(companies, company)
	}

	return companies, total, nil
}

// DomainCreateCompany implements domain.ContactRepository.CreateCompany
func (a *ContactAdapter) DomainCreateCompany(company *domain.Company) error {
	query := `
		INSERT INTO companies (user_id, name, domain, industry, size, website, description, logo_url)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, created_at, updated_at
	`

	return a.db.QueryRowx(query,
		company.UserID,
		company.Name,
		company.Domain,
		company.Industry,
		company.Size,
		company.Website,
		company.Description,
		company.LogoURL,
	).Scan(&company.ID, &company.CreatedAt, &company.UpdatedAt)
}

// DomainUpdateCompany implements domain.ContactRepository.UpdateCompany
func (a *ContactAdapter) DomainUpdateCompany(company *domain.Company) error {
	query := `
		UPDATE companies SET
			name = $1, domain = $2, industry = $3, size = $4,
			website = $5, description = $6, logo_url = $7,
			updated_at = NOW()
		WHERE id = $8
	`

	result, err := a.db.Exec(query,
		company.Name, company.Domain, company.Industry, company.Size,
		company.Website, company.Description, company.LogoURL, company.ID,
	)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("company not found")
	}
	return nil
}

// DomainDeleteCompany implements domain.ContactRepository.DeleteCompany
func (a *ContactAdapter) DomainDeleteCompany(id int64) error {
	query := `DELETE FROM companies WHERE id = $1`

	result, err := a.db.Exec(query, id)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("company not found")
	}
	return nil
}
