// Copyright (C) 2026 otoneko.
// SPDX-License-Identifier: GPL-3.0-only

package tray

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"strconv"
)

// The tile colors match the app icon drawn by tools/genicons, so the number reads on light and dark taskbars alike.
var (
	indigo = color.NRGBA{0x3F, 0x3C, 0xBB, 0xFF}
	slate  = color.NRGBA{0x6B, 0x72, 0x80, 0xFF}
	white  = color.NRGBA{0xFF, 0xFF, 0xFF, 0xFF}
	// faded is the number while muted: still readable, but plainly not the live volume.
	faded = color.NRGBA{0xFF, 0xFF, 0xFF, 0x80}
)

// noDeviceLabel is what the icon shows when there is no playback device.
const noDeviceLabel = "-"

// Label returns the text the tray icon shows: the volume, or a dash without a playback device.
func Label(percent int, hasDevice bool) string {
	if !hasDevice {
		return noDeviceLabel
	}
	return strconv.Itoa(min(max(percent, 0), 100))
}

// Draw paints the tray icon at size × size pixels: label in white on an indigo tile, or faded on a grey tile while muted. label holds digits and "-"; other characters are skipped.
//
// The digits are drawn from segments on whole pixels instead of from a font, so that "100" still reads at 16 px:
//
//	img := tray.Draw("40", false, 20)
func Draw(label string, muted bool, size int) *image.NRGBA {
	tile, ink := indigo, white
	if muted {
		tile, ink = slate, faded
	}
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	paintTile(img, tile)
	// The segments overlap at their corners, so they are gathered into a mask first; blending each segment would paint the corners of the faded number twice.
	mask := image.NewAlpha(img.Bounds())
	m := metricsFor(size)
	x := (size - m.width(label)) / 2
	y := (size - m.height) / 2
	for i, r := range label {
		if i > 0 {
			x += m.gap
		}
		for _, s := range m.glyph(r) {
			draw.Draw(mask, s.Add(image.Pt(x, y)), image.Opaque, image.Point{}, draw.Src)
		}
		x += m.advance(r)
	}
	blend(img, mask, ink)
	return img
}

func encodePNG(img image.Image) []byte {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(err) // png.Encode fails only on write errors, and a bytes.Buffer does not fail
	}
	return buf.Bytes()
}

// metrics size the glyphs for one icon size. Every glyph is height tall, built from strokes stroke wide around two counters of counter pixels; "1" is a single stroke wide, so that "100" fits as well as two full digits do.
type metrics struct {
	stroke, counter, digit, gap, height int
}

func metricsFor(size int) metrics {
	m := metrics{
		stroke: max(1, (size+4)/9),
		gap:    max(1, size/16),
	}
	m.digit = (size - m.stroke - 2*m.gap) / 2
	m.counter = max(1, (size*4/5-3*m.stroke)/2)
	m.height = 3*m.stroke + 2*m.counter
	return m
}

func (m metrics) advance(r rune) int {
	if r == '1' {
		return m.stroke
	}
	return m.digit
}

func (m metrics) width(label string) int {
	w := 0
	for i, r := range label {
		if i > 0 {
			w += m.gap
		}
		w += m.advance(r)
	}
	return w
}

// The seven segments, as on a digital display.
const (
	segTop = 1 << iota
	segUpperRight
	segLowerRight
	segBottom
	segLowerLeft
	segUpperLeft
	segMiddle
)

var segmentsOf = map[rune]int{
	'0': segTop | segUpperRight | segLowerRight | segBottom | segLowerLeft | segUpperLeft,
	'1': segUpperRight | segLowerRight,
	'2': segTop | segUpperRight | segMiddle | segLowerLeft | segBottom,
	'3': segTop | segUpperRight | segMiddle | segLowerRight | segBottom,
	'4': segUpperLeft | segMiddle | segUpperRight | segLowerRight,
	'5': segTop | segUpperLeft | segMiddle | segLowerRight | segBottom,
	'6': segTop | segUpperLeft | segMiddle | segLowerLeft | segLowerRight | segBottom,
	'7': segTop | segUpperRight | segLowerRight,
	'8': segTop | segUpperRight | segLowerRight | segBottom | segLowerLeft | segUpperLeft | segMiddle,
	'9': segTop | segUpperRight | segLowerRight | segBottom | segUpperLeft | segMiddle,
	'-': segMiddle,
}

// glyph returns the rectangles of r, relative to its top-left corner. Neighboring segments overlap at the corners, which leaves the corners filled.
func (m metrics) glyph(r rune) []image.Rectangle {
	w, t, h := m.advance(r), m.stroke, m.height
	mid := t + m.counter
	half := 2*t + m.counter
	parts := [...]image.Rectangle{
		image.Rect(0, 0, w, t),       // top
		image.Rect(w-t, 0, w, half),  // upper right
		image.Rect(w-t, mid, w, h),   // lower right
		image.Rect(0, h-t, w, h),     // bottom
		image.Rect(0, mid, t, h),     // lower left
		image.Rect(0, 0, t, half),    // upper left
		image.Rect(0, mid, w, mid+t), // middle
	}
	var rects []image.Rectangle
	for i, rect := range parts {
		if segmentsOf[r]&(1<<i) != 0 {
			rects = append(rects, rect)
		}
	}
	return rects
}

// paintTile fills img with a rounded square, sampling each edge pixel on a 4 × 4 grid so that the corners are smooth.
func paintTile(img *image.NRGBA, c color.NRGBA) {
	const samples = 4
	size := float64(img.Bounds().Dx())
	radius := size / 8
	inside := func(x, y float64) bool {
		cx := min(max(x, radius), size-radius)
		cy := min(max(y, radius), size-radius)
		return math.Hypot(x-cx, y-cy) <= radius
	}
	for py := range img.Bounds().Dy() {
		for px := range img.Bounds().Dx() {
			hits := 0
			for sy := range samples {
				for sx := range samples {
					if inside(float64(px)+(float64(sx)+0.5)/samples, float64(py)+(float64(sy)+0.5)/samples) {
						hits++
					}
				}
			}
			if hits > 0 {
				img.SetNRGBA(px, py, color.NRGBA{c.R, c.G, c.B, uint8(int(c.A) * hits / (samples * samples))})
			}
		}
	}
}

// blend draws c over the pixels set in mask, so that the faded number lets the tile show through.
func blend(img *image.NRGBA, mask *image.Alpha, c color.NRGBA) {
	a := float64(c.A) / 255
	for y := range img.Bounds().Dy() {
		for x := range img.Bounds().Dx() {
			if mask.AlphaAt(x, y).A == 0 {
				continue
			}
			dst := img.NRGBAAt(x, y)
			da := float64(dst.A) / 255
			oa := a + da*(1-a)
			if oa == 0 {
				continue
			}
			mix := func(s, d uint8) uint8 {
				return uint8(math.Round((float64(s)*a + float64(d)*da*(1-a)) / oa))
			}
			img.SetNRGBA(x, y, color.NRGBA{mix(c.R, dst.R), mix(c.G, dst.G), mix(c.B, dst.B), uint8(math.Round(oa * 255))})
		}
	}
}
