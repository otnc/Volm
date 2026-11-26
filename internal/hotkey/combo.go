// Copyright (C) 2026 otoneko.
// SPDX-License-Identifier: GPL-3.0-only

// Package hotkey parses the key combinations of the settings file, such as "Ctrl+Shift+Q", and names the actions they run.
package hotkey

import (
	"errors"
	"fmt"
	"strconv"
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

// Combo is a key pressed with modifiers. Key is the virtual-key code. Some keys of extra keyboards and macro pads have no virtual-key code of their own; for those Key is 0 and Scan holds the scan code, with 0xE000 added for an extended key.
type Combo struct {
	Mods Modifiers
	Key  uint16
	Scan uint16
}

// keyNames maps the names the settings file uses to virtual-key codes. Letters, digits and F1 to F24 are handled apart, and a key without a name is written as "VK" and its code (see KeyName).
var keyNames = map[string]uint16{
	"Up": 0x26, "Down": 0x28, "Left": 0x25, "Right": 0x27,
	"Space": 0x20, "Enter": 0x0D, "Tab": 0x09, "Escape": 0x1B, "Backspace": 0x08,
	"Insert": 0x2D, "Delete": 0x2E, "Home": 0x24, "End": 0x23, "PageUp": 0x21, "PageDown": 0x22,
	"Pause": 0x13, "PrintScreen": 0x2C, "ScrollLock": 0x91, "NumLock": 0x90, "Apps": 0x5D, "Sleep": 0x5F,
	"Clear": 0x0C, "Help": 0x2F,
	"Num0": 0x60, "Num1": 0x61, "Num2": 0x62, "Num3": 0x63, "Num4": 0x64,
	"Num5": 0x65, "Num6": 0x66, "Num7": 0x67, "Num8": 0x68, "Num9": 0x69,
	"NumMultiply": 0x6A, "NumAdd": 0x6B, "NumSubtract": 0x6D, "NumDecimal": 0x6E, "NumDivide": 0x6F,
	"BrowserBack": 0xA6, "BrowserForward": 0xA7, "BrowserRefresh": 0xA8, "BrowserStop": 0xA9,
	"BrowserSearch": 0xAA, "BrowserFavorites": 0xAB, "BrowserHome": 0xAC,
	"VolumeMute": 0xAD, "VolumeDown": 0xAE, "VolumeUp": 0xAF,
	"MediaNext": 0xB0, "MediaPrevious": 0xB1, "MediaStop": 0xB2, "MediaPlayPause": 0xB3,
	"LaunchMail": 0xB4, "LaunchMedia": 0xB5, "LaunchApp1": 0xB6, "LaunchApp2": 0xB7,
	";": 0xBA, "=": 0xBB, ",": 0xBC, "-": 0xBD, ".": 0xBE, "/": 0xBF, "`": 0xC0,
	"[": 0xDB, `\`: 0xDC, "]": 0xDD, "'": 0xDE,
}

// ErrNoKey reports a combination of modifiers only, or of an unknown key.
var ErrNoKey = errors.New("hotkey: no key in the combination")

// Parse reads a combination such as "Win+Up", "Ctrl+Shift+Q", "LaunchApp1", "VKE8" (a virtual-key code in hexadecimal, other than 0xFF, which Windows sends for a key it has no code for) or "SC07E" (a scan code). Names are matched without regard to case, and modifiers may come in any order.
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
		key, scan, ok := parseKey(p)
		if !ok {
			return Combo{}, fmt.Errorf("%w: %q", ErrNoKey, s)
		}
		c.Key, c.Scan = key, scan
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

func parseKey(s string) (vk, scan uint16, ok bool) {
	if len(s) == 1 {
		c := strings.ToUpper(s)[0]
		if (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			return uint16(c), 0, true
		}
	}
	if len(s) >= 2 && (s[0] == 'F' || s[0] == 'f') {
		if n, err := strconv.Atoi(s[1:]); err == nil && n >= 1 && n <= 24 && strconv.Itoa(n) == s[1:] {
			return uint16(0x70 + n - 1), 0, true
		}
	}
	for name, code := range keyNames {
		if strings.EqualFold(s, name) {
			return code, 0, true
		}
	}
	upper := strings.ToUpper(s)
	if hex, found := strings.CutPrefix(upper, "VK"); found && len(hex) == 2 {
		if n, err := strconv.ParseUint(hex, 16, 8); err == nil && n != 0 && n != 0xFF && !IsModifierKey(uint16(n)) {
			return uint16(n), 0, true
		}
	}
	if hex, found := strings.CutPrefix(upper, "SC"); found && len(hex) >= 2 && len(hex) <= 4 {
		if n, err := strconv.ParseUint(hex, 16, 16); err == nil && n != 0 {
			return 0, uint16(n), true
		}
	}
	return 0, 0, false
}

// String writes the combination as Parse reads it, modifiers in a fixed order: "Ctrl+Shift+Q".
func (c Combo) String() string {
	var b strings.Builder
	for _, m := range modifierNames {
		if c.Mods&m.mod != 0 {
			b.WriteString(m.name + "+")
		}
	}
	if c.Key == 0 {
		fmt.Fprintf(&b, "SC%03X", c.Scan)
	} else {
		b.WriteString(KeyName(c.Key))
	}
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

// NeedsModifier reports whether the combination takes a key used in typing and in apps, such as a letter, a digit, F1 to F12, an arrow or Enter, without a modifier. Bound alone, such a key would stop working everywhere, so it is refused. Keys made for shortcuts work alone: F13 to F24, the media, browser and launch keys, and keys without a name, as extra keyboards and macro pads send.
func (c Combo) NeedsModifier() bool {
	if c.Mods != 0 || c.Key == 0 {
		return false
	}
	k := c.Key
	switch {
	case k >= 'A' && k <= 'Z', k >= '0' && k <= '9':
		return true
	case k >= 0x70 && k <= 0x7B: // F1 to F12
		return true
	case k >= 0x60 && k <= 0x6F: // the numeric keypad
		return true
	case k >= 0xBA && k <= 0xC0, k >= 0xDB && k <= 0xDE: // punctuation
		return true
	}
	switch k {
	case 0x08, 0x09, 0x0D, 0x1B, 0x20, // Backspace, Tab, Enter, Escape, Space
		0x21, 0x22, 0x23, 0x24, 0x25, 0x26, 0x27, 0x28, // PageUp, PageDown, End, Home, arrows
		0x2C, 0x2D, 0x2E, 0x90: // PrintScreen, Insert, Delete, NumLock
		return true
	}
	return false
}
