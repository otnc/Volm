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

// TestEveryLabelFits checks that every volume, "100" included, fits inside the icon with whole counters, so that no digit is cut off or fills in at small sizes.
func TestEveryLabelFits(t *testing.T) {
	for _, size := range iconSizes {
		m := metricsFor(size)
		if m.height > size {
			t.Errorf("size %d: glyphs are %d px tall", size, m.height)
		}
		if m.digit-2*m.stroke < 1 {
			t.Errorf("size %d: a %d px digit with %d px strokes has no counter", size, m.digit, m.stroke)
		}
		for percent := range 101 {
			if w := m.width(strconv.Itoa(percent)); w > size {
				t.Errorf("size %d: %d is %d px wide", size, percent, w)
			}
		}
	}
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
		img, err := png.Decode(bytes.NewReader(EncodePNG(Draw("100", false, size))))
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
	const size = 20
	m := metricsFor(size)
	corner := image.Pt((size-m.digit)/2, (size-m.height)/2)
	middle := image.Pt(size/2, corner.Y+m.stroke+m.counter)
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
		img := Draw("8", tt.muted, size)
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
		m := metricsFor(size)
		img := Draw("88", false, size)
		left := (size - m.width("88")) / 2
		y := (size - m.height) / 2 // the top segment, which spans each digit's full width
		// The column right after the first digit must be tile even if the gap shrank to nothing.
		x := left + m.digit
		if got := rgba(img, image.Pt(x, y)); got != [4]uint8{0x3F, 0x3C, 0xBB, 0xFF} {
			t.Errorf("size %d: pixel (%d, %d) after the first digit is %x", size, x, y, got)
		}
	}
}

func rgba(img *image.NRGBA, p image.Point) [4]uint8 {
	c := img.NRGBAAt(p.X, p.Y)
	return [4]uint8{c.R, c.G, c.B, c.A}
}
