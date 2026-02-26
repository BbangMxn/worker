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

	"github.com/goccy/go-json"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// =============================================================================
// MongoDB Report Adapter
// =============================================================================

const (
	collectionReports = "mail_reports"

	// Compression threshold for reports
	reportCompressionThreshold = 512 // 512 bytes
)

// ReportAdapter implements out.ReportRepository using MongoDB.
type ReportAdapter struct {
	db         *mongo.Database
	collection *mongo.Collection
}

// NewReportAdapter creates a new MongoDB report adapter.
func NewReportAdapter(db *mongo.Database) *ReportAdapter {
	collection := db.Collection(collectionReports)
	return &ReportAdapter{
		db:         db,
		collection: collection,
	}
}

// EnsureIndexes creates necessary indexes for the collection.
func (a *ReportAdapter) EnsureIndexes(ctx context.Context) error {
	indexes := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "id", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys: bson.D{{Key: "user_id", Value: 1}},
		},
		{
			Keys: bson.D{
				{Key: "user_id", Value: 1},
				{Key: "type", Value: 1},
				{Key: "period_start", Value: -1},
			},
		},
		{
			Keys:    bson.D{{Key: "expires_at", Value: 1}},
			Options: options.Index().SetExpireAfterSeconds(0), // TTL index
		},
		{
			Keys: bson.D{{Key: "created_at", Value: -1}},
		},
	}

	_, err := a.collection.Indexes().CreateMany(ctx, indexes)
	return err
}

// =============================================================================
// Document Model
// =============================================================================

// reportDocument represents the MongoDB document structure.
type reportDocument struct {
	ID     string `bson:"id"`
	UserID string `bson:"user_id"`

	// Report type and period
	Type        string    `bson:"type"`
	PeriodStart time.Time `bson:"period_start"`
	PeriodEnd   time.Time `bson:"period_end"`

	// Content (potentially compressed JSON)
	Content      []byte `bson:"content"`
	IsCompressed bool   `bson:"is_compressed"`

	// Status
	Status       string `bson:"status"`
	ErrorMessage string `bson:"error_message,omitempty"`

	// Size info
	OriginalSize   int64 `bson:"original_size"`
	CompressedSize int64 `bson:"compressed_size"`

	// Timestamps
	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
	ExpiresAt time.Time `bson:"expires_at"`
}

// =============================================================================
// Single Operations
// =============================================================================

// Save saves a report to MongoDB.
func (a *ReportAdapter) Save(ctx context.Context, report *out.ReportEntity) error {
	doc, err := a.toDocument(report)
	if err != nil {
		return fmt.Errorf("failed to convert report to document: %w", err)
	}

	opts := options.Replace().SetUpsert(true)
	filter := bson.M{"id": report.ID}

	_, err = a.collection.ReplaceOne(ctx, filter, doc, opts)
	if err != nil {
		return fmt.Errorf("failed to save report: %w", err)
	}

	return nil
}

// GetByID retrieves a report by ID.
func (a *ReportAdapter) GetByID(ctx context.Context, id string) (*out.ReportEntity, error) {
	var doc reportDocument
	filter := bson.M{"id": id}

	err := a.collection.FindOne(ctx, filter).Decode(&doc)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get report: %w", err)
	}

	return a.toEntity(&doc)
}

// Delete deletes a report from MongoDB.
func (a *ReportAdapter) Delete(ctx context.Context, id string) error {
	filter := bson.M{"id": id}

	_, err := a.collection.DeleteOne(ctx, filter)
	if err != nil {
		return fmt.Errorf("failed to delete report: %w", err)
	}

	return nil
}

// =============================================================================
// Query Operations
// =============================================================================

// GetByUserAndPeriod retrieves a report by user, type, and period start.
func (a *ReportAdapter) GetByUserAndPeriod(ctx context.Context, userID uuid.UUID, reportType out.ReportType, periodStart time.Time) (*out.ReportEntity, error) {
	filter := bson.M{
		"user_id":      userID.String(),
		"type":         string(reportType),
		"period_start": periodStart,
	}

	var doc reportDocument
	err := a.collection.FindOne(ctx, filter).Decode(&doc)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get report by period: %w", err)
	}

	return a.toEntity(&doc)
}

