// Copyright (C) 2026 otoneko.
// SPDX-License-Identifier: GPL-3.0-only

// Package trayicon draws the tray icon: the app icon's scale and the volume as a number on a tile. It does no Windows calls, so tools/genicons can draw the icon on any OS.
package trayicon

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"strconv"
)

// The colors match the app icon drawn by tools/genicons. The tile makes the icon read on light and dark taskbars alike.
var (
	indigo = color.NRGBA{0x3F, 0x3C, 0xBB, 0xFF}
	slate  = color.NRGBA{0x6B, 0x72, 0x80, 0xFF}
	amber  = color.NRGBA{0xFF, 0xB2, 0x1E, 0xFF}
	// red crosses out the speaker while muted.
	red   = color.NRGBA{0xE5, 0x48, 0x4D, 0xFF}
	white = color.NRGBA{0xFF, 0xFF, 0xFF, 0xFF}
	// scaleTrack is the empty part of the scale, white at 28% as in the app icon.
	scaleTrack = color.NRGBA{0xFF, 0xFF, 0xFF, 0x47}
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

// Draw paints the tray icon at size × size pixels, after the app icon: on an indigo tile, the amber scale open at the bottom, filled as far as the volume, with the volume as a number where the app icon has its knob. While muted the tile turns grey, the scale empties and the number fades. Without a playback device the scale is empty and the number is a dash.
//
// The digits are drawn from segments on whole pixels instead of from a font, so that "100" still reads inside the scale at 16 px:
//
//	img := trayicon.Draw(40, true, false, 20)
func Draw(percent int, hasDevice, muted bool, size int) *image.NRGBA {
	tile, ink := indigo, white
	if muted {
		tile, ink = slate, faded
	}
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	s := float64(size)
	paint(img, roundedSquare(s, s/8), tile)

	scale := scaleFor(s)
	paint(img, scale.arc(scaleStart, scaleStart+scaleSweep), scaleTrack)
	if fill := float64(min(max(percent, 0), 100)) / 100; hasDevice && !muted && fill > 0 {
		paint(img, scale.arc(scaleStart, scaleStart+scaleSweep*fill), amber)
	}

	sp := speakerFor(scale)
	paint(img, sp.body(), white)
	if muted {
		paint(img, sp.slash(), red)
	} else {
		paint(img, sp.wave(), white)
	}

	// The segments overlap at their corners, so they are gathered into a mask first; blending each segment would paint the corners of the faded number twice.
	label := Label(percent, hasDevice)
	mask := image.NewAlpha(img.Bounds())
	m, at := numberLayout(label, size)
	x := at.X
	for i, r := range label {
		if i > 0 {
			x += m.gap
		}
		for _, seg := range m.glyph(r) {
			draw.Draw(mask, seg.Add(image.Pt(x, at.Y)), image.Opaque, image.Point{}, draw.Src)
		}
		x += m.advance(r)
	}
	// The outline, in the tile's color, keeps the number readable where it crosses the scale.
	blend(img, dilate(mask, outlineWidth(size)), tile)
	blend(img, mask, ink)
	return img
}

// outlineWidth is how far the tile-colored outline reaches around the number: 1 px up to 31 px, then 1 px more for every 16.
func outlineWidth(size int) int { return max(1, size/16) }

// dilate returns mask grown by n pixels in every direction, corners included.
func dilate(mask *image.Alpha, n int) *image.Alpha {
	b := mask.Bounds()
	out := image.NewAlpha(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if mask.AlphaAt(x, y).A == 0 {
				continue
			}
			draw.Draw(out, image.Rect(x-n, y-n, x+n+1, y+n+1).Intersect(b), image.Opaque, image.Point{}, draw.Src)
		}
	}
	return out
}

