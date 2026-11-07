package main

import "testing"

func TestExplorerWorkerCount(t *testing.T) {
	tests := []struct {
		name             string
		requested, cores uint
		wantCount        uint
		wantLowered      bool
	}{
		{"default", 0, 16, 4, false},
		{"default on a small machine", 0, 2, 2, false},
		{"as requested", 6, 16, 6, false},
		{"one per core", 16, 16, 16, false},
		{"more than the cores", 64, 16, 16, true},
		{"no cores reported", 0, 0, 1, false},
	}
	for _, tc := range tests {
		count, lowered := explorerWorkerCount(tc.requested, tc.cores)
		if count != tc.wantCount || lowered != tc.wantLowered {
			t.Errorf("%s: explorerWorkerCount(%d, %d) = %d, %v; want %d, %v", tc.name, tc.requested, tc.cores, count, lowered, tc.wantCount, tc.wantLowered)
		}
	}
}
