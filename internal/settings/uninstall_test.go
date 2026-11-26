// Copyright (C) 2026 otoneko.
// SPDX-License-Identifier: GPL-3.0-only

package settings

import "testing"

func TestUninstallerFor(t *testing.T) {
	const command = `"C:\Users\a\AppData\Local\Programs\Volm\unins000.exe"`
	const want = `C:\Users\a\AppData\Local\Programs\Volm\unins000.exe`
	tests := []struct {
		name, exeDir, location, want string
	}{
		{"installed copy", `C:\Users\a\AppData\Local\Programs\Volm`, `C:\Users\a\AppData\Local\Programs\Volm\`, want},
		{"case differs", `c:\users\a\appdata\local\programs\volm`, `C:\Users\a\AppData\Local\Programs\Volm\`, want},
		{"another copy", `D:\Codes\Volm\build\windows-amd64`, `C:\Users\a\AppData\Local\Programs\Volm\`, ""},
		{"no location", `C:\Users\a\AppData\Local\Programs\Volm`, "", ""},
	}
	for _, tt := range tests {
		if got := uninstallerFor(tt.exeDir, tt.location, command); got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
}
