// Copyright (C) 2026 otoneko.
// SPDX-License-Identifier: GPL-3.0-only

package trayicon

import (
	"bytes"
	"image"
	"image/png"
	"strconv"
	"testing"
)

// iconSizes are the small icon sizes of the display scales from 100% to 300%.
var iconSizes = []int{16, 20, 24, 28, 32, 40, 48}

func TestLabel(t *testing.T) {
	tests := []struct {
		percent   int
		hasDevice bool
		want      string
	}{
		{0, true, "0"},
		{7, true, "7"},
		{40, true, "40"},
		{100, true, "100"},
		{40, false, "-"},
	}
	for _, tt := range tests {
		if got := Label(tt.percent, tt.hasDevice); got != tt.want {
			t.Errorf("Label(%d, %v) = %q, want %q", tt.percent, tt.hasDevice, got, tt.want)
		}
	}
}

// TestEveryLabelFits checks that every volume, "100" included, is drawn with whole counters and that none of its pixels touches the scale or the speaker, at every size.
func TestEveryLabelFits(t *testing.T) {
	for _, size := range iconSizes {
		scale := scaleFor(float64(size))
		sp := speakerFor(scale)
		others := []func(x, y float64) bool{scale.arc(scaleStart, scaleStart+scaleSweep), sp.body(), sp.wave(), sp.slash()}
		for percent := range 101 {
			label := strconv.Itoa(percent)
			m, _ := numberLayout(label, size)
			if m.digit-2*m.stroke < 1 || m.counter < 1 {
				t.Fatalf("size %d: a %d × %d px digit with %d px strokes has no counter", size, m.digit, m.height, m.stroke)
			}
			for _, p := range numberPixels(label, size) {
				for _, other := range others {
					if covers(other, p) {
						t.Errorf("size %d: pixel %v of %s touches the scale or the speaker", size, p, label)
					}
				}
			}
		}
	}
}

// numberPixels lists the pixels Draw inks for label.
func numberPixels(label string, size int) []image.Point {
	m, at := numberLayout(label, size)
	var pixels []image.Point
	x := at.X
	for i, r := range label {
		if i > 0 {
			x += m.gap
		}
		for _, seg := range m.glyph(r) {
			seg = seg.Add(image.Pt(x, at.Y))
			for py := seg.Min.Y; py < seg.Max.Y; py++ {
				for px := seg.Min.X; px < seg.Max.X; px++ {
					pixels = append(pixels, image.Pt(px, py))
				}
			}
		}
		x += m.advance(r)
	}
	return pixels
}

// covers reports whether the shape reaches into pixel p at any of the samples paint uses.
func covers(shape func(x, y float64) bool, p image.Point) bool {
	for sy := range 4 {
		for sx := range 4 {
			if shape(float64(p.X)+(float64(sx)+0.5)/4, float64(p.Y)+(float64(sy)+0.5)/4) {
				return true
			}
		}
	}
	return false
}

// TestDigitsDiffer guards against two characters sharing a shape, which would make the icon show a wrong number.
func TestDigitsDiffer(t *testing.T) {
	seen := map[int]rune{}
	for _, r := range "0123456789-" {
		segments, ok := segmentsOf[r]
		if !ok || segments == 0 {
			t.Fatalf("%q has no segments", r)
		}
		if other, ok := seen[segments]; ok {
			t.Errorf("%q and %q have the same segments", other, r)
		}
		seen[segments] = r
	}
}

func TestDrawSize(t *testing.T) {
	for _, size := range iconSizes {
		img, err := png.Decode(bytes.NewReader(EncodePNG(Draw(100, true, false, size))))
		if err != nil {
			t.Fatalf("size %d: %v", size, err)
		}
		if b := img.Bounds(); b.Dx() != size || b.Dy() != size {
			t.Errorf("size %d: drew %v", size, b.Size())
		}
	}
}

