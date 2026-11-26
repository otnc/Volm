// Copyright (C) 2026 otoneko.
// SPDX-License-Identifier: GPL-3.0-only

// Package osd shows the on-screen display: a small panel with an icon, a name and a level bar, which fades out after a while.
//
// It is a Win32 layered window painted with GDI rather than a MyGo window: one hidden MyGo window raises the resident process from about 20MB to about 90MB of working set.
package osd

import (
	"sync"
	"time"
	"unsafe"

	"github.com/otnc/Volm/internal/win32"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// Content is what the panel shows.
type Content struct {
	Icon    Icon
	Title   string
	Percent int
	Muted   bool
}

// Placement says where the panel goes and for how long.
type Placement struct {
	// Near is a point on the monitor to show the panel on, usually the cursor position of the input that changed the level.
	Near win32.Point
	// Position is one of the config.Position* values.
	Position string
	Duration time.Duration
}

// OSD is the panel, owned by a thread of its own.
type OSD struct {
	threadID uint32
	hwnd     uintptr

	mu      sync.Mutex
	pending pendingShow

	// Touched only on the OSD thread.
	shown pendingShow
	theme theme
	scale float64
	alpha int
	fonts fonts
}

type pendingShow struct {
	content   Content
	placement Placement
}

const (
	msgShow    = win32.WM_APP + 1
	timerHold  = 1
	timerFade  = 2
	fadeFrames = 12
	fadeFrame  = 16 * time.Millisecond

	// Sizes at 96 DPI.
	panelWidth  = 300
	panelHeight = 76
	panelMargin = 16
)

// New creates the hidden panel.
func New() (*OSD, error) {
	o := &OSD{}
	id, err := win32.StartThread(o.createWindow)
	if err != nil {
		return nil, err
	}
	o.threadID = id
	return o, nil
}

// Show displays content, replacing what is shown, and restarts the timer after which the panel fades out. It returns at once; the panel updates on its own thread.
func (o *OSD) Show(c Content, p Placement) {
	o.mu.Lock()
	o.pending = pendingShow{content: c, placement: p}
	o.mu.Unlock()
	win32.ProcPostMessageW.Call(o.hwnd, msgShow, 0, 0)
}

// Close destroys the panel and ends its thread.
func (o *OSD) Close() {
	win32.QuitThread(o.threadID)
}

func (o *OSD) createWindow() error {
	className, _ := windows.UTF16PtrFromString("VolmOSD")
	instance, _, _ := win32.ProcGetModuleHandleW.Call(0)
	wc := win32.WndClassEx{
		WndProc:   windows.NewCallback(o.wndProc),
		Instance:  windows.Handle(instance),
		ClassName: className,
	}
	wc.Size = uint32(unsafe.Sizeof(wc))
	if r, _, err := win32.ProcRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		return err
	}
	exStyle := uintptr(win32.WS_EX_LAYERED | win32.WS_EX_TRANSPARENT | win32.WS_EX_TOPMOST | win32.WS_EX_TOOLWINDOW | win32.WS_EX_NOACTIVATE)
	hwnd, _, err := win32.ProcCreateWindowExW.Call(exStyle, uintptr(unsafe.Pointer(className)), 0, win32.WS_POPUP,
		0, 0, panelWidth, panelHeight, 0, 0, instance, 0)
	if hwnd == 0 {
		return err
	}
	o.hwnd = hwnd
	corner := uint32(win32.DWMWCP_ROUND)
	win32.ProcDwmSetWindowAttribute.Call(hwnd, win32.DWMWA_WINDOW_CORNER_PREFERENCE, uintptr(unsafe.Pointer(&corner)), unsafe.Sizeof(corner))
	return nil
}

func (o *OSD) wndProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	switch msg {
	case msgShow:
		o.mu.Lock()
		o.shown = o.pending
		o.mu.Unlock()
		o.present()
		return 0
	case win32.WM_TIMER:
		o.onTimer(wParam)
		return 0
	case win32.WM_PAINT:
		o.paint()
		return 0
	case win32.WM_ERASEBKGND:
		return 1
	case win32.WM_DESTROY:
		o.fonts.release()
		return 0
	}
	r, _, _ := win32.ProcDefWindowProcW.Call(hwnd, msg, wParam, lParam)
	return r
}

