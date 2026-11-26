// Copyright (C) 2026 otoneko.
// SPDX-License-Identifier: GPL-3.0-only

package tray

import (
	"bytes"
	"image/png"
	"testing"
)

func TestLevelOf(t *testing.T) {
	tests := []struct {
		percent int
		muted   bool
		want    Level
	}{
		{80, true, LevelMuted},
		{0, false, LevelMuted},
		{1, false, LevelLow},
		{33, false, LevelLow},
		{34, false, LevelMid},
		{66, false, LevelMid},
		{67, false, LevelHigh},
		{100, false, LevelHigh},
	}
	for _, tt := range tests {
		if got := LevelOf(tt.percent, tt.muted); got != tt.want {
			t.Errorf("LevelOf(%d, %v) = %d, want %d", tt.percent, tt.muted, got, tt.want)
		}
	}
}

func TestTooltip(t *testing.T) {
	tests := []struct {
		name    string
		percent int
		muted   bool
		device  string
		want    string
	}{
		{"volume", 40, false, "スピーカー (Realtek(R) Audio)", "音量 40% / スピーカー (Realtek(R) Audio)"},
		{"muted", 40, true, "スピーカー (Realtek(R) Audio)", "ミュート中 / スピーカー (Realtek(R) Audio)"},
		{"no device", 0, false, "", "再生デバイスがありません"},
	}
	for _, tt := range tests {
		if got := Tooltip(tt.percent, tt.muted, tt.device); got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
}

// TestLevelIconsDiffer guards against two levels embedding the same picture, which would leave the tray icon unchanged as the volume moves.
func TestLevelIconsDiffer(t *testing.T) {
	seen := map[string]Level{}
	for l := LevelMuted; l <= LevelHigh; l++ {
		data := levelIcon(l)
		img, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("level %d: %v", l, err)
		}
		if b := img.Bounds(); b.Dx() != 32 || b.Dy() != 32 {
			t.Fatalf("level %d: size %v, want 32x32", l, b.Size())
		}
		if other, ok := seen[string(data)]; ok {
			t.Fatalf("levels %d and %d draw the same icon", other, l)
		}
		seen[string(data)] = l
	}
}
