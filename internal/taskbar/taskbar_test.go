// Copyright (C) 2026 otoneko.
// SPDX-License-Identifier: GPL-3.0-only

package taskbar

import "testing"

func TestTitleAppName(t *testing.T) {
	tests := []struct{ in, want string }{
		{"YouTube - Brave", "Brave"},
		{"main.go - Volm - Visual Studio Code", "Visual Studio Code"},
		{"メモ帳", "メモ帳"},
		{"Spotify Premium", "Spotify Premium"},
	}
	for _, tt := range tests {
		if got := TitleAppName(tt.in); got != tt.want {
			t.Errorf("TitleAppName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestAppName(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Brave - 1 個の実行中ウィンドウ 固定済み", "Brave"},
		{"Discord 固定済み", "Discord"},
		{"Visual Studio Code - 1 個の実行中ウィンドウ 固定済み", "Visual Studio Code"},
		{"Spotify", "Spotify"},
		{"Firefox - 2 running windows Pinned", "Firefox"},
	}
	for _, tt := range tests {
		if got := AppName(tt.in); got != tt.want {
			t.Errorf("AppName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