// ListByUser retrieves reports for a user with options.
func (a *ReportAdapter) ListByUser(ctx context.Context, userID uuid.UUID, opts *out.ReportListOptions) ([]*out.ReportEntity, error) {
	filter := bson.M{"user_id": userID.String()}

	if opts != nil {
		if opts.Type != nil {
			filter["type"] = string(*opts.Type)
		}
		if opts.Status != nil {
			filter["status"] = string(*opts.Status)
		}
		if opts.PeriodFrom != nil || opts.PeriodTo != nil {
			periodFilter := bson.M{}
			if opts.PeriodFrom != nil {
				periodFilter["$gte"] = *opts.PeriodFrom
			}
			if opts.PeriodTo != nil {
				periodFilter["$lte"] = *opts.PeriodTo
			}
			filter["period_start"] = periodFilter
		}
	}

	findOpts := options.Find().SetSort(bson.D{{Key: "period_start", Value: -1}})
	if opts != nil {
		if opts.Limit > 0 {
			findOpts.SetLimit(int64(opts.Limit))
		}
		if opts.Offset > 0 {
			findOpts.SetSkip(int64(opts.Offset))
		}
	}

	cursor, err := a.collection.Find(ctx, filter, findOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to list reports: %w", err)
	}
	defer cursor.Close(ctx)

	var reports []*out.ReportEntity
	for cursor.Next(ctx) {
		var doc reportDocument
		if err := cursor.Decode(&doc); err != nil {
			return nil, fmt.Errorf("failed to decode report: %w", err)
		}

		entity, err := a.toEntity(&doc)
		if err != nil {
			return nil, fmt.Errorf("failed to convert report: %w", err)
		}
		reports = append(reports, entity)
	}

	return reports, nil
}

// GetLatestByUser retrieves the latest report for a user by type.
func (a *ReportAdapter) GetLatestByUser(ctx context.Context, userID uuid.UUID, reportType out.ReportType) (*out.ReportEntity, error) {
	filter := bson.M{
		"user_id": userID.String(),
		"type":    string(reportType),
		"status":  string(out.ReportStatusCompleted),
	}

	findOpts := options.FindOne().SetSort(bson.D{{Key: "period_start", Value: -1}})

	var doc reportDocument
	err := a.collection.FindOne(ctx, filter, findOpts).Decode(&doc)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get latest report: %w", err)
	}

	return a.toEntity(&doc)
}

// =============================================================================
// Cleanup Operations
// =============================================================================

// DeleteExpired deletes all expired reports.
func (a *ReportAdapter) DeleteExpired(ctx context.Context) (int64, error) {
	filter := bson.M{"expires_at": bson.M{"$lt": time.Now()}}

	result, err := a.collection.DeleteMany(ctx, filter)
	if err != nil {
		return 0, fmt.Errorf("failed to delete expired reports: %w", err)
	}

	return result.DeletedCount, nil
}

// DeleteByUser deletes all reports for a user.
func (a *ReportAdapter) DeleteByUser(ctx context.Context, userID uuid.UUID) (int64, error) {
	filter := bson.M{"user_id": userID.String()}

	result, err := a.collection.DeleteMany(ctx, filter)
	if err != nil {
		return 0, fmt.Errorf("failed to delete user reports: %w", err)
	}

	return result.DeletedCount, nil
}

// DeleteOlderThan deletes all reports older than the specified time.
func (a *ReportAdapter) DeleteOlderThan(ctx context.Context, before time.Time) (int64, error) {
	filter := bson.M{"created_at": bson.M{"$lt": before}}

	result, err := a.collection.DeleteMany(ctx, filter)
	if err != nil {
		return 0, fmt.Errorf("failed to delete old reports: %w", err)
	}

	return result.DeletedCount, nil
}

// =============================================================================
// Stats
// =============================================================================

