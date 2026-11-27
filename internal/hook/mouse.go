// Copyright (C) 2026 otoneko.
// SPDX-License-Identifier: GPL-3.0-only

// Package hook watches the mouse with a low-level hook and reports the wheel and middle-button input that lands on a taskbar.
package hook

import (
	"time"
	"unsafe"

	"github.com/otnc/Volm/internal/win32"
	"golang.org/x/sys/windows"
)

// Kind says which input an Event is.
type Kind int

// The kinds of input the hook reports.
const (
	Wheel Kind = iota + 1
	MiddleClick
)

// Event is one wheel notch or middle-button press, as the hook saw it.
type Event struct {
	Kind Kind
	// Delta is the wheel delta: 120 per notch of a standard wheel, positive when rolled away from the user. Zero for MiddleClick.
	Delta int
	// Pos is the cursor position in physical pixels.
	Pos       win32.Point
	OnTaskbar bool
	// OnTitleBar is set when Pos is in the title bar band of an ordinary window (see win32.OnTitleBar), whose top-level window is Window.
	OnTitleBar       bool
	Window           uintptr
	Ctrl, Shift, Alt bool
	// Time is when the hook saw the event. The handler can fall behind while it changes the volume and draws the OSD, so gaps between events are measured with this rather than when they are handled.
	Time time.Time
}

// Filter decides, inside the hook callback, whether Volm takes the event. A taken event is sent to the channel and withheld from every other app. It must return quickly: Windows removes a hook that does not return within LowLevelHooksTimeout, without telling the app.
type Filter func(Event) bool

// Mouse is a running low-level mouse hook.
type Mouse struct {
	threadID uint32
}

// StartMouse installs the hook on a thread of its own. Events that filter takes are sent to out; when out is full they are dropped rather than blocking the callback.
//
//	events := make(chan hook.Event, 64)
//	m, err := hook.StartMouse(func(e hook.Event) bool { return e.OnTaskbar }, events)
//	if err != nil {
//		return err
//	}
//	defer m.Stop()
func StartMouse(filter Filter, out chan<- Event) (*Mouse, error) {
	h := &mouseHook{filter: filter, out: out}
	var handle uintptr
	id, err := win32.StartThread(func() error {
		// The callback must outlive the hook, and NewCallback slots are never freed.
		r, _, err := win32.ProcSetWindowsHookExW.Call(win32.WH_MOUSE_LL, windows.NewCallback(h.callback), 0, 0)
		if r == 0 {
			return err
		}
		handle = r
		return nil
	})
	if err != nil {
		return nil, err
	}
	h.handle = handle
	return &Mouse{threadID: id}, nil
}

// Stop removes the hook. Windows removes it anyway when the process exits.
func (m *Mouse) Stop() { win32.QuitThread(m.threadID) }

// mouseHook's fields other than handle are only touched by the hook thread.
type mouseHook struct {
	filter Filter
	out    chan<- Event
	handle uintptr

	taskbar            taskbarCache
	middleDownWasTaken bool
}

func (h *mouseHook) callback(code int32, wParam, lParam uintptr) uintptr {
	if code >= 0 && h.handleMessage(wParam, (*win32.MSLLHookStruct)(unsafe.Pointer(lParam))) {
		return 1
	}
	r, _, _ := win32.ProcCallNextHookEx.Call(0, uintptr(code), wParam, lParam)
	return r
}

// handleMessage reports whether to withhold the message from other apps.
func (h *mouseHook) handleMessage(message uintptr, info *win32.MSLLHookStruct) bool {
	var ev Event
	switch message {
	case win32.WM_MOUSEWHEEL:
		ev = Event{Kind: Wheel, Delta: int(int16(info.MouseData >> 16))}
	case win32.WM_MBUTTONDOWN:
		ev = Event{Kind: MiddleClick}
	case win32.WM_MBUTTONUP:
		// The release must follow the press: an app that got the press needs the release, and one that did not must not get a lone release.
		taken := h.middleDownWasTaken
		h.middleDownWasTaken = false
		return taken
	default:
		return false
	}
	ev.Pos = info.Pt
	ev.Time = time.Now()
	ev.OnTaskbar, ev.Window = h.taskbar.at(info.Pt)
	if !ev.OnTaskbar && ev.Window != 0 && !h.taskbar.isDesktop {
		ev.OnTitleBar = win32.OnTitleBar(ev.Window, info.Pt)
	}
	ev.Ctrl = win32.KeyDown(win32.VK_CONTROL)
	ev.Shift = win32.KeyDown(win32.VK_SHIFT)
	ev.Alt = win32.KeyDown(win32.VK_MENU)
	if !h.filter(ev) {
		return false
	}
	if ev.Kind == MiddleClick {
		h.middleDownWasTaken = true
	}
	select {
	case h.out <- ev:
	default:
	}
	return true
}

// taskbarCache remembers the answer for the last window under the cursor. Asking for the root window's class costs up to about 1ms, and a wheel sends many events in a row over the same window.
type taskbarCache struct {
	hwnd      uintptr
	root      uintptr
	isTaskbar bool
	// isDesktop is set over the desktop, a window as large as the screen whose top edge is not a title bar.
	isDesktop bool
}

// at reports whether pt is on a taskbar, and the top-level window under pt.
func (c *taskbarCache) at(pt win32.Point) (isTaskbar bool, root uintptr) {
	hwnd := win32.WindowAt(pt)
	if hwnd != 0 && hwnd == c.hwnd {
		return c.isTaskbar, c.root
	}
	c.hwnd = hwnd
	c.root = win32.Root(hwnd)
	class := win32.RootClass(hwnd)
	c.isTaskbar = IsTaskbarClass(class)
	c.isDesktop = class == "Progman" || class == "WorkerW"
	return c.isTaskbar, c.root
}

// IsTaskbarClass reports whether a top-level window class is a taskbar: Shell_TrayWnd on the main monitor, Shell_SecondaryTrayWnd on the others. The notification area and the clock belong to them.
func IsTaskbarClass(class string) bool {
	return class == "Shell_TrayWnd" || class == "Shell_SecondaryTrayWnd"
}
