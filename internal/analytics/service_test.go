package analytics

import (
	"testing"
	"time"
)

func TestParseRedisMemory(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int64
	}{
		{
			name:     "empty string",
			input:    "",
			expected: 0,
		},
		{
			name:     "valid memory info",
			input:    "# Memory\r\nused_memory:1234567\r\nused_memory_human:1.18M\r\n",
			expected: 1234567,
		},
		{
			name:     "memory info with unix line endings",
			input:    "# Memory\nused_memory:9876543\nused_memory_human:9.42M\n",
			expected: 9876543,
		},
		{
			name:     "memory info with spaces",
			input:    "used_memory: 5555555 \n",
			expected: 5555555,
		},
		{
			name:     "invalid number",
			input:    "used_memory:not_a_number\n",
			expected: 0,
		},
		{
			name:     "missing used_memory",
			input:    "used_memory_peak:1234567\n",
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseRedisMemory(tt.input)
			if result != tt.expected {
				t.Errorf("parseRedisMemory(%q) = %d, want %d", tt.input, result, tt.expected)
			}
		})
	}
}

func TestTimeAgo(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		expected string
	}{
		{
			name:     "just now",
			duration: 30 * time.Second,
			expected: "just now",
		},
		{
			name:     "1 minute",
			duration: 1*time.Minute + 30*time.Second,
			expected: "1 min ago",
		},
		{
			name:     "multiple minutes",
			duration: 5 * time.Minute,
			expected: "5 min ago",
		},
		{
			name:     "1 hour",
			duration: 1*time.Hour + 15*time.Minute,
			expected: "1 hour ago",
		},
		{
			name:     "multiple hours",
			duration: 3 * time.Hour,
			expected: "3 hours ago",
		},
		{
			name:     "1 day",
			duration: 25 * time.Hour,
			expected: "1 day ago",
		},
		{
			name:     "multiple days",
			duration: 72 * time.Hour,
			expected: "3 days ago",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testTime := time.Now().Add(-tt.duration)
			result := TimeAgo(testTime)
			if result != tt.expected {
				t.Errorf("TimeAgo() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestGetStorageLimit(t *testing.T) {
	tests := []struct {
		tier     string
		expected int64
	}{
		{"free", StorageLimitFree},
		{"pro", StorageLimitPro},
		{"enterprise", StorageLimitEnterprise},
		{"unknown", StorageLimitFree},
		{"", StorageLimitFree},
	}

	for _, tt := range tests {
		t.Run(tt.tier, func(t *testing.T) {
			result := getStorageLimit(tt.tier)
			if result != tt.expected {
				t.Errorf("getStorageLimit(%q) = %d, want %d", tt.tier, result, tt.expected)
			}
		})
	}
}

type mockPoolStats struct {
	acquired int32
	total    int32
}

func (m *mockPoolStats) AcquiredConns() int32 { return m.acquired }
func (m *mockPoolStats) TotalConns() int32    { return m.total }

func TestPoolStatsFunc(t *testing.T) {
	mock := &mockPoolStats{acquired: 5, total: 10}
	adapter := NewPoolStatsFunc(func() PoolStats { return mock })

	if adapter.AcquiredConns() != 5 {
		t.Errorf("AcquiredConns() = %d, want 5", adapter.AcquiredConns())
	}
	if adapter.TotalConns() != 10 {
		t.Errorf("TotalConns() = %d, want 10", adapter.TotalConns())
	}

	mock.acquired = 8
	if adapter.AcquiredConns() != 8 {
		t.Errorf("AcquiredConns() after update = %d, want 8", adapter.AcquiredConns())
	}
}
