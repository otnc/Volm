// Copyright (C) 2026 otoneko.
// SPDX-License-Identifier: GPL-3.0-only

package appid

import (
	"strings"
	"testing"
)

func TestTargetMatches(t *testing.T) {
	brave := NewTarget("Brave", map[string]bool{`C:\Program Files\BraveSoftware\Brave-Browser\Application\brave.exe`: true})
	claude := NewTarget("Claude_pzs8sxrjxfjjc!Claude", nil)
	spotify := NewTarget(`C:\Users\someone\AppData\Roaming\Spotify\Spotify.exe`, nil)

	tests := []struct {
		name       string
		target     Target
		exe, aumid string
		want       bool
	}{
		{"a browser's audio process shares the window's executable", brave, `c:\program files\bravesoftware\brave-browser\application\brave.exe`, "", true},
		{"another app", brave, `C:\Program Files\VideoLAN\VLC\vlc.exe`, "", false},
		{"packaged app by its AUMID", claude, `C:\Program Files\WindowsApps\Claude\claude.exe`, "Claude_pzs8sxrjxfjjc!Claude", true},
		{"packaged app with another AUMID", claude, "", "Other_123!App", false},
		{"Win32 app whose AUMID is its path", spotify, `C:\Users\someone\AppData\Roaming\Spotify\Spotify.exe`, "", true},
		{"unknown process", brave, "", "", false},
	}
	for _, tt := range tests {
		if got := tt.target.Matches(tt.exe, tt.aumid); got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestExpandKnownFolder(t *testing.T) {
	got := ExpandKnownFolder(`{6D809377-6AF0-444B-8957-A3773F02200E}\VideoLAN\VLC\vlc.exe`)
	if !strings.HasSuffix(got, `\VideoLAN\VLC\vlc.exe`) || strings.HasPrefix(got, "{") {
		t.Errorf("ProgramFilesX64 path = %q", got)
	}
	for _, unchanged := range []string{"Brave", "Claude_pzs8sxrjxfjjc!Claude", "{not-a-guid}\\x.exe"} {
		if got := ExpandKnownFolder(unchanged); got != unchanged {
			t.Errorf("ExpandKnownFolder(%q) = %q, want it unchanged", unchanged, got)
		}
	}
}
