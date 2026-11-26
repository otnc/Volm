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
		{"Win+Up", Combo{Mods: Win, Key: 0x26}},
		{"Ctrl+Shift+Q", Combo{Mods: Ctrl | Shift, Key: 'Q'}},
		{"shift+ctrl+q", Combo{Mods: Ctrl | Shift, Key: 'Q'}},
		{"Alt+F4", Combo{Mods: Alt, Key: 0x73}},
		{"Ctrl+F12", Combo{Mods: Ctrl, Key: 0x7B}},
		{"Ctrl+Alt+7", Combo{Mods: Ctrl | Alt, Key: '7'}},
		{"Win+PageDown", Combo{Mods: Win, Key: 0x22}},
		{"Ctrl+/", Combo{Mods: Ctrl, Key: 0xBF}},
		{"MediaPlayPause", Combo{Key: 0xB3}},
		{"LaunchApp1", Combo{Key: 0xB6}},
		{"Ctrl+BrowserBack", Combo{Mods: Ctrl, Key: 0xA6}},
		{"F13", Combo{Key: 0x7C}},
		{"vke8", Combo{Key: 0xE8}},
		{"Alt+SC07E", Combo{Mods: Alt, Scan: 0x7E}},
		{"SCE05F", Combo{Scan: 0xE05F}},
	}
	for _, tt := range tests {
		got, err := Parse(tt.in)
		if err != nil || got != tt.want {
			t.Errorf("Parse(%q) = %+v, %v; want %+v", tt.in, got, err, tt.want)
		}
	}
}

func TestParseRejects(t *testing.T) {
	for _, in := range []string{"", "Ctrl", "Ctrl+Win", "Ctrl+Foo", "Hyper+A", "Ctrl+F25", "Ctrl+F01", "VK00", "VK11", "VKFF", "VKZZ", "SC", "SC0", "SC12345"} {
		if c, err := Parse(in); err == nil {
			t.Errorf("Parse(%q) = %+v, want an error", in, c)
		}
	}
	if _, err := Parse("Ctrl+Shift"); !errors.Is(err, ErrNoKey) {
		t.Errorf("Parse of modifiers only: err = %v, want ErrNoKey", err)
	}
}

func TestStringRoundTrips(t *testing.T) {
	for _, in := range []string{"Win+Up", "Ctrl+Shift+Q", "Ctrl+Alt+Shift+Win+F24", "Ctrl+/", "Alt+Num5", "Ctrl+\\", "LaunchApp2", "VKE8", "Ctrl+SC07E", "SCE05F"} {
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
		{Mods: Win, Key: 0x26}:        VolumeUp,
		{Mods: Ctrl | Alt, Key: 0x26}: VolumeUp,
		{Mods: Win, Key: 0x28}:        VolumeDown,
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

func TestBindSkipsTypingKeysAlone(t *testing.T) {
	b := Bind(map[string][]string{"volumeUp": {"A", "F13"}, "playPause": {"MediaPlayPause"}})
	if _, ok := b[Combo{Key: 'A'}]; ok {
		t.Error("a bare letter was bound")
	}
	if b[Combo{Key: 0x7C}] != VolumeUp || b[Combo{Key: 0xB3}] != PlayPause {
		t.Errorf("keys made for shortcuts were not bound alone: %v", b)
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

func TestNeedsModifier(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"A", true},
		{"7", true},
		{"F5", true},
		{"Enter", true},
		{"Up", true},
		{"Num5", true},
		{"/", true},
		{"Ctrl+A", false},
		{"Win+Up", false},
		{"F13", false},
		{"F24", false},
		{"MediaPlayPause", false},
		{"BrowserHome", false},
		{"LaunchApp1", false},
		{"VKE8", false},
		{"SC07E", false},
	}
	for _, tt := range tests {
		c, err := Parse(tt.in)
		if err != nil {
			t.Fatalf("Parse(%q): %v", tt.in, err)
		}
		if got := c.NeedsModifier(); got != tt.want {
			t.Errorf("%s: NeedsModifier = %v, want %v", tt.in, got, tt.want)
		}
	}
}