// numberLayout sizes the number and returns its top-left corner, centered across the icon. The number with its outline may cross the sides of the scale, but stays below its top, a pixel inside the tile's edge and above the speaker. Every label gets the size "100", the widest, needs, so that the digits keep one size as the volume changes.
func numberLayout(label string, size int) (metrics, image.Point) {
	scale := scaleFor(float64(size))
	o := outlineWidth(size)
	// The top of the number stays below the inner edge of the scale, so that the scale's top, where most volumes end, is never covered.
	top := int(math.Ceil(scale.cy - scale.radius + scale.width/2))
	speakerTop := int(speakerFor(scale).top())
	m := metricsFor(size-2*(o+1), speakerTop-1-o-top)
	// The number is sized with a pixel to spare above the speaker, then set down onto it, so that its outline meets the speaker without covering it and more of the scale's top shows.
	return m, image.Pt((size-m.width(label))/2, speakerTop-o-m.height)
}

// The scale runs clockwise through 270°, from 135° left of 12 o'clock to 135° right of it, as in the app icon.
const (
	scaleStart = -135.0
	scaleSweep = 270.0
)

// scaleRing is the scale's ring within an icon of one size, in pixels.
type scaleRing struct {
	cx, cy, radius, width float64
}

func scaleFor(size float64) scaleRing {
	width := max(1.5, size*0.11)
	// The margin to the tile's edge shrinks faster than the icon, so that "100" still fits inside the ring at 16 px.
	margin := max(0, size*0.05-0.5)
	return scaleRing{cx: size / 2, cy: size / 2, radius: size/2 - margin - width/2, width: width}
}

// speaker is the small speaker in the opening at the bottom of the scale, as in the app icon: a box, a cone and one sound wave, drawn in units of the app icon's 48-unit grid around its center.
type speaker struct {
	cx, cy, unit float64
}

// speakerFor centers the speaker between the ends of the scale's opening. It is drawn a quarter larger than in the app icon, so that it still reads at 16 px.
func speakerFor(r scaleRing) speaker {
	return speaker{cx: r.cx, cy: r.cy + r.radius*0.8, unit: r.cx * 2 / 48 * 1.25}
}

// top returns the highest pixel row the speaker reaches.
func (s speaker) top() float64 { return s.cy - 4.5*s.unit }

// The box, the cone and the wave lie at these offsets from the speaker's center, in grid units; together they span -5.5 to 5.5 across.
func (s speaker) at(dx, dy float64) (float64, float64) { return s.cx + dx*s.unit, s.cy + dy*s.unit }

func (s speaker) body() func(x, y float64) bool {
	bx0, by0 := s.at(-5.5, -2)
	bx1, by1 := s.at(-2.5, 2)
	cone := [][2]float64{}
	for _, p := range [][2]float64{{-3, -2}, {0.5, -4.5}, {0.5, 4.5}, {-3, 2}} {
		x, y := s.at(p[0], p[1])
		cone = append(cone, [2]float64{x, y})
	}
	return func(x, y float64) bool {
		return (x >= bx0 && x <= bx1 && y >= by0 && y <= by1) || inPolygon(cone, x, y)
	}
}

// wave is the sound wave in front of the cone, which the speaker loses while muted.
func (s speaker) wave() func(x, y float64) bool {
	ring := scaleRing{radius: 3.8 * s.unit, width: max(1, 1.6*s.unit)}
	ring.cx, ring.cy = s.at(0.5, 0)
	return ring.arc(45, 135)
}

// slash is the red stroke across the muted speaker, from its upper left to its lower right.
func (s speaker) slash() func(x, y float64) bool {
	x1, y1 := s.at(-6, -4)
	x2, y2 := s.at(5.5, 4.5)
	return thickLine(x1, y1, x2, y2, max(1.2, 1.8*s.unit))
}

func inPolygon(points [][2]float64, x, y float64) bool {
	inside := false
	for i, j := 0, len(points)-1; i < len(points); j, i = i, i+1 {
		xi, yi, xj, yj := points[i][0], points[i][1], points[j][0], points[j][1]
		if (yi > y) != (yj > y) && x < (xj-xi)*(y-yi)/(yj-yi)+xi {
			inside = !inside
		}
	}
	return inside
}

