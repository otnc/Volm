// Copyright (C) 2026 otoneko.
// SPDX-License-Identifier: GPL-3.0-only

// Package hotkey parses the key combinations of the settings file, such as "Ctrl+Shift+Q", and names the actions they run.
package hotkey

import (
	"errors"
	"fmt"
	"strings"
)

// Modifiers is a set of modifier keys, either side.
type Modifiers uint8

// The modifiers, in the order a combination names them.
const (
	Ctrl Modifiers = 1 << iota
	Alt
	Shift
	Win
)

var modifierNames = []struct {
	mod  Modifiers
	name string
}{{Ctrl, "Ctrl"}, {Alt, "Alt"}, {Shift, "Shift"}, {Win, "Win"}}

// Combo is a key pressed with modifiers: a virtual-key code.
type Combo struct {
	Mods Modifiers
	Key  uint16
}

// keyNames maps the names the settings file uses to virtual-key codes. Letters, digits and F1 to F24 are handled apart.
var keyNames = map[string]uint16{
	"Up": 0x26, "Down": 0x28, "Left": 0x25, "Right": 0x27,
	"Space": 0x20, "Enter": 0x0D, "Tab": 0x09, "Escape": 0x1B, "Backspace": 0x08,
	"Insert": 0x2D, "Delete": 0x2E, "Home": 0x24, "End": 0x23, "PageUp": 0x21, "PageDown": 0x22,
	"Pause": 0x13, "PrintScreen": 0x2C, "ScrollLock": 0x91,
	"Num0": 0x60, "Num1": 0x61, "Num2": 0x62, "Num3": 0x63, "Num4": 0x64,
	"Num5": 0x65, "Num6": 0x66, "Num7": 0x67, "Num8": 0x68, "Num9": 0x69,
	"NumMultiply": 0x6A, "NumAdd": 0x6B, "NumSubtract": 0x6D, "NumDecimal": 0x6E, "NumDivide": 0x6F,
	"VolumeMute": 0xAD, "VolumeDown": 0xAE, "VolumeUp": 0xAF,
	"MediaNext": 0xB0, "MediaPrevious": 0xB1, "MediaStop": 0xB2, "MediaPlayPause": 0xB3,
	";": 0xBA, "=": 0xBB, ",": 0xBC, "-": 0xBD, ".": 0xBE, "/": 0xBF, "`": 0xC0,
	"[": 0xDB, `\`: 0xDC, "]": 0xDD, "'": 0xDE,
}

// ErrNoKey reports a combination of modifiers only, or of an unknown key.
var ErrNoKey = errors.New("hotkey: no key in the combination")

// Parse reads a combination such as "Win+Up" or "Ctrl+Shift+Q". Names are matched without regard to case, and modifiers may come in any order.
//
//	c, err := hotkey.Parse("Ctrl+Shift+Q")
//	// c == Combo{Mods: Ctrl | Shift, Key: 'Q'}
func Parse(s string) (Combo, error) {
	var c Combo
	parts := strings.Split(s, "+")
	// "+" itself, as in "Ctrl++", leaves an empty part before it.
	if strings.HasSuffix(s, "++") {
		parts = append(parts[:len(parts)-2], "+")
	}
	for i, p := range parts {
		p = strings.TrimSpace(p)
		if mod, ok := parseModifier(p); ok && i < len(parts)-1 {
			c.Mods |= mod
			continue
		}
		if i != len(parts)-1 {
			return Combo{}, fmt.Errorf("hotkey: %q is not a modifier in %q", p, s)
		}
		key, ok := parseKey(p)
		if !ok {
			return Combo{}, fmt.Errorf("%w: %q", ErrNoKey, s)
		}
		c.Key = key
	}
	return c, nil
}

func parseModifier(s string) (Modifiers, bool) {
	for _, m := range modifierNames {
		if strings.EqualFold(s, m.name) {
			return m.mod, true
		}
	}
	return 0, false
}

func parseKey(s string) (uint16, bool) {
	if len(s) == 1 {
		c := strings.ToUpper(s)[0]
		if (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			return uint16(c), true
		}
	}
	if len(s) >= 2 && (s[0] == 'F' || s[0] == 'f') {
		var n int
		if _, err := fmt.Sscanf(s[1:], "%d", &n); err == nil && n >= 1 && n <= 24 && fmt.Sprint(n) == s[1:] {
			return uint16(0x70 + n - 1), true
		}
	}
	for name, vk := range keyNames {
		if strings.EqualFold(s, name) {
			return vk, true
		}
	}
	return 0, false
}

// String writes the combination as Parse reads it, modifiers in a fixed order: "Ctrl+Shift+Q".
func (c Combo) String() string {
	var b strings.Builder
	for _, m := range modifierNames {
		if c.Mods&m.mod != 0 {
			b.WriteString(m.name + "+")
		}
	}
	b.WriteString(KeyName(c.Key))
	return b.String()
}

// KeyName names a virtual-key code as the settings file does, or as "VK" and its hexadecimal code when the key has no name.
func KeyName(vk uint16) string {
	switch {
	case vk >= 'A' && vk <= 'Z', vk >= '0' && vk <= '9':
		return string(rune(vk))
	case vk >= 0x70 && vk <= 0x87:
		return fmt.Sprintf("F%d", vk-0x70+1)
	}
	for name, code := range keyNames {
		if code == vk {
			return name
		}
	}
	return fmt.Sprintf("VK%02X", vk)
}

// IsModifierKey reports whether vk is one of the modifier keys themselves, which never complete a combination.
func IsModifierKey(vk uint16) bool {
	switch vk {
	case 0x10, 0x11, 0x12, 0x5B, 0x5C, 0xA0, 0xA1, 0xA2, 0xA3, 0xA4, 0xA5:
		return true
	}
	return false
}
