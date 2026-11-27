// Copyright (C) 2026 otoneko.
// SPDX-License-Identifier: GPL-3.0-only

package tray

import "testing"

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
