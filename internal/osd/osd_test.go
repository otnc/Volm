// Copyright (C) 2026 otoneko.
// SPDX-License-Identifier: GPL-3.0-only

package osd

import (
	"testing"

	"github.com/otnc/Volm/internal/win32"
)

func TestPlacePanel(t *testing.T) {
	// A secondary monitor left of the main one, with a taskbar 48px tall.
	work := win32.Rect{Left: -1600, Top: 312, Right: 0, Bottom: 1164}
	const w, h, margin = 300, 76, 16
	tests := []struct {
		position string
		x, y     int32
	}{
		{"bottomRight", -316, 1072},
		{"bottomLeft", -1584, 1072},
		{"topRight", -316, 328},
		{"topLeft", -1584, 328},
		{"bottomCenter", -950, 1072},
		{"unknown", -316, 1072},
	}
	for _, tt := range tests {
		x, y := placePanel(work, w, h, margin, tt.position)
		if x != tt.x || y != tt.y {
			t.Errorf("%s: got (%d, %d), want (%d, %d)", tt.position, x, y, tt.x, tt.y)
		}
	}
}

func TestSpeakerIcon(t *testing.T) {
	tests := []struct {
		percent int
		muted   bool
		want    Icon
	}{
		{50, true, IconMute},
		{0, false, IconSpeakerZero},
		{1, false, IconSpeakerLow},
		{33, false, IconSpeakerLow},
		{34, false, IconSpeakerMid},
		{66, false, IconSpeakerMid},
		{67, false, IconSpeakerHigh},
		{100, false, IconSpeakerHigh},
	}
	for _, tt := range tests {
		if got := SpeakerIcon(tt.percent, tt.muted); got != tt.want {
			t.Errorf("SpeakerIcon(%d, %v) = %#x, want %#x", tt.percent, tt.muted, got, tt.want)
		}
	}
}
