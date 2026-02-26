// Package mongodb implements MongoDB adapters for the application.
package mongodb

import (
	"worker_server/core/port/out"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// =============================================================================
// MongoDB Mail Body Adapter
// =============================================================================

const (
	collectionMailBodies = "mail_bodies"

	// Compression threshold - only compress if content is larger than this
	compressionThreshold = 1024 // 1KB
)

// MailBodyAdapter implements out.EmailBodyRepository using MongoDB.
type MailBodyAdapter struct {
	db         *mongo.Database
	collection *mongo.Collection
}

// NewMailBodyAdapter creates a new MongoDB mail body adapter.
func NewMailBodyAdapter(db *mongo.Database) *MailBodyAdapter {
	collection := db.Collection(collectionMailBodies)
	return &MailBodyAdapter{
		db:         db,
		collection: collection,
	}
}

// EnsureIndexes creates necessary indexes for the collection.
func (a *MailBodyAdapter) EnsureIndexes(ctx context.Context) error {
	indexes := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "email_id", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys: bson.D{{Key: "connection_id", Value: 1}},
		},
		{
			Keys:    bson.D{{Key: "expires_at", Value: 1}},
			Options: options.Index().SetExpireAfterSeconds(0), // TTL index
		},
		{
			Keys: bson.D{{Key: "cached_at", Value: 1}},
		},
	}

	_, err := a.collection.Indexes().CreateMany(ctx, indexes)
	return err
}

// =============================================================================
// Document Model
// =============================================================================

// mailBodyDocument represents the MongoDB document structure.
type mailBodyDocument struct {
	EmailID      int64  `bson:"email_id"`
	ConnectionID int64  `bson:"connection_id"`
	ExternalID   string `bson:"external_id"`

	// Content (potentially compressed)
	HTML         []byte `bson:"html"`
	Text         []byte `bson:"text"`
	IsCompressed bool   `bson:"is_compressed"`

	// Attachments metadata
	Attachments []attachmentDocument `bson:"attachments,omitempty"`

	// Size info
	OriginalSize   int64 `bson:"original_size"`
	CompressedSize int64 `bson:"compressed_size"`

	// Cache metadata
	CachedAt  time.Time `bson:"cached_at"`
	ExpiresAt time.Time `bson:"expires_at"`
	TTLDays   int       `bson:"ttl_days"`
}

type attachmentDocument struct {
	ID        string `bson:"id"`
	Name      string `bson:"name"`
	MimeType  string `bson:"mime_type"`
	Size      int64  `bson:"size"`
	ContentID string `bson:"content_id,omitempty"`
	IsInline  bool   `bson:"is_inline"`
	URL       string `bson:"url,omitempty"`
}

// =============================================================================
// Single Operations
// =============================================================================

// SaveBody saves a mail body to MongoDB.
func (a *MailBodyAdapter) SaveBody(ctx context.Context, body *out.MailBodyEntity) error {
	doc, err := a.toDocument(body)
	if err != nil {
		return fmt.Errorf("failed to convert body to document: %w", err)
	}

	opts := options.Replace().SetUpsert(true)
	filter := bson.M{"email_id": body.EmailID}

	_, err = a.collection.ReplaceOne(ctx, filter, doc, opts)
	if err != nil {
		return fmt.Errorf("failed to save mail body: %w", err)
	}

	return nil
}

// GetBody retrieves a mail body from MongoDB.
func (a *MailBodyAdapter) GetBody(ctx context.Context, emailID int64) (*out.MailBodyEntity, error) {
	var doc mailBodyDocument
	filter := bson.M{"email_id": emailID}

	err := a.collection.FindOne(ctx, filter).Decode(&doc)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get mail body: %w", err)
	}

	return a.toEntity(&doc)
}

// DeleteBody deletes a mail body from MongoDB.
func (a *MailBodyAdapter) DeleteBody(ctx context.Context, emailID int64) error {
	filter := bson.M{"email_id": emailID}

	_, err := a.collection.DeleteOne(ctx, filter)
	if err != nil {
		return fmt.Errorf("failed to delete mail body: %w", err)
	}

	return nil
}

