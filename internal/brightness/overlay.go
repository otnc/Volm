// Copyright (C) 2026 otoneko.
// SPDX-License-Identifier: GPL-3.0-only

package brightness

import (
	"math"
	"sync"
	"unsafe"

	"github.com/otnc/Volm/internal/win32"
	"golang.org/x/sys/windows"
)

// overlays owns the dimming windows, one per dimmed monitor, and a hidden window that hears display changes, all on one thread.
type overlays struct {
	threadID uint32
	listener uintptr

	mu      sync.Mutex
	pending map[string]dimRequest

	// Touched only on the overlay thread.
	windows         map[string]uintptr
	onDisplayChange func()
	className       *uint16
	instance        uintptr
}

type dimRequest struct {
	bounds  win32.Rect
	percent int
}

const msgApplyDim = win32.WM_APP + 2

// overlayAlpha is the opacity of the black overlay that leaves percent of the screen's light: 100 is no overlay at all.
func overlayAlpha(percent int) byte {
	percent = min(max(percent, 0), 100)
	return byte(math.Round(255 * float64(100-percent) / 100))
}

// startOverlays starts the overlay thread. onDisplayChange runs on that thread after monitors are added, removed or rearranged, and must return quickly.
func startOverlays(onDisplayChange func()) (*overlays, error) {
	o := &overlays{pending: map[string]dimRequest{}, windows: map[string]uintptr{}, onDisplayChange: onDisplayChange}
	id, err := win32.StartThread(o.setup)
	if err != nil {
		return nil, err
	}
	o.threadID = id
	return o, nil
}

func (o *overlays) setup() error {
	o.className, _ = windows.UTF16PtrFromString("VolmDimOverlay")
	o.instance, _, _ = win32.ProcGetModuleHandleW.Call(0)
	brush, _, _ := win32.ProcGetStockObject.Call(win32.BLACK_BRUSH)
	wc := win32.WndClassEx{
		WndProc:    windows.NewCallback(o.wndProc),
		Instance:   windows.Handle(o.instance),
		Background: windows.Handle(brush),
		ClassName:  o.className,
	}
	wc.Size = uint32(unsafe.Sizeof(wc))
	if r, _, err := win32.ProcRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		return err
	}
	// A hidden top-level window: message-only windows do not get WM_DISPLAYCHANGE.
	h, _, err := win32.ProcCreateWindowExW.Call(win32.WS_EX_TOOLWINDOW, uintptr(unsafe.Pointer(o.className)), 0, win32.WS_POPUP, 0, 0, 0, 0, 0, 0, o.instance, 0)
	if h == 0 {
		return err
	}
	o.listener = h
	return nil
}

// setDim leaves percent of the light of the monitor with the given ID and bounds. It returns at once.
func (o *overlays) setDim(id string, bounds win32.Rect, percent int) {
	o.mu.Lock()
	o.pending[id] = dimRequest{bounds: bounds, percent: percent}
	o.mu.Unlock()
	win32.ProcPostMessageW.Call(o.listener, msgApplyDim, 0, 0)
}

// clearAll removes every overlay, as when monitors change and the overlays may no longer cover them.
func (o *overlays) clearAll() {
	o.mu.Lock()
	for id := range o.pending {
		o.pending[id] = dimRequest{percent: 100}
	}
	o.mu.Unlock()
	win32.ProcPostMessageW.Call(o.listener, msgApplyDim, 1, 0)
}

func (o *overlays) close() { win32.QuitThread(o.threadID) }

func (o *overlays) wndProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	switch msg {
	case msgApplyDim:
		o.apply(wParam == 1)
		return 0
	case win32.WM_DISPLAYCHANGE:
		if hwnd == o.listener && o.onDisplayChange != nil {
			o.onDisplayChange()
		}
	}
	r, _, _ := win32.ProcDefWindowProcW.Call(hwnd, msg, wParam, lParam)
	return r
}

func (o *overlays) apply(clearAll bool) {
	o.mu.Lock()
	requests := o.pending
	o.pending = map[string]dimRequest{}
	o.mu.Unlock()
	if clearAll {
		for id, h := range o.windows {
			win32.ProcDestroyWindow.Call(h)
			delete(o.windows, id)
		}
	}
	for id, req := range requests {
		h := o.windows[id]
		if req.percent >= 100 {
			if h != 0 {
				win32.ProcDestroyWindow.Call(h)
				delete(o.windows, id)
			}
			continue
		}
		if h == 0 {
			// Clicks go through (WS_EX_TRANSPARENT), and WindowFromPoint looks past it, so the taskbar under the overlay still takes the wheel.
			exStyle := uintptr(win32.WS_EX_LAYERED | win32.WS_EX_TRANSPARENT | win32.WS_EX_TOPMOST | win32.WS_EX_TOOLWINDOW | win32.WS_EX_NOACTIVATE)
			h, _, _ = win32.ProcCreateWindowExW.Call(exStyle, uintptr(unsafe.Pointer(o.className)), 0, win32.WS_POPUP, 0, 0, 0, 0, 0, 0, o.instance, 0)
			if h == 0 {
				continue
			}
			o.windows[id] = h
		}
		b := req.bounds
		win32.ProcSetLayeredWindowAttributes.Call(h, 0, uintptr(overlayAlpha(req.percent)), win32.LWA_ALPHA)
		win32.ProcSetWindowPos.Call(h, win32.HWND_TOPMOST, uintptr(b.Left), uintptr(b.Top), uintptr(b.Width()), uintptr(b.Height()), win32.SWP_NOACTIVATE|win32.SWP_SHOWWINDOW)
	}
}
