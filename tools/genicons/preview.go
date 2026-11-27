// Copyright (C) 2026 otoneko.
// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"image"
	"image/color"
	"image/draw"

	"github.com/otnc/Volm/internal/tray"
)

// previewSheet lays out what to check after changing the artwork: the logo on a light and a dark background, the icon large, and tray icons for a few volumes at 16 and 24 px (100% and 150% scale), at their real size and enlarged without smoothing, on both kinds of taskbar.
func previewSheet() *image.NRGBA {
	const (
		width   = 1200
		pad     = 40
		logoH   = 160
		iconPx  = 320
		trayPx  = 24
		zoom    = 4
		trayGap = 20
	)
	light := color.NRGBA{0xF3, 0xF3, 0xF3, 0xFF}
	dark := color.NRGBA{0x1C, 0x1C, 0x1F, 0xFF}
	rowH := logoH + pad*2
	trayRowH := trayPx*zoom + pad*2
	height := rowH*2 + iconPx + pad*2 + trayRowH*2
	sheet := image.NewNRGBA(image.Rect(0, 0, width, height))

	y := 0
	logo := logoArt().renderHeight(logoH)
	for _, bg := range []color.NRGBA{light, dark} {
		fill(sheet, image.Rect(0, y, width, y+rowH), bg)
		paste(sheet, logo, image.Pt(pad, y+pad))
		y += rowH
	}

	fill(sheet, image.Rect(0, y, width, y+iconPx+pad*2), light)
	paste(sheet, iconArt().render(iconPx), image.Pt(pad, y+pad))
	y += iconPx + pad*2

	samples := []struct {
		label string
		muted bool
		size  int
	}{
		{"8", false, 16}, {"40", false, 16}, {"100", false, 16}, {"40", true, 16}, {"-", false, 16},
		{"40", false, 24}, {"100", false, 24},
	}
	for _, bg := range []color.NRGBA{light, dark} {
		fill(sheet, image.Rect(0, y, width, y+trayRowH), bg)
		x := pad
		for _, s := range samples {
			small := tray.Draw(s.label, s.muted, s.size)
			paste(sheet, small, image.Pt(x, y+pad))
			paste(sheet, enlarge(small, zoom), image.Pt(x+trayPx+trayGap/2, y+pad))
			x += trayPx + trayGap/2 + s.size*zoom + trayGap
		}
		y += trayRowH
	}
	return sheet
}

func fill(dst *image.NRGBA, r image.Rectangle, c color.NRGBA) {
	draw.Draw(dst, r, &image.Uniform{c}, image.Point{}, draw.Src)
}

func paste(dst *image.NRGBA, src image.Image, at image.Point) {
	draw.Draw(dst, src.Bounds().Add(at), src, src.Bounds().Min, draw.Over)
}

// enlarge scales an image up by an integer factor without smoothing, so that single pixels stay visible.
func enlarge(src *image.NRGBA, factor int) *image.NRGBA {
	b := src.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx()*factor, b.Dy()*factor))
	for y := range out.Bounds().Dy() {
		for x := range out.Bounds().Dx() {
			out.SetNRGBA(x, y, src.NRGBAAt(x/factor, y/factor))
		}
	}
	return out
}