// ExistsBody checks if a mail body exists in MongoDB.
func (a *MailBodyAdapter) ExistsBody(ctx context.Context, emailID int64) (bool, error) {
	filter := bson.M{"email_id": emailID}

	count, err := a.collection.CountDocuments(ctx, filter, options.Count().SetLimit(1))
	if err != nil {
		return false, fmt.Errorf("failed to check body existence: %w", err)
	}

	return count > 0, nil
}

// IsCached checks if a mail body is cached (alias for ExistsBody).
func (a *MailBodyAdapter) IsCached(ctx context.Context, emailID int64) (bool, error) {
	return a.ExistsBody(ctx, emailID)
}

// =============================================================================
// Bulk Operations
// =============================================================================

// BulkSaveBody saves multiple mail bodies to MongoDB.
func (a *MailBodyAdapter) BulkSaveBody(ctx context.Context, bodies []*out.MailBodyEntity) error {
	if len(bodies) == 0 {
		return nil
	}

	models := make([]mongo.WriteModel, 0, len(bodies))
	for _, body := range bodies {
		doc, err := a.toDocument(body)
		if err != nil {
			return fmt.Errorf("failed to convert body %d: %w", body.EmailID, err)
		}

		filter := bson.M{"email_id": body.EmailID}
		model := mongo.NewReplaceOneModel().
			SetFilter(filter).
			SetReplacement(doc).
			SetUpsert(true)
		models = append(models, model)
	}

	opts := options.BulkWrite().SetOrdered(false)
	_, err := a.collection.BulkWrite(ctx, models, opts)
	if err != nil {
		return fmt.Errorf("failed to bulk save mail bodies: %w", err)
	}

	return nil
}

// BulkDeleteBody deletes multiple mail bodies from MongoDB.
func (a *MailBodyAdapter) BulkDeleteBody(ctx context.Context, emailIDs []int64) error {
	if len(emailIDs) == 0 {
		return nil
	}

	filter := bson.M{"email_id": bson.M{"$in": emailIDs}}

	_, err := a.collection.DeleteMany(ctx, filter)
	if err != nil {
		return fmt.Errorf("failed to bulk delete mail bodies: %w", err)
	}

	return nil
}

// BulkGetBody retrieves multiple mail bodies from MongoDB.
func (a *MailBodyAdapter) BulkGetBody(ctx context.Context, emailIDs []int64) (map[int64]*out.MailBodyEntity, error) {
	if len(emailIDs) == 0 {
		return make(map[int64]*out.MailBodyEntity), nil
	}

	filter := bson.M{"email_id": bson.M{"$in": emailIDs}}

	cursor, err := a.collection.Find(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to bulk get mail bodies: %w", err)
	}
	defer cursor.Close(ctx)

	result := make(map[int64]*out.MailBodyEntity, len(emailIDs))
	for cursor.Next(ctx) {
		var doc mailBodyDocument
		if err := cursor.Decode(&doc); err != nil {
			return nil, fmt.Errorf("failed to decode mail body: %w", err)
		}

		entity, err := a.toEntity(&doc)
		if err != nil {
			return nil, fmt.Errorf("failed to convert body %d: %w", doc.EmailID, err)
		}
		result[entity.EmailID] = entity
	}

	return result, nil
}

// =============================================================================
// Cleanup Operations
// =============================================================================

// DeleteExpired deletes all expired mail bodies.
func (a *MailBodyAdapter) DeleteExpired(ctx context.Context) (int64, error) {
	filter := bson.M{"expires_at": bson.M{"$lt": time.Now()}}

	result, err := a.collection.DeleteMany(ctx, filter)
	if err != nil {
		return 0, fmt.Errorf("failed to delete expired bodies: %w", err)
	}

	return result.DeletedCount, nil
}

// DeleteByConnectionID deletes all mail bodies for a connection.
func (a *MailBodyAdapter) DeleteByConnectionID(ctx context.Context, connectionID int64) (int64, error) {
	filter := bson.M{"connection_id": connectionID}

	result, err := a.collection.DeleteMany(ctx, filter)
	if err != nil {
		return 0, fmt.Errorf("failed to delete bodies by connection: %w", err)
	}

	return result.DeletedCount, nil
}

