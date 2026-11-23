package config

import "testing"

// BITBUCKET_MAX_WORKERS=0 or a negative value must not disable worker pools:
// GlobalMaxWorkers is used to size semaphores and worker loops, and a zero
// value deadlocks them. Invalid or non-positive values fall back to 5.
func TestLoadConfig_MaxWorkersFloor(t *testing.T) {
	tests := []struct {
		name string
		env  string
		want int
	}{
		{"unset", "", 5},
		{"zero", "0", 5},
		{"negative", "-3", 5},
		{"not a number", "abc", 5},
		{"valid", "7", 7},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("BITBUCKET_MAX_WORKERS", tt.env)
			prev := GlobalMaxWorkers
			t.Cleanup(func() { GlobalMaxWorkers = prev })

			if _, err := LoadConfig(); err != nil {
				t.Fatalf("LoadConfig: %v", err)
			}
			if GlobalMaxWorkers != tt.want {
				t.Fatalf("GlobalMaxWorkers = %d, want %d", GlobalMaxWorkers, tt.want)
			}
		})
	}
}