// thickLine is a stroke of the given width between two points, with round ends.
func thickLine(x1, y1, x2, y2, width float64) func(x, y float64) bool {
	dx, dy := x2-x1, y2-y1
	length2 := dx*dx + dy*dy
	return func(x, y float64) bool {
		t := min(max(((x-x1)*dx+(y-y1)*dy)/length2, 0), 1)
		return math.Hypot(x-(x1+t*dx), y-(y1+t*dy)) <= width/2
	}
}

// arc returns the stroke of the ring from one angle to another, in degrees clockwise from 12 o'clock, with round ends.
func (r scaleRing) arc(from, to float64) func(x, y float64) bool {
	end := func(deg float64) (float64, float64) {
		rad := deg * math.Pi / 180
		return r.cx + r.radius*math.Sin(rad), r.cy - r.radius*math.Cos(rad)
	}
	x1, y1 := end(from)
	x2, y2 := end(to)
	half := r.width / 2
	return func(x, y float64) bool {
		if math.Hypot(x-x1, y-y1) <= half || math.Hypot(x-x2, y-y2) <= half {
			return true
		}
		if math.Abs(math.Hypot(x-r.cx, y-r.cy)-r.radius) > half {
			return false
		}
		deg := math.Atan2(x-r.cx, r.cy-y) * 180 / math.Pi
		return deg >= from && deg <= to
	}
}

// EncodePNG encodes an icon from Draw as PNG, the format a tray icon is set from.
func EncodePNG(img image.Image) []byte {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(err) // png.Encode fails only on write errors, and a bytes.Buffer does not fail
	}
	return buf.Bytes()
}

// metrics size the glyphs for the box they must fit in. Every glyph is height tall, built from strokes stroke wide around two counters of counter pixels; "1" is a single stroke wide, so that "100" fits as well as two full digits do.
type metrics struct {
	stroke, counter, digit, gap, height int
}

func metricsFor(w, h int) metrics {
	m := metrics{stroke: max(1, (h+3)/6), gap: 1}
	m.digit = max(2*m.stroke+1, (w-m.stroke-2*m.gap)/2)
	m.counter = max(1, (h-3*m.stroke)/2)
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

// roundedSquare is the tile: the whole icon with corners of the given radius.
func roundedSquare(size, radius float64) func(x, y float64) bool {
	return func(x, y float64) bool {
		cx := min(max(x, radius), size-radius)
		cy := min(max(y, radius), size-radius)
		return math.Hypot(x-cx, y-cy) <= radius
	}
}

// paint draws c over the shape, sampling each pixel on a 4 × 4 grid so that curved edges are smooth.
func paint(img *image.NRGBA, inside func(x, y float64) bool, c color.NRGBA) {
	const samples = 4
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
				over(img, px, py, color.NRGBA{c.R, c.G, c.B, uint8(int(c.A) * hits / (samples * samples))})
			}
		}
	}
}

// blend draws c over the pixels set in mask, so that the faded number lets the tile show through.
func blend(img *image.NRGBA, mask *image.Alpha, c color.NRGBA) {
	for y := range img.Bounds().Dy() {
		for x := range img.Bounds().Dx() {
			if mask.AlphaAt(x, y).A != 0 {
				over(img, x, y, c)
			}
		}
	}
}

// over composites c over the pixel at x, y.
func over(img *image.NRGBA, x, y int, c color.NRGBA) {
	a := float64(c.A) / 255
	dst := img.NRGBAAt(x, y)
	da := float64(dst.A) / 255
	oa := a + da*(1-a)
	if oa == 0 {
		return
	}
	mix := func(s, d uint8) uint8 {
		return uint8(math.Round((float64(s)*a + float64(d)*da*(1-a)) / oa))
	}
	img.SetNRGBA(x, y, color.NRGBA{mix(c.R, dst.R), mix(c.G, dst.G), mix(c.B, dst.B), uint8(math.Round(oa * 255))})
}
