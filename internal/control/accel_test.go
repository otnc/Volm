// Copyright (C) 2026 otoneko.
// SPDX-License-Identifier: GPL-3.0-only

package control

import (
	"testing"
	"time"
)

func TestAccelerator(t *testing.T) {
	type notch struct {
		after   time.Duration // since the previous notch
		notches int
		want    int
	}
	tests := []struct {
		name  string
		steps []notch
	}{
		{"first notch is never sped up", []notch{{0, 1, 1}}},
		{"slow turning keeps the step", []notch{{0, 1, 1}, {200 * time.Millisecond, 1, 1}, {70 * time.Millisecond, 1, 1}}},
		{"quick turning doubles it", []notch{{0, 1, 1}, {69 * time.Millisecond, 1, 2}, {35 * time.Millisecond, 1, 2}}},
		{"fast turning quadruples it", []notch{{0, 1, 1}, {34 * time.Millisecond, 1, 4}, {10 * time.Millisecond, 1, 4}}},
		{"downward works the same", []notch{{0, -1, 1}, {20 * time.Millisecond, -1, 4}}},
		{"a reversal starts slow again", []notch{{0, 1, 1}, {20 * time.Millisecond, 1, 4}, {20 * time.Millisecond, -1, 1}, {20 * time.Millisecond, -1, 4}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var a Accelerator
			now := time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)
			for i, s := range tt.steps {
				now = now.Add(s.after)
				if got := a.Factor(now, s.notches); got != s.want {
					t.Fatalf("notch %d (%v after the previous): factor %d, want %d", i, s.after, got, s.want)
				}
			}
		})
	}
}
