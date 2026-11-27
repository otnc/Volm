// Copyright (C) 2026 otoneko.
// SPDX-License-Identifier: GPL-3.0-only

package tray

import (
	"testing"
)

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

func TestDeviceMenu(t *testing.T) {
	var selected string
	item := deviceMenu("入力デバイス", "録音デバイスがありません", []Device{
		{ID: "mic-a", Name: "マイク (USB)"},
		{ID: "mic-b", Name: "マイク (Realtek(R) Audio)", Default: true},
	}, func(id string) { selected = id })
	if item.Label != "入力デバイス" || len(item.Submenu) != 2 {
		t.Fatalf("got %q with %d items", item.Label, len(item.Submenu))
	}
	if item.Submenu[0].Checked || !item.Submenu[1].Checked {
		t.Error("the check is not on the default device")
	}
	item.Submenu[0].Click(item.Submenu[0], nil)
	if selected != "mic-a" {
		t.Errorf("clicking the first device selected %q", selected)
	}
}

func TestDeviceMenuWithoutDevices(t *testing.T) {
	item := deviceMenu("入力デバイス", "録音デバイスがありません", nil, func(string) {})
	if len(item.Submenu) != 1 || item.Submenu[0].Label != "録音デバイスがありません" || !item.Submenu[0].Disabled {
		t.Fatalf("got %+v, want one disabled notice", item.Submenu)
	}
}
