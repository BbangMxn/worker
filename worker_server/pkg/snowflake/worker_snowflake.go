// Package snowflake implements Twitter's Snowflake ID generator.
//
// Snowflake ID structure (64 bits):
//
//	┌─────────┬─────────────────────┬────────────┬──────────────┐
//	│ 1 bit   │      41 bits        │  10 bits   │   12 bits    │
//	│ sign(0) │ timestamp (ms)      │ worker_id  │  sequence    │
//	└─────────┴─────────────────────┴────────────┴──────────────┘
//
// - 41 bits: milliseconds since custom epoch (~69 years)
// - 10 bits: worker/node ID (0-1023)
// - 12 bits: sequence number (0-4095 per ms)
//
// Benefits:
// - Globally unique without coordination
// - Time-sortable (lexicographic ordering = chronological)
// - High throughput: 4096 IDs/ms per worker
package snowflake

import (
	"errors"
	"sync"
	"time"
)

const (
	// Custom epoch: 2024-01-01 00:00:00 UTC
	epoch int64 = 1704067200000

	// Bit lengths
	timestampBits = 41
	workerIDBits  = 10
	sequenceBits  = 12

	// Max values
	maxWorkerID = (1 << workerIDBits) - 1 // 1023
	maxSequence = (1 << sequenceBits) - 1 // 4095

	// Bit shifts
	timestampShift = workerIDBits + sequenceBits // 22
	workerIDShift  = sequenceBits                // 12
)

var (
	ErrInvalidWorkerID = errors.New("worker ID must be between 0 and 1023")
	ErrClockMovedBack  = errors.New("clock moved backwards")
)

// Generator generates unique Snowflake IDs.
type Generator struct {
	mu       sync.Mutex
	workerID int64
	sequence int64
	lastTime int64
}

// NewGenerator creates a new Snowflake ID generator.
// workerID must be between 0 and 1023.
func NewGenerator(workerID int64) (*Generator, error) {
	if workerID < 0 || workerID > maxWorkerID {
		return nil, ErrInvalidWorkerID
	}

	return &Generator{
		workerID: workerID,
		sequence: 0,
		lastTime: 0,
	}, nil
}

// Generate generates a new unique Snowflake ID.
func (g *Generator) Generate() (int64, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	now := currentTimeMillis()

	if now < g.lastTime {
		return 0, ErrClockMovedBack
	}

	if now == g.lastTime {
		// Same millisecond, increment sequence
		g.sequence = (g.sequence + 1) & maxSequence
		if g.sequence == 0 {
			// Sequence overflow, wait for next millisecond
			now = waitNextMillis(g.lastTime)
		}
	} else {
		// New millisecond, reset sequence
		g.sequence = 0
	}

	g.lastTime = now

	// Compose the ID
	id := ((now - epoch) << timestampShift) |
		(g.workerID << workerIDShift) |
		g.sequence

	return id, nil
}

// MustGenerate generates a new ID and panics on error.
func (g *Generator) MustGenerate() int64 {
	id, err := g.Generate()
	if err != nil {
		panic(err)
	}
	return id
}

// Parse extracts components from a Snowflake ID.
func Parse(id int64) (timestamp time.Time, workerID int64, sequence int64) {
	ts := (id >> timestampShift) + epoch
	timestamp = time.UnixMilli(ts)
	workerID = (id >> workerIDShift) & maxWorkerID
	sequence = id & maxSequence
	return
}

// Timestamp extracts the timestamp from a Snowflake ID.
func Timestamp(id int64) time.Time {
	ts := (id >> timestampShift) + epoch
	return time.UnixMilli(ts)
}

// currentTimeMillis returns current time in milliseconds.
func currentTimeMillis() int64 {
	return time.Now().UnixMilli()
}

// waitNextMillis waits until the next millisecond.
func waitNextMillis(lastTime int64) int64 {
	now := currentTimeMillis()
	for now <= lastTime {
		time.Sleep(100 * time.Microsecond)
		now = currentTimeMillis()
	}
	return now
}

// =============================================================================
// Global Generator (for convenience)
// =============================================================================

var (
	globalGen  *Generator
	globalOnce sync.Once
	globalErr  error
)

// Init initializes the global generator with the given worker ID.
// This should be called once at application startup.
func Init(workerID int64) error {
	globalOnce.Do(func() {
		globalGen, globalErr = NewGenerator(workerID)
	})
	return globalErr
}

// ID generates a new Snowflake ID using the global generator.
// Init must be called before using this function.
func ID() int64 {
	if globalGen == nil {
		panic("snowflake: global generator not initialized, call Init() first")
	}
	return globalGen.MustGenerate()
}

// NextID is an alias for ID().
func NextID() int64 {
	return ID()
}
