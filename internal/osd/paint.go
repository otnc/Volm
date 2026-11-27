// Copyright (C) 2026 otoneko.
// SPDX-License-Identifier: GPL-3.0-only

package osd

import (
	"fmt"
	"unsafe"

	"github.com/otnc/Volm/internal/win32"
	"golang.org/x/sys/windows"
)

// Icon is a glyph of the system icon font.
type Icon rune

// Code points shared by Segoe Fluent Icons (Windows 11) and Segoe MDL2 Assets (Windows 10).
const (
	IconMute        Icon = 0xE74F
	IconSpeakerZero Icon = 0xE992
	IconSpeakerLow  Icon = 0xE993
	IconSpeakerMid  Icon = 0xE994
	IconSpeakerHigh Icon = 0xE995
	IconBrightness  Icon = 0xE706
	IconMicrophone  Icon = 0xE720
	IconMicOff      Icon = 0xEC54
)

// MicrophoneIcon returns the microphone glyph, crossed out while muted.
func MicrophoneIcon(muted bool) Icon {
	if muted {
		return IconMicOff
	}
	return IconMicrophone
}

// SpeakerIcon returns the speaker glyph for a volume.
func SpeakerIcon(percent int, muted bool) Icon {
	switch {
	case muted:
		return IconMute
	case percent == 0:
		return IconSpeakerZero
	case percent < 34:
		return IconSpeakerLow
	case percent < 67:
		return IconSpeakerMid
	default:
		return IconSpeakerHigh
	}
}

type fonts struct {
	scale              float64
	icon, title, value uintptr
}

// ensure creates the fonts for scale, replacing those of another scale.
func (f *fonts) ensure(scale float64) {
	if f.icon != 0 && f.scale == scale {
		return
	}
	f.release()
	f.scale = scale
	px := func(v int) int32 { return int32(float64(v)*scale + 0.5) }
	f.icon = createFont(iconFontFace(), px(26), 400)
	f.title = createFont("Yu Gothic UI", px(14), 400)
	f.value = createFont("Yu Gothic UI", px(15), 600)
}

func (f *fonts) release() {
	for _, h := range []uintptr{f.icon, f.title, f.value} {
		if h != 0 {
			win32.ProcDeleteObject.Call(h)
		}
	}
	*f = fonts{}
}

// iconFontFace picks the icon font that ships with the running Windows: GDI substitutes another font for a missing face, which draws the private use code points as boxes.
func iconFontFace() string {
	if windows.RtlGetVersion().BuildNumber >= 22000 {
		return "Segoe Fluent Icons"
	}
	return "Segoe MDL2 Assets"
}

func createFont(face string, height int32, weight int32) uintptr {
	name, _ := windows.UTF16PtrFromString(face)
	const cleartypeQuality = 5
	h, _, _ := win32.ProcCreateFontW.Call(uintptr(-height), 0, 0, 0, uintptr(weight), 0, 0, 0, 1, 0, 0, cleartypeQuality, 0, uintptr(unsafe.Pointer(name)))
	return h
}

// paint draws the panel into an off-screen bitmap and copies it at once, so that a quick succession of updates does not flicker.
func (o *OSD) paint() {
	var ps win32.PaintStruct
	hdc, _, _ := win32.ProcBeginPaint.Call(o.hwnd, uintptr(unsafe.Pointer(&ps)))
	defer win32.ProcEndPaint.Call(o.hwnd, uintptr(unsafe.Pointer(&ps)))

	w, h := o.px(panelWidth), o.px(panelHeight)
	mem, _, _ := win32.ProcCreateCompatibleDC.Call(hdc)
	defer win32.ProcDeleteDC.Call(mem)
	bmp, _, _ := win32.ProcCreateCompatibleBitmap.Call(hdc, uintptr(w), uintptr(h))
	defer win32.ProcDeleteObject.Call(bmp)
	oldBmp, _, _ := win32.ProcSelectObject.Call(mem, bmp)
	defer win32.ProcSelectObject.Call(mem, oldBmp)

	c := o.shown.content
	t := o.theme
	fillRect(mem, win32.Rect{Right: w, Bottom: h}, t.background)
	win32.ProcSetBkMode.Call(mem, win32.TRANSPARENT)
	win32.ProcSetTextColor.Call(mem, uintptr(t.text))

	pad := o.px(16)
	iconRight := pad + o.px(36)
	drawText(mem, o.fonts.icon, string(rune(c.Icon)), win32.Rect{Left: pad, Top: 0, Right: iconRight, Bottom: h}, win32.DT_LEFT)

	textLeft := iconRight + o.px(12)
	valueLeft := w - pad - o.px(80)
	drawText(mem, o.fonts.title, c.Title, win32.Rect{Left: textLeft, Top: o.px(12), Right: w - pad, Bottom: o.px(36)}, win32.DT_LEFT|win32.DT_END_ELLIPSIS)

	value := fmt.Sprintf("%d%%", c.Percent)
	fill := t.fill
	if c.Muted {
		value = "ミュート中"
		fill = t.mutedFill
	}
	percent := c.Percent
	if c.Status != "" {
		value, percent = c.Status, 0
	}
	barTop, barHeight := o.px(48), max(o.px(6), 2)
	bar := win32.Rect{Left: textLeft, Top: barTop, Right: valueLeft - o.px(12), Bottom: barTop + barHeight}
	fillRect(mem, bar, t.track)
	filled := bar
	filled.Right = bar.Left + bar.Width()*int32(min(max(percent, 0), 100))/100
	fillRect(mem, filled, fill)
	drawText(mem, o.fonts.value, value, win32.Rect{Left: valueLeft, Top: barTop - o.px(12), Right: w - pad, Bottom: barTop + barHeight + o.px(12)}, win32.DT_RIGHT)

	win32.ProcBitBlt.Call(hdc, 0, 0, uintptr(w), uintptr(h), mem, 0, 0, win32.SRCCOPY)
}

func fillRect(hdc uintptr, r win32.Rect, color uint32) {
	brush, _, _ := win32.ProcCreateSolidBrush.Call(uintptr(color))
	defer win32.ProcDeleteObject.Call(brush)
	win32.ProcFillRect.Call(hdc, uintptr(unsafe.Pointer(&r)), brush)
}

func drawText(hdc, font uintptr, text string, r win32.Rect, align uintptr) {
	old, _, _ := win32.ProcSelectObject.Call(hdc, font)
	defer win32.ProcSelectObject.Call(hdc, old)
	s, _ := windows.UTF16FromString(text)
	win32.ProcDrawTextW.Call(hdc, uintptr(unsafe.Pointer(&s[0])), uintptr(len(s)-1), uintptr(unsafe.Pointer(&r)), align|win32.DT_VCENTER|win32.DT_SINGLELINE|win32.DT_NOPREFIX)
}