// present places the panel on the monitor of the placement, at full opacity, and starts the hold timer.
func (o *OSD) present() {
	info, dpi := win32.MonitorAt(o.shown.placement.Near)
	o.scale = float64(dpi) / 96
	o.theme = currentTheme()
	o.fonts.ensure(o.scale)

	w, h := o.px(panelWidth), o.px(panelHeight)
	x, y := placePanel(info.Work, w, h, o.px(panelMargin), o.shown.placement.Position)
	win32.ProcKillTimer.Call(o.hwnd, timerFade)
	o.alpha = 255
	win32.ProcSetLayeredWindowAttributes.Call(o.hwnd, 0, uintptr(o.alpha), win32.LWA_ALPHA)
	win32.ProcSetWindowPos.Call(o.hwnd, win32.HWND_TOPMOST, uintptr(x), uintptr(y), uintptr(w), uintptr(h), win32.SWP_NOACTIVATE|win32.SWP_SHOWWINDOW)
	win32.ProcInvalidateRect.Call(o.hwnd, 0, 0)
	win32.ProcSetTimer.Call(o.hwnd, timerHold, uintptr(o.shown.placement.Duration.Milliseconds()), 0)
}

func (o *OSD) onTimer(id uintptr) {
	switch id {
	case timerHold:
		win32.ProcKillTimer.Call(o.hwnd, timerHold)
		win32.ProcSetTimer.Call(o.hwnd, timerFade, uintptr(fadeFrame.Milliseconds()), 0)
	case timerFade:
		o.alpha -= 255 / fadeFrames
		if o.alpha <= 0 {
			win32.ProcKillTimer.Call(o.hwnd, timerFade)
			win32.ProcShowWindow.Call(o.hwnd, win32.SW_HIDE)
			return
		}
		win32.ProcSetLayeredWindowAttributes.Call(o.hwnd, 0, uintptr(o.alpha), win32.LWA_ALPHA)
	}
}

func (o *OSD) px(v int) int32 { return int32(float64(v)*o.scale + 0.5) }

// placePanel returns the top-left corner of a w×h panel inside the work area, margin away from its edges.
func placePanel(work win32.Rect, w, h, margin int32, position string) (x, y int32) {
	switch position {
	case "bottomLeft":
		return work.Left + margin, work.Bottom - h - margin
	case "topRight":
		return work.Right - w - margin, work.Top + margin
	case "topLeft":
		return work.Left + margin, work.Top + margin
	case "bottomCenter":
		return work.Left + (work.Width()-w)/2, work.Bottom - h - margin
	default:
		return work.Right - w - margin, work.Bottom - h - margin
	}
}

// theme holds COLORREF values (0x00BBGGRR).
type theme struct {
	background, text, track, fill, mutedFill uint32
}

var (
	darkTheme  = theme{background: rgb(0x2B, 0x2B, 0x2B), text: rgb(0xFF, 0xFF, 0xFF), track: rgb(0x4A, 0x4A, 0x4A), fill: rgb(0x4C, 0xC2, 0xFF), mutedFill: rgb(0x8A, 0x8A, 0x8A)}
	lightTheme = theme{background: rgb(0xF9, 0xF9, 0xF9), text: rgb(0x1A, 0x1A, 0x1A), track: rgb(0xD1, 0xD1, 0xD1), fill: rgb(0x00, 0x5F, 0xB8), mutedFill: rgb(0x8A, 0x8A, 0x8A)}
)

func rgb(r, g, b uint8) uint32 { return uint32(r) | uint32(g)<<8 | uint32(b)<<16 }

// currentTheme follows the app theme the user picked in Windows settings.
func currentTheme() theme {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`, registry.QUERY_VALUE)
	if err != nil {
		return lightTheme
	}
	defer k.Close()
	if light, _, err := k.GetIntegerValue("AppsUseLightTheme"); err == nil && light == 0 {
		return darkTheme
	}
	return lightTheme
}
