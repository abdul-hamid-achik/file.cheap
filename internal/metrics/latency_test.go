package metrics

import (
	"testing"
)

func TestGetLatencyP95(t *testing.T) {
	latencyMu.Lock()
	latencyWindow = nil
	latencyMu.Unlock()

	if p95 := GetLatencyP95(); p95 != 0 {
		t.Errorf("GetLatencyP95() with empty window = %d, want 0", p95)
	}

	for i := int64(1); i <= 100; i++ {
		recordLatency(i)
	}

	p95 := GetLatencyP95()
	if p95 < 95 || p95 > 96 {
		t.Errorf("GetLatencyP95() = %d, want ~95", p95)
	}

	latencyMu.Lock()
	latencyWindow = nil
	latencyMu.Unlock()
}

func TestRecordLatency(t *testing.T) {
	latencyMu.Lock()
	latencyWindow = nil
	latencyMu.Unlock()

	recordLatency(100)
	recordLatency(200)
	recordLatency(300)

	latencyMu.Lock()
	count := len(latencyWindow)
	latencyMu.Unlock()

	if count != 3 {
		t.Errorf("latencyWindow has %d items, want 3", count)
	}

	latencyMu.Lock()
	latencyWindow = nil
	latencyMu.Unlock()
}

func TestRecordLatencyMaxWindow(t *testing.T) {
	latencyMu.Lock()
	latencyWindow = nil
	latencyMu.Unlock()

	for i := 0; i < maxLatencyRecords+100; i++ {
		recordLatency(int64(i))
	}

	latencyMu.Lock()
	count := len(latencyWindow)
	latencyMu.Unlock()

	if count != maxLatencyRecords {
		t.Errorf("latencyWindow has %d items, want %d (maxLatencyRecords)", count, maxLatencyRecords)
	}

	latencyMu.Lock()
	first := latencyWindow[0]
	latencyMu.Unlock()

	if first != 100 {
		t.Errorf("first item in window = %d, want 100 (oldest items should be evicted)", first)
	}

	latencyMu.Lock()
	latencyWindow = nil
	latencyMu.Unlock()
}

func TestGetLatencyP95SingleValue(t *testing.T) {
	latencyMu.Lock()
	latencyWindow = nil
	latencyMu.Unlock()

	recordLatency(50)

	p95 := GetLatencyP95()
	if p95 != 50 {
		t.Errorf("GetLatencyP95() with single value = %d, want 50", p95)
	}

	latencyMu.Lock()
	latencyWindow = nil
	latencyMu.Unlock()
}