// TestDrawInk checks the colors at the middle segment of "8": white on indigo normally, half-transparent white over the grey tile while muted.
func TestDrawInk(t *testing.T) {
	const size = 32
	m, corner := numberLayout("8", size)
	middle := image.Pt(corner.X+m.digit/2, corner.Y+m.stroke+m.counter)
	tests := []struct {
		name  string
		muted bool
		ink   [4]uint8
		tile  [4]uint8
	}{
		{"live", false, [4]uint8{0xFF, 0xFF, 0xFF, 0xFF}, [4]uint8{0x3F, 0x3C, 0xBB, 0xFF}},
		// White at 50% over #6B7280 rounds to #B5B9C0.
		{"muted", true, [4]uint8{0xB5, 0xB9, 0xC0, 0xFF}, [4]uint8{0x6B, 0x72, 0x80, 0xFF}},
	}
	for _, tt := range tests {
		img := Draw(8, true, tt.muted, size)
		if got := rgba(img, middle); got != tt.ink {
			t.Errorf("%s: ink %x, want %x", tt.name, got, tt.ink)
		}
		// The top and the upper left segments overlap at the corner, which must not come out lighter than the rest.
		if got := rgba(img, corner); got != tt.ink {
			t.Errorf("%s: corner %x, want %x", tt.name, got, tt.ink)
		}
		// Just above the middle segment lies the upper counter, where the tile shows.
		if got := rgba(img, middle.Sub(image.Pt(0, 1))); got != tt.tile {
			t.Errorf("%s: counter %x, want %x", tt.name, got, tt.tile)
		}
	}
}

// TestDrawSeparatesDigits checks that a column of tile is left between two digits, so that "88" does not read as one shape.
func TestDrawSeparatesDigits(t *testing.T) {
	for _, size := range iconSizes {
		m, at := numberLayout("88", size)
		img := Draw(88, true, false, size)
		y := at.Y // the top segment, which spans each digit's full width
		// The column right after the first digit must be tile even if the gap shrank to nothing.
		x := at.X + m.digit
		if got := rgba(img, image.Pt(x, y)); got != [4]uint8{0x3F, 0x3C, 0xBB, 0xFF} {
			t.Errorf("size %d: pixel (%d, %d) after the first digit is %x", size, x, y, got)
		}
	}
}

// TestScaleFollowsVolume checks the amber fill of the scale against the middle of its left and right sides, which lie at 25% and 75% of its sweep.
func TestScaleFollowsVolume(t *testing.T) {
	const size = 32
	scale := scaleFor(size)
	left := image.Pt(int(scale.cx-scale.radius), int(scale.cy))
	right := image.Pt(int(scale.cx+scale.radius), int(scale.cy))
	tests := []struct {
		name                  string
		percent               int
		hasDevice, muted      bool
		leftAmber, rightAmber bool
	}{
		{"empty at 0%", 0, true, false, false, false},
		{"left side at 50%", 50, true, false, true, false},
		{"both sides at 100%", 100, true, false, true, true},
		{"empty while muted", 100, true, true, false, false},
		{"empty without a device", 100, false, false, false, false},
	}
	for _, tt := range tests {
		img := Draw(tt.percent, tt.hasDevice, tt.muted, size)
		if got := rgba(img, left) == rgba(fullAmber(), image.Point{}); got != tt.leftAmber {
			t.Errorf("%s: left side amber = %v, want %v (%x)", tt.name, got, tt.leftAmber, rgba(img, left))
		}
		if got := rgba(img, right) == rgba(fullAmber(), image.Point{}); got != tt.rightAmber {
			t.Errorf("%s: right side amber = %v, want %v (%x)", tt.name, got, tt.rightAmber, rgba(img, right))
		}
	}
}

// TestSlashOnlyWhileMuted checks that the red slash crosses the speaker while muted and never shows otherwise.
func TestSlashOnlyWhileMuted(t *testing.T) {
	for _, size := range iconSizes {
		if hasRed(Draw(50, true, false, size)) {
			t.Errorf("size %d: red without mute", size)
		}
		if !hasRed(Draw(50, true, true, size)) {
			t.Errorf("size %d: no red slash while muted", size)
		}
	}
}

func hasRed(img *image.NRGBA) bool {
	for y := range img.Bounds().Dy() {
		for x := range img.Bounds().Dx() {
			if c := img.NRGBAAt(x, y); c.R > 0xC0 && c.G < 0x80 && c.B < 0x80 {
				return true
			}
		}
	}
	return false
}

func fullAmber() *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	img.SetNRGBA(0, 0, amber)
	return img
}

func rgba(img *image.NRGBA, p image.Point) [4]uint8 {
	c := img.NRGBAAt(p.X, p.Y)
	return [4]uint8{c.R, c.G, c.B, c.A}
}
