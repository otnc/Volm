// Copyright (C) 2026 otoneko.
// SPDX-License-Identifier: GPL-3.0-only

package hotkey

import (
	"errors"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		in   string
		want Combo
	}{
		{"Win+Up", Combo{Win, 0x26}},
		{"Ctrl+Shift+Q", Combo{Ctrl | Shift, 'Q'}},
		{"shift+ctrl+q", Combo{Ctrl | Shift, 'Q'}},
		{"Alt+F4", Combo{Alt, 0x73}},
		{"Ctrl+F12", Combo{Ctrl, 0x7B}},
		{"Ctrl+Alt+7", Combo{Ctrl | Alt, '7'}},
		{"Win+PageDown", Combo{Win, 0x22}},
		{"Ctrl+/", Combo{Ctrl, 0xBF}},
		{"MediaPlayPause", Combo{0, 0xB3}},
	}
	for _, tt := range tests {
		got, err := Parse(tt.in)
		if err != nil || got != tt.want {
			t.Errorf("Parse(%q) = %+v, %v; want %+v", tt.in, got, err, tt.want)
		}
	}
}

func TestParseRejects(t *testing.T) {
	for _, in := range []string{"", "Ctrl", "Ctrl+Win", "Ctrl+Foo", "Hyper+A", "Ctrl+F25", "Ctrl+F01"} {
		if c, err := Parse(in); err == nil {
			t.Errorf("Parse(%q) = %+v, want an error", in, c)
		}
	}
	if _, err := Parse("Ctrl+Shift"); !errors.Is(err, ErrNoKey) {
		t.Errorf("Parse of modifiers only: err = %v, want ErrNoKey", err)
	}
}

func TestStringRoundTrips(t *testing.T) {
	for _, in := range []string{"Win+Up", "Ctrl+Shift+Q", "Ctrl+Alt+Shift+Win+F24", "Ctrl+/", "Alt+Num5", "Ctrl+\\"} {
		c, err := Parse(in)
		if err != nil {
			t.Fatalf("Parse(%q): %v", in, err)
		}
		if got := c.String(); got != in {
			t.Errorf("Parse(%q).String() = %q", in, got)
		}
	}
	if got := (Combo{Mods: Shift | Ctrl, Key: 'A'}).String(); got != "Ctrl+Shift+A" {
		t.Errorf("modifier order: got %q", got)
	}
}

func TestBind(t *testing.T) {
	b := Bind(map[string][]string{
		"volumeUp":   {"Win+Up", "Ctrl+Alt+Up", "Ctrl+Alt+PageUp"},
		"volumeDown": {"Win+Down", "Win+Up"},
		"playPause":  {"Ctrl+Nope"},
	})
	want := Bindings{
		{Win, 0x26}:        VolumeUp,
		{Ctrl | Alt, 0x26}: VolumeUp,
		{Win, 0x28}:        VolumeDown,
	}
	if len(b) != len(want) {
		t.Fatalf("got %d bindings %v, want %v", len(b), b, want)
	}
	for c, a := range want {
		if b[c] != a {
			t.Errorf("%s = %q, want %q", c, b[c], a)
		}
	}
}

func TestConflict(t *testing.T) {
	keys := map[string][]string{"volumeUp": {"Win+Up"}, "nextTrack": {"Ctrl+Shift+E"}}
	c, _ := Parse("Ctrl+Shift+E")
	if a, ok := Conflict(keys, c, VolumeUp); !ok || a != NextTrack {
		t.Errorf("Conflict = %q, %v; want nextTrack", a, ok)
	}
	if _, ok := Conflict(keys, c, NextTrack); ok {
		t.Error("an action conflicted with itself")
	}
}
