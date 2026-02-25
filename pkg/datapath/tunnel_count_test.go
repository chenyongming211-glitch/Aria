package datapath

import "testing"

func TestDetermineTunnelCount(t *testing.T) {
	tests := []struct {
		name     string
		count    int
		expected int
	}{
		{
			name:     "default (0) returns 4",
			count:    0,
			expected: 4,
		},
		{
			name:     "explicit 1 tunnel",
			count:    1,
			expected: 1,
		},
		{
			name:     "explicit 2 tunnels",
			count:    2,
			expected: 2,
		},
		{
			name:     "explicit 4 tunnels",
			count:    4,
			expected: 4,
		},
		{
			name:     "explicit 8 tunnels",
			count:    8,
			expected: 8,
		},
		{
			name:     "below minimum (0) returns default 4",
			count:    0,
			expected: 4,
		},
		{
			name:     "above maximum (10) capped at 8",
			count:    10,
			expected: 8,
		},
		{
			name:     "negative value returns minimum 1",
			count:    -1,
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetermineTunnelCount(tt.count)
			if result != tt.expected {
				t.Errorf("DetermineTunnelCount(%d) = %d, want %d",
					tt.count, result, tt.expected)
			}
		})
	}
}

func TestDetectCPUCount(t *testing.T) {
	count := DetectCPUCount()
	if count <= 0 {
		t.Errorf("DetectCPUCount() = %d, want > 0", count)
	}
	t.Logf("Detected CPU count: %d", count)
}

func TestDefaultTunnelCount(t *testing.T) {
	if DefaultTunnelCount != 4 {
		t.Errorf("DefaultTunnelCount = %d, want 4", DefaultTunnelCount)
	}
}

func TestTunnelCountConstants(t *testing.T) {
	if MinTunnelCount < 1 {
		t.Errorf("MinTunnelCount = %d, want >= 1", MinTunnelCount)
	}
	if MaxTunnelCount < MinTunnelCount {
		t.Errorf("MaxTunnelCount (%d) < MinTunnelCount (%d)", MaxTunnelCount, MinTunnelCount)
	}
	if DefaultTunnelCount < MinTunnelCount || DefaultTunnelCount > MaxTunnelCount {
		t.Errorf("DefaultTunnelCount (%d) out of range [%d, %d]",
			DefaultTunnelCount, MinTunnelCount, MaxTunnelCount)
	}
}
