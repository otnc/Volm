// Copyright (C) 2026 otoneko.
// SPDX-License-Identifier: GPL-3.0-only

package control

import "testing"

func TestNotchCounter(t *testing.T) {
	tests := []struct {
		name   string
		deltas []int
		want   []int
	}{
		{"standard wheel up", []int{120, 120}, []int{1, 1}},
		{"standard wheel down", []int{-120}, []int{-1}},
		{"fast wheel sends two notches at once", []int{240}, []int{2}},
		{"high resolution deltas add up to one notch", []int{40, 40, 40}, []int{0, 0, 1}},
		{"leftover carries into the next notch", []int{100, 100, 100}, []int{0, 1, 1}},
		{"reversal drops the partial notch", []int{100, -120}, []int{0, -1}},
		{"reversal then small delta stays below a notch", []int{100, -20}, []int{0, 0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var c NotchCounter
			for i, d := range tt.deltas {
				if got := c.Add(d); got != tt.want[i] {
					t.Fatalf("Add(%d) at step %d = %d, want %d", d, i, got, tt.want[i])
				}
			}
		})
	}
}

func TestNotchCounterResetDropsPartial(t *testing.T) {
	var c NotchCounter
	c.Add(100)
	c.Reset()
	if got := c.Add(40); got != 0 {
		t.Fatalf("Add after Reset = %d, want 0", got)
	}
}

func TestStepPercent(t *testing.T) {
	tests := []struct {
		current, notches, step, want int
	}{
		{40, 1, 2, 42},
		{40, -3, 2, 34},
		{99, 1, 2, 100},
		{1, -1, 2, 0},
		{0, -5, 5, 0},
		{100, 2, 5, 100},
	}
	for _, tt := range tests {
		if got := StepPercent(tt.current, tt.notches, tt.step); got != tt.want {
			t.Errorf("StepPercent(%d, %d, %d) = %d, want %d", tt.current, tt.notches, tt.step, got, tt.want)
		}
	}
}
