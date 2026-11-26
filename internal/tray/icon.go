// Copyright (C) 2026 otoneko.
// SPDX-License-Identifier: GPL-3.0-only

package tray

import _ "embed"

// Level is the volume band the tray icon shows.
type Level int

const (
	LevelMuted Level = iota
	LevelLow
	LevelMid
	LevelHigh
)

// LevelOf returns the icon band for a volume. Zero percent shows as muted, since nothing is heard either way.
func LevelOf(percent int, muted bool) Level {
	switch {
	case muted || percent == 0:
		return LevelMuted
	case percent < 34:
		return LevelLow
	case percent < 67:
		return LevelMid
	default:
		return LevelHigh
	}
}

// The icons are drawn by tools/genicons; run it again after changing them.
var (
	//go:embed icons/muted.png
	mutedIcon []byte
	//go:embed icons/low.png
	lowIcon []byte
	//go:embed icons/mid.png
	midIcon []byte
	//go:embed icons/high.png
	highIcon []byte
)

func levelIcon(l Level) []byte {
	return [...][]byte{LevelMuted: mutedIcon, LevelLow: lowIcon, LevelMid: midIcon, LevelHigh: highIcon}[l]
}
