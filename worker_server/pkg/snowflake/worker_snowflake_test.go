package snowflake

import (
	"sync"
	"testing"
	"time"
)

func TestNewGenerator(t *testing.T) {
	tests := []struct {
		name     string
		workerID int64
		wantErr  bool
	}{
		{"valid worker 0", 0, false},
		{"valid worker 1", 1, false},
		{"valid worker max", 1023, false},
		{"invalid worker -1", -1, true},
		{"invalid worker 1024", 1024, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewGenerator(tt.workerID)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewGenerator(%d) error = %v, wantErr %v", tt.workerID, err, tt.wantErr)
			}
		})
	}
}

func TestGenerate_Unique(t *testing.T) {
	gen, err := NewGenerator(1)
	if err != nil {
		t.Fatal(err)
	}

	ids := make(map[int64]bool)
	count := 10000

	for i := 0; i < count; i++ {
		id, err := gen.Generate()
		if err != nil {
			t.Fatalf("Generate() error = %v", err)
		}

		if ids[id] {
			t.Fatalf("duplicate ID generated: %d", id)
		}
		ids[id] = true
	}
}

func TestGenerate_Concurrent(t *testing.T) {
	gen, err := NewGenerator(1)
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	ids := sync.Map{}
	goroutines := 10
	idsPerGoroutine := 1000

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < idsPerGoroutine; j++ {
				id, err := gen.Generate()
				if err != nil {
					t.Errorf("Generate() error = %v", err)
					return
				}

				if _, loaded := ids.LoadOrStore(id, true); loaded {
					t.Errorf("duplicate ID: %d", id)
					return
				}
			}
		}()
	}

	wg.Wait()

	// Verify count
	count := 0
	ids.Range(func(_, _ interface{}) bool {
		count++
		return true
	})

	expected := goroutines * idsPerGoroutine
	if count != expected {
		t.Errorf("expected %d unique IDs, got %d", expected, count)
	}
}

func TestGenerate_Sorted(t *testing.T) {
	gen, err := NewGenerator(1)
	if err != nil {
		t.Fatal(err)
	}

	var ids []int64
	for i := 0; i < 100; i++ {
		id, _ := gen.Generate()
		ids = append(ids, id)
		time.Sleep(time.Microsecond * 10)
	}

	// Verify IDs are in ascending order
	for i := 1; i < len(ids); i++ {
		if ids[i] <= ids[i-1] {
			t.Errorf("IDs not sorted: ids[%d]=%d <= ids[%d]=%d", i, ids[i], i-1, ids[i-1])
		}
	}
}

func TestParse(t *testing.T) {
	gen, err := NewGenerator(42)
	if err != nil {
		t.Fatal(err)
	}

	beforeGen := time.Now()
	id, _ := gen.Generate()
	afterGen := time.Now()

	ts, workerID, seq := Parse(id)

	// Check worker ID
	if workerID != 42 {
		t.Errorf("workerID = %d, want 42", workerID)
	}

	// Check sequence
	if seq != 0 {
		t.Errorf("sequence = %d, want 0 (first ID)", seq)
	}

	// Check timestamp (within reasonable range)
	if ts.Before(beforeGen.Add(-time.Second)) || ts.After(afterGen.Add(time.Second)) {
		t.Errorf("timestamp %v not in expected range [%v, %v]", ts, beforeGen, afterGen)
	}
}

func TestTimestamp(t *testing.T) {
	gen, err := NewGenerator(1)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	id, _ := gen.Generate()

	ts := Timestamp(id)

	// Should be within 1 second of now
	diff := ts.Sub(now)
	if diff < -time.Second || diff > time.Second {
		t.Errorf("Timestamp diff = %v, want within ±1s", diff)
	}
}

func BenchmarkGenerate(b *testing.B) {
	gen, _ := NewGenerator(1)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		gen.Generate()
	}
}

func BenchmarkGenerate_Parallel(b *testing.B) {
	gen, _ := NewGenerator(1)
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			gen.Generate()
		}
	})
}
