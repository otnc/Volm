// Copyright (C) 2026 otoneko.
// SPDX-License-Identifier: GPL-3.0-only

package main

import "math"

// The icon is a volume knob seen from the front, the control Volm turns when the wheel rolls over the taskbar. Everything sits on a 48-unit grid around one center, and nothing overlaps:
//
//   - a 270° scale arc, open at the bottom, whose filled length is the volume
//   - a round knob inside it, with a pointer at the end of the filled length
//   - in the opening of the arc, a small speaker that says the knob is for sound
//
// The scale is filled all the way. The tray icon is not drawn here: internal/tray draws the volume as a number on the same indigo tile.
//
// The wordmark draws "Volm" with strokes of one width and round ends, and its "o" is the knob: the scale, open at the bottom and amber as in the icon, around an indigo dot.

const gridSize = 48

var (
	indigo      = hex(0x3F3CBB)
	amber       = hex(0xFFB21E)
	white       = hex(0xFFFFFF)
	scaleTrack  = white.withAlpha(0.28)
	scaleStart  = -135.0 // degrees clockwise from 12 o'clock
	scaleSweep  = 270.0
	iconCenterX = 24.0
	// iconCenterY sits above the tile's middle, so that the arc and the speaker under it balance as a whole.
	iconCenterY = 22.0
	scaleRadius = 14.0
	scaleWidth  = 4.0
	knobRadius  = 7.0
)

func iconArt() drawing {
	end := scaleStart + scaleSweep
	d := drawing{width: gridSize, height: gridSize}
	d.shapes = append(d.shapes,
		roundRect{x: 2, y: 2, w: 44, h: 44, radius: 11, fill: indigo},
		arc{cx: iconCenterX, cy: iconCenterY, r: scaleRadius, from: scaleStart, to: scaleStart + scaleSweep, width: scaleWidth, stroke: scaleTrack},
		arc{cx: iconCenterX, cy: iconCenterY, r: scaleRadius, from: scaleStart, to: end, width: scaleWidth, stroke: amber},
		circle{cx: iconCenterX, cy: iconCenterY, r: knobRadius, fill: white},
		pointer(iconCenterX, iconCenterY, end, indigo),
	)
	d.shapes = append(d.shapes, speaker()...)
	return d
}

// speaker sits centered in the opening at the bottom of the scale, clear of the knob and the arc's ends: a box, a cone and one sound wave.
func speaker() []shape {
	const cy = 37.0
	return []shape{
		roundRect{x: 18.5, y: cy - 2, w: 3, h: 4, radius: 0.8, fill: white},
		polygon{points: [][2]float64{{21, cy - 2}, {24.5, cy - 4.5}, {24.5, cy + 4.5}, {21, cy + 2}}, fill: white},
		arc{cx: 24.5, cy: cy, r: 3.5, from: 45, to: 135, width: 1.6, stroke: white},
	}
}

// pointer is the knob's mark, a short stroke from near the center toward angle deg, which stays inside the knob.
func pointer(cx, cy, deg float64, c rgba) line {
	inner := arc{cx: cx, cy: cy, r: 2}
	outer := arc{cx: cx, cy: cy, r: 4.5}
	x1, y1 := inner.point(deg)
	x2, y2 := outer.point(deg)
	return line{x1: x1, y1: y1, x2: x2, y2: y2, width: 3, stroke: c}
}

// Wordmark metrics on the 48-unit grid. Positions are stroke centers; the letters are white strokes with an indigo outline, which reads on light and dark backgrounds alike.
const (
	strokeWidth  = 5.0
	outlineWidth = 2.0 // on each side of a stroke
	capTop       = 12.0
	xTop         = 23.5
	baseline     = 38.0
	letterGap    = 7.0 // between the stroke edges of neighboring letters, wide enough for both outlines
	logoGap      = 12.0
)

// stroke is a letter stroke, which the logo draws twice: wider in indigo for the outline, then in white.
type stroke interface {
	shape
	restyle(width float64, c rgba) shape
}

func (s line) restyle(width float64, c rgba) shape { s.width, s.stroke = width, c; return s }
func (s arc) restyle(width float64, c rgba) shape  { s.width, s.stroke = width, c; return s }

// accent marks the stroke the logo fills in amber instead of white: the o, which repeats the icon's scale.
type accent struct{ arc }

// logoArt places the icon and, to its right, the wordmark.
func logoArt() drawing {
	d := iconArt()
	x := gridSize + logoGap
	half := strokeWidth / 2
	var letters []stroke

	// V: two strokes meeting at the baseline.
	vLeft := x + half
	vRight := vLeft + 18
	letters = append(letters,
		line{x1: vLeft, y1: capTop, x2: vLeft + 9, y2: baseline},
		line{x1: vLeft + 9, y1: baseline, x2: vRight, y2: capTop},
	)
	// The V's right stroke slants inward, so the gap to the o is measured where the o is widest, at the middle of the x-height, as for the other letters. A slanted stroke is wider across than its width by 1/cos of its slant.
	oMiddle := (xTop + baseline) / 2
	slant := math.Atan2(9, baseline-capTop)
	vEdgeAtMiddle := vRight - 9*(oMiddle-capTop)/(baseline-capTop) + half/math.Cos(slant)
	x = vEdgeAtMiddle + letterGap

	// o: the icon's scale, open at the bottom. Its top meets the x-height and the round ends of the opening meet the baseline, as the other letters do: the center sits r below the x-height and r·cos45° above the baseline.
	oRadius := (baseline - xTop) / (1 + math.Sqrt2/2)
	oCenterX := x + oRadius + half
	oCenterY := xTop + oRadius
	letters = append(letters, accent{arc{cx: oCenterX, cy: oCenterY, r: oRadius, from: scaleStart, to: scaleStart + scaleSweep}})
	x = oCenterX + oRadius + half + letterGap

	// l
	lx := x + half
	letters = append(letters, line{x1: lx, y1: capTop, x2: lx, y2: baseline})
	x = lx + half + letterGap

	// m: three stems joined by two half circles that reach the x-height.
	const archRadius = 4.5
	archY := xTop + archRadius
	m0 := x + half
	for i := range 3 {
		sx := m0 + float64(i)*archRadius*2
		letters = append(letters, line{x1: sx, y1: archY, x2: sx, y2: baseline})
	}
	for i := range 2 {
		cx := m0 + archRadius + float64(i)*archRadius*2
		letters = append(letters, arc{cx: cx, cy: archY, r: archRadius, from: -90, to: 90})
	}

	// All outlines first, so that where strokes join, as at the bottom of the V, no outline runs across a letter.
	for _, l := range letters {
		d.shapes = append(d.shapes, l.restyle(strokeWidth+outlineWidth*2, indigo))
	}
	for _, l := range letters {
		fill := white
		if _, ok := l.(accent); ok {
			fill = amber
		}
		d.shapes = append(d.shapes, l.restyle(strokeWidth, fill))
	}
	// The knob's dot in the middle of the o, in the outline's indigo and without an outline of its own, which would merge with the inside of the o's outline.
	d.shapes = append(d.shapes, circle{cx: oCenterX, cy: oCenterY, r: 2.25, fill: indigo})
	d.width = m0 + archRadius*4 + half + outlineWidth
	return d
}
