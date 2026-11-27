// Copyright (C) 2026 otoneko.
// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"image"
	"image/color"
	"image/draw"

	"github.com/otnc/Volm/internal/trayicon"
)

// previewSheet lays out what to check after changing the artwork: the logo on a light and a dark background, the icon large, and the tray icon in the states of traySamples at each of traySizes, at its real size and enlarged without smoothing, on both kinds of taskbar.
func previewSheet() *image.NRGBA {
	const (
		width   = 1200
		pad     = 40
		logoH   = 160
		iconPx  = 320
		zoom    = 4
		trayGap = 20
	)
	light := color.NRGBA{0xF3, 0xF3, 0xF3, 0xFF}
	dark := color.NRGBA{0x1C, 0x1C, 0x1F, 0xFF}
	rowH := logoH + pad*2
	height := rowH*2 + iconPx + pad*2
	for _, size := range traySizes {
		height += (size*zoom + pad) * 2
	}
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

	for _, size := range traySizes {
		rowH := size*zoom + pad
		for _, bg := range []color.NRGBA{light, dark} {
			fill(sheet, image.Rect(0, y, width, y+rowH), bg)
			x := pad
			for _, s := range traySamples {
				small := trayicon.Draw(s.percent, s.hasDevice, s.muted, size)
				paste(sheet, small, image.Pt(x, y+pad/2))
				paste(sheet, enlarge(small, zoom), image.Pt(x+size+trayGap/2, y+pad/2))
				x += size + trayGap/2 + 32*zoom + trayGap
			}
			y += rowH
		}
	}
	return sheet
}

// traySizes are the tray icon sizes at display scales of 100%, 125%, 150% and 200%.
var traySizes = []int{16, 20, 24, 32}

// traySamples are the states the sheet shows, left to right: 0, 10, 50 and 100%, 50% muted, and no playback device.
var traySamples = []struct {
	percent          int
	hasDevice, muted bool
}{
	{0, true, false}, {10, true, false}, {50, true, false}, {100, true, false}, {50, true, true}, {0, false, false},
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