// DeleteOlderThan deletes all mail bodies older than the specified time.
func (a *MailBodyAdapter) DeleteOlderThan(ctx context.Context, before time.Time) (int64, error) {
	filter := bson.M{"cached_at": bson.M{"$lt": before}}

	result, err := a.collection.DeleteMany(ctx, filter)
	if err != nil {
		return 0, fmt.Errorf("failed to delete old bodies: %w", err)
	}

	return result.DeletedCount, nil
}

// =============================================================================
// Stats
// =============================================================================

// GetStorageStats returns storage statistics.
func (a *MailBodyAdapter) GetStorageStats(ctx context.Context) (*out.BodyStorageStats, error) {
	pipeline := []bson.M{
		{
			"$group": bson.M{
				"_id":             nil,
				"total_count":     bson.M{"$sum": 1},
				"total_size":      bson.M{"$sum": "$original_size"},
				"compressed_size": bson.M{"$sum": "$compressed_size"},
				"oldest_entry":    bson.M{"$min": "$cached_at"},
				"newest_entry":    bson.M{"$max": "$cached_at"},
			},
		},
	}

	cursor, err := a.collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("failed to get storage stats: %w", err)
	}
	defer cursor.Close(ctx)

	stats := &out.BodyStorageStats{}
	if cursor.Next(ctx) {
		var result struct {
			TotalCount     int64      `bson:"total_count"`
			TotalSize      int64      `bson:"total_size"`
			CompressedSize int64      `bson:"compressed_size"`
			OldestEntry    *time.Time `bson:"oldest_entry"`
			NewestEntry    *time.Time `bson:"newest_entry"`
		}
		if err := cursor.Decode(&result); err != nil {
			return nil, fmt.Errorf("failed to decode storage stats: %w", err)
		}

		stats.TotalCount = result.TotalCount
		stats.TotalSize = result.TotalSize
		stats.CompressedSize = result.CompressedSize
		stats.OldestEntry = result.OldestEntry
		stats.NewestEntry = result.NewestEntry

		if stats.TotalSize > 0 {
			stats.AvgCompression = float64(stats.CompressedSize) / float64(stats.TotalSize)
		}
	}

	// Count expired
	expiredCount, err := a.collection.CountDocuments(ctx, bson.M{
		"expires_at": bson.M{"$lt": time.Now()},
	})
	if err == nil {
		stats.ExpiredCount = expiredCount
	}

	return stats, nil
}

// GetCompressionStats returns compression statistics.
func (a *MailBodyAdapter) GetCompressionStats(ctx context.Context) (*out.CompressionStats, error) {
	pipeline := []bson.M{
		{
			"$group": bson.M{
				"_id":            "$is_compressed",
				"count":          bson.M{"$sum": 1},
				"original_sum":   bson.M{"$sum": "$original_size"},
				"compressed_sum": bson.M{"$sum": "$compressed_size"},
			},
		},
	}

	cursor, err := a.collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("failed to get compression stats: %w", err)
	}
	defer cursor.Close(ctx)

	stats := &out.CompressionStats{}
	var totalOriginal, totalCompressed int64

	for cursor.Next(ctx) {
		var result struct {
			ID            bool  `bson:"_id"`
			Count         int64 `bson:"count"`
			OriginalSum   int64 `bson:"original_sum"`
			CompressedSum int64 `bson:"compressed_sum"`
		}
		if err := cursor.Decode(&result); err != nil {
			return nil, fmt.Errorf("failed to decode compression stats: %w", err)
		}

		if result.ID {
			stats.CompressedBodies = result.Count
		} else {
			stats.UncompressedBodies = result.Count
		}
		totalOriginal += result.OriginalSum
		totalCompressed += result.CompressedSum
	}

	stats.TotalBodies = stats.CompressedBodies + stats.UncompressedBodies
	if totalOriginal > 0 {
		stats.AvgCompressionRatio = float64(totalCompressed) / float64(totalOriginal)
		stats.TotalSaved = totalOriginal - totalCompressed
	}

	return stats, nil
}