// GetStorageStats returns storage statistics.
func (a *ReportAdapter) GetStorageStats(ctx context.Context) (*out.ReportStorageStats, error) {
	pipeline := []bson.M{
		{
			"$group": bson.M{
				"_id":             nil,
				"total_count":     bson.M{"$sum": 1},
				"total_size":      bson.M{"$sum": "$original_size"},
				"compressed_size": bson.M{"$sum": "$compressed_size"},
				"oldest_entry":    bson.M{"$min": "$created_at"},
				"newest_entry":    bson.M{"$max": "$created_at"},
			},
		},
	}

	cursor, err := a.collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("failed to get storage stats: %w", err)
	}
	defer cursor.Close(ctx)

	stats := &out.ReportStorageStats{
		ByType: make(map[out.ReportType]int64),
	}

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

	// Count by type
	typePipeline := []bson.M{
		{
			"$group": bson.M{
				"_id":   "$type",
				"count": bson.M{"$sum": 1},
			},
		},
	}

	typeCursor, err := a.collection.Aggregate(ctx, typePipeline)
	if err == nil {
		defer typeCursor.Close(ctx)
		for typeCursor.Next(ctx) {
			var result struct {
				Type  string `bson:"_id"`
				Count int64  `bson:"count"`
			}
			if err := typeCursor.Decode(&result); err == nil {
				stats.ByType[out.ReportType(result.Type)] = result.Count
			}
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

// =============================================================================
// Conversion Helpers
// =============================================================================

func (a *ReportAdapter) toDocument(entity *out.ReportEntity) (*reportDocument, error) {
	// Serialize content to JSON
	var contentBytes []byte
	var originalSize int64
	var compressedSize int64
	var isCompressed bool

	if entity.Content != nil {
		var err error
		contentBytes, err = json.Marshal(entity.Content)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal content: %w", err)
		}
		originalSize = int64(len(contentBytes))

		// Compress if content is large enough
		if originalSize > reportCompressionThreshold {
			compressed, err := compress(contentBytes)
			if err != nil {
				return nil, fmt.Errorf("failed to compress content: %w", err)
			}
			contentBytes = compressed
			isCompressed = true
			compressedSize = int64(len(compressed))
		} else {
			compressedSize = originalSize
		}
	}

	return &reportDocument{
		ID:             entity.ID,
		UserID:         entity.UserID.String(),
		Type:           string(entity.Type),
		PeriodStart:    entity.PeriodStart,
		PeriodEnd:      entity.PeriodEnd,
		Content:        contentBytes,
		IsCompressed:   isCompressed,
		Status:         string(entity.Status),
		ErrorMessage:   entity.ErrorMessage,
		OriginalSize:   originalSize,
		CompressedSize: compressedSize,
		CreatedAt:      entity.CreatedAt,
		UpdatedAt:      entity.UpdatedAt,
		ExpiresAt:      entity.ExpiresAt,
	}, nil
}

func (a *ReportAdapter) toEntity(doc *reportDocument) (*out.ReportEntity, error) {
	userID, err := uuid.Parse(doc.UserID)
	if err != nil {
		return nil, fmt.Errorf("failed to parse user ID: %w", err)
	}

	var content *out.ReportContent
	if len(doc.Content) > 0 {
		contentBytes := doc.Content

		// Decompress if needed
		if doc.IsCompressed {
			decompressed, err := decompress(doc.Content)
			if err != nil {
				return nil, fmt.Errorf("failed to decompress content: %w", err)
			}
			contentBytes = decompressed
		}

		content = &out.ReportContent{}
		if err := json.Unmarshal(contentBytes, content); err != nil {
			return nil, fmt.Errorf("failed to unmarshal content: %w", err)
		}
	}

	return &out.ReportEntity{
		ID:             doc.ID,
		UserID:         userID,
		Type:           out.ReportType(doc.Type),
		PeriodStart:    doc.PeriodStart,
		PeriodEnd:      doc.PeriodEnd,
		Content:        content,
		Status:         out.ReportStatus(doc.Status),
		ErrorMessage:   doc.ErrorMessage,
		IsCompressed:   doc.IsCompressed,
		OriginalSize:   doc.OriginalSize,
		CompressedSize: doc.CompressedSize,
		CreatedAt:      doc.CreatedAt,
		UpdatedAt:      doc.UpdatedAt,
		ExpiresAt:      doc.ExpiresAt,
	}, nil
}

// =============================================================================
// Compression Helpers (reuse from mail_body_adapter)
// =============================================================================

func compressReport(data []byte) ([]byte, error) {
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

func decompressReport(data []byte) ([]byte, error) {
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

var _ out.ReportRepository = (*ReportAdapter)(nil)