// =============================================================================
// Conversion Helpers
// =============================================================================

func (a *MailBodyAdapter) toDocument(entity *out.MailBodyEntity) (*mailBodyDocument, error) {
	htmlBytes := []byte(entity.HTML)
	textBytes := []byte(entity.Text)
	originalSize := int64(len(htmlBytes) + len(textBytes))

	isCompressed := false
	compressedSize := originalSize

	// Compress if content is large enough
	if originalSize > compressionThreshold {
		compressedHTML, err := compress(htmlBytes)
		if err != nil {
			return nil, fmt.Errorf("failed to compress HTML: %w", err)
		}
		compressedText, err := compress(textBytes)
		if err != nil {
			return nil, fmt.Errorf("failed to compress text: %w", err)
		}

		htmlBytes = compressedHTML
		textBytes = compressedText
		isCompressed = true
		compressedSize = int64(len(htmlBytes) + len(textBytes))
	}

	attachments := make([]attachmentDocument, len(entity.Attachments))
	for i, att := range entity.Attachments {
		attachments[i] = attachmentDocument{
			ID:        att.ID,
			Name:      att.Name,
			MimeType:  att.MimeType,
			Size:      att.Size,
			ContentID: att.ContentID,
			IsInline:  att.IsInline,
			URL:       att.URL,
		}
	}

	return &mailBodyDocument{
		EmailID:        entity.EmailID,
		ConnectionID:   entity.ConnectionID,
		ExternalID:     entity.ExternalID,
		HTML:           htmlBytes,
		Text:           textBytes,
		IsCompressed:   isCompressed,
		Attachments:    attachments,
		OriginalSize:   originalSize,
		CompressedSize: compressedSize,
		CachedAt:       entity.CachedAt,
		ExpiresAt:      entity.ExpiresAt,
		TTLDays:        entity.TTLDays,
	}, nil
}

func (a *MailBodyAdapter) toEntity(doc *mailBodyDocument) (*out.MailBodyEntity, error) {
	htmlBytes := doc.HTML
	textBytes := doc.Text

	// Decompress if needed
	if doc.IsCompressed {
		var err error
		htmlBytes, err = decompress(doc.HTML)
		if err != nil {
			return nil, fmt.Errorf("failed to decompress HTML: %w", err)
		}
		textBytes, err = decompress(doc.Text)
		if err != nil {
			return nil, fmt.Errorf("failed to decompress text: %w", err)
		}
	}

	attachments := make([]out.AttachmentEntity, len(doc.Attachments))
	for i, att := range doc.Attachments {
		attachments[i] = out.AttachmentEntity{
			ID:        att.ID,
			Name:      att.Name,
			MimeType:  att.MimeType,
			Size:      att.Size,
			ContentID: att.ContentID,
			IsInline:  att.IsInline,
			URL:       att.URL,
		}
	}

	return &out.MailBodyEntity{
		EmailID:        doc.EmailID,
		ConnectionID:   doc.ConnectionID,
		ExternalID:     doc.ExternalID,
		HTML:           string(htmlBytes),
		Text:           string(textBytes),
		Attachments:    attachments,
		OriginalSize:   doc.OriginalSize,
		CompressedSize: doc.CompressedSize,
		IsCompressed:   doc.IsCompressed,
		CachedAt:       doc.CachedAt,
		ExpiresAt:      doc.ExpiresAt,
		TTLDays:        doc.TTLDays,
	}, nil
}

// =============================================================================
// Compression Helpers
// =============================================================================

func compress(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return data, nil
	}

	var buf bytes.Buffer
	writer := gzip.NewWriter(&buf)

	if _, err := writer.Write(data); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func decompress(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return data, nil
	}

	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	return io.ReadAll(reader)
}

// =============================================================================
// Interface Compliance
// =============================================================================

var _ out.EmailBodyRepository = (*MailBodyAdapter)(nil)
