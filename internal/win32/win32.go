// Copyright (C) 2026 otoneko.
// SPDX-License-Identifier: GPL-3.0-only

// Package win32 declares the user32 and gdi32 functions, constants and structures that Volm calls directly. x/sys/windows does not cover them.
package win32

import (
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

// The DLLs and their functions, named as in the Windows SDK with a Proc prefix.
var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	gdi32    = windows.NewLazySystemDLL("gdi32.dll")
	shcore   = windows.NewLazySystemDLL("shcore.dll")
	dwmapi   = windows.NewLazySystemDLL("dwmapi.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")

	ProcSetWindowsHookExW          = user32.NewProc("SetWindowsHookExW")
	ProcUnhookWindowsHookEx        = user32.NewProc("UnhookWindowsHookEx")
	ProcCallNextHookEx             = user32.NewProc("CallNextHookEx")
	ProcGetMessageW                = user32.NewProc("GetMessageW")
	ProcTranslateMessage           = user32.NewProc("TranslateMessage")
	ProcDispatchMessageW           = user32.NewProc("DispatchMessageW")
	ProcPostThreadMessageW         = user32.NewProc("PostThreadMessageW")
	ProcPostMessageW               = user32.NewProc("PostMessageW")
	ProcWindowFromPoint            = user32.NewProc("WindowFromPoint")
	ProcGetAncestor                = user32.NewProc("GetAncestor")
	ProcGetClassNameW              = user32.NewProc("GetClassNameW")
	ProcGetAsyncKeyState           = user32.NewProc("GetAsyncKeyState")
	ProcSendInput                  = user32.NewProc("SendInput")
	ProcRegisterClassExW           = user32.NewProc("RegisterClassExW")
	ProcCreateWindowExW            = user32.NewProc("CreateWindowExW")
	ProcDestroyWindow              = user32.NewProc("DestroyWindow")
	ProcDefWindowProcW             = user32.NewProc("DefWindowProcW")
	ProcShowWindow                 = user32.NewProc("ShowWindow")
	ProcSetWindowPos               = user32.NewProc("SetWindowPos")
	ProcSetLayeredWindowAttributes = user32.NewProc("SetLayeredWindowAttributes")
	ProcInvalidateRect             = user32.NewProc("InvalidateRect")
	ProcBeginPaint                 = user32.NewProc("BeginPaint")
	ProcEndPaint                   = user32.NewProc("EndPaint")
	ProcFillRect                   = user32.NewProc("FillRect")
	ProcDrawTextW                  = user32.NewProc("DrawTextW")
	ProcSetTimer                   = user32.NewProc("SetTimer")
	ProcKillTimer                  = user32.NewProc("KillTimer")
	ProcMonitorFromPoint           = user32.NewProc("MonitorFromPoint")
	ProcGetMonitorInfoW            = user32.NewProc("GetMonitorInfoW")
	ProcGetCursorPos               = user32.NewProc("GetCursorPos")
	ProcPostQuitMessage            = user32.NewProc("PostQuitMessage")
	ProcFindWindowW                = user32.NewProc("FindWindowW")
	ProcSetForegroundWindow        = user32.NewProc("SetForegroundWindow")
	ProcIsIconic                   = user32.NewProc("IsIconic")

	ProcCreateCompatibleDC     = gdi32.NewProc("CreateCompatibleDC")
	ProcCreateCompatibleBitmap = gdi32.NewProc("CreateCompatibleBitmap")
	ProcSelectObject           = gdi32.NewProc("SelectObject")
	ProcDeleteObject           = gdi32.NewProc("DeleteObject")
	ProcDeleteDC               = gdi32.NewProc("DeleteDC")
	ProcBitBlt                 = gdi32.NewProc("BitBlt")
	ProcCreateSolidBrush       = gdi32.NewProc("CreateSolidBrush")
	ProcCreateFontW            = gdi32.NewProc("CreateFontW")
	ProcSetBkMode              = gdi32.NewProc("SetBkMode")
	ProcSetTextColor           = gdi32.NewProc("SetTextColor")

	ProcGetDpiForMonitor = shcore.NewProc("GetDpiForMonitor")

	ProcDwmSetWindowAttribute = dwmapi.NewProc("DwmSetWindowAttribute")

	ProcGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")
)

// Constants of the Windows SDK, under their SDK names.
const (
	WH_KEYBOARD_LL = 13
	WH_MOUSE_LL    = 14

	WM_DESTROY     = 0x0002
	WM_PAINT       = 0x000F
	WM_ERASEBKGND  = 0x0014
	WM_QUIT        = 0x0012
	WM_TIMER       = 0x0113
	WM_MOUSEWHEEL  = 0x020A
	WM_MBUTTONDOWN = 0x0207
	WM_MBUTTONUP   = 0x0208
	WM_APP         = 0x8000

	GA_ROOT = 2

	VK_SHIFT   = 0x10
	VK_CONTROL = 0x11
	VK_MENU    = 0x12
	VK_LWIN    = 0x5B
	VK_RWIN    = 0x5C
	// VK_NONAME is assigned to nothing, so sending it has no effect of its own.
	VK_NONAME = 0xFC

	INPUT_KEYBOARD    = 1
	KEYEVENTF_KEYUP   = 0x0002
	LLMHF_INJECTED    = 0x01
	WS_POPUP          = 0x80000000
	WS_EX_LAYERED     = 0x00080000
	WS_EX_TRANSPARENT = 0x00000020
	WS_EX_TOPMOST     = 0x00000008
	WS_EX_TOOLWINDOW  = 0x00000080
	WS_EX_NOACTIVATE  = 0x08000000
	SW_HIDE           = 0
	SW_SHOWNOACTIVATE = 4
	SW_RESTORE        = 9
	LWA_ALPHA         = 0x2
	HWND_TOPMOST      = ^uintptr(0)
	SWP_NOACTIVATE    = 0x0010
	SWP_SHOWWINDOW    = 0x0040

	MONITOR_DEFAULTTONEAREST = 2
	MDT_EFFECTIVE_DPI        = 0

	DT_LEFT         = 0x0000
	DT_RIGHT        = 0x0002
	DT_VCENTER      = 0x0004
	DT_SINGLELINE   = 0x0020
	DT_NOPREFIX     = 0x0800
	DT_END_ELLIPSIS = 0x8000
	TRANSPARENT     = 1
	SRCCOPY         = 0x00CC0020

	DWMWA_WINDOW_CORNER_PREFERENCE = 33
	DWMWCP_ROUND                   = 2
)

// Point is POINT.
type Point struct{ X, Y int32 }

// Packed returns p as Windows passes a POINT by value on x64: in one register.
func (p Point) Packed() uintptr { return uintptr(uint32(p.X)) | uintptr(uint32(p.Y))<<32 }

// Rect is RECT.
type Rect struct{ Left, Top, Right, Bottom int32 }

// Width returns Right - Left.
func (r Rect) Width() int32 { return r.Right - r.Left }

// Height returns Bottom - Top.
func (r Rect) Height() int32 { return r.Bottom - r.Top }

// MonitorInfo is MONITORINFO.
type MonitorInfo struct {
	Size    uint32
	Monitor Rect
	Work    Rect
	Flags   uint32
}

// MSLLHookStruct is MSLLHOOKSTRUCT, what a WH_MOUSE_LL hook receives.
type MSLLHookStruct struct {
	Pt        Point
	MouseData uint32
	Flags     uint32
	Time      uint32
	ExtraInfo uintptr
}

// KeybdInput is INPUT holding a KEYBDINPUT.
type KeybdInput struct {
	Type      uint32
	_         uint32
	Vk        uint16
	Scan      uint16
	Flags     uint32
	Time      uint32
	ExtraInfo uintptr
	_         [8]byte // pads to the size of INPUT, whose union is as large as MOUSEINPUT
}

// WndClassEx is WNDCLASSEXW.
type WndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   windows.Handle
	Icon       windows.Handle
	Cursor     windows.Handle
	Background windows.Handle
	MenuName   *uint16
	ClassName  *uint16
	IconSm     windows.Handle
}

// PaintStruct is PAINTSTRUCT.
type PaintStruct struct {
	HDC       uintptr
	Erase     int32
	Paint     Rect
	Restore   int32
	IncUpdate int32
	Reserved  [32]byte
}

// Msg is large enough for MSG on x64; Volm never reads its fields.
type Msg [48]byte

// KeyDown reports whether the virtual key is held down now.
func KeyDown(vk uintptr) bool {
	r, _, _ := ProcGetAsyncKeyState.Call(vk)
	return r&0x8000 != 0
}

// WindowAt returns the window under pt, which is in physical pixels as low-level hooks report it.
func WindowAt(pt Point) uintptr {
	hwnd, _, _ := ProcWindowFromPoint.Call(pt.Packed())
	return hwnd
}

// RootClass returns the window class of hwnd's top-level window.
func RootClass(hwnd uintptr) string {
	if hwnd == 0 {
		return ""
	}
	root, _, _ := ProcGetAncestor.Call(hwnd, GA_ROOT)
	var buf [64]uint16
	n, _, _ := ProcGetClassNameW.Call(root, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return windows.UTF16ToString(buf[:n])
}

// SendMaskKey presses and releases VK_NONAME. Send it while Alt or Win is held, after consuming an input that used it: otherwise the foreground app sees a lone Alt and focuses its menu bar, or Windows opens the Start menu on the lone Win.
func SendMaskKey() {
	keys := [2]KeybdInput{
		{Type: INPUT_KEYBOARD, Vk: VK_NONAME},
		{Type: INPUT_KEYBOARD, Vk: VK_NONAME, Flags: KEYEVENTF_KEYUP},
	}
	ProcSendInput.Call(uintptr(len(keys)), uintptr(unsafe.Pointer(&keys[0])), unsafe.Sizeof(keys[0]))
}

// MonitorAt returns the monitor nearest to pt, its rectangles and its effective DPI.
func MonitorAt(pt Point) (info MonitorInfo, dpi uint32) {
	mon, _, _ := ProcMonitorFromPoint.Call(pt.Packed(), MONITOR_DEFAULTTONEAREST)
	info.Size = uint32(unsafe.Sizeof(info))
	ProcGetMonitorInfoW.Call(mon, uintptr(unsafe.Pointer(&info)))
	var dpiX, dpiY uint32
	if hr, _, _ := ProcGetDpiForMonitor.Call(mon, MDT_EFFECTIVE_DPI, uintptr(unsafe.Pointer(&dpiX)), uintptr(unsafe.Pointer(&dpiY))); hr != 0 {
		dpiX = 96
	}
	return info, dpiX
}

// CursorPos returns the cursor position in physical pixels.
func CursorPos() Point {
	var p Point
	ProcGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
	return p
}

// RunMessageLoop pumps messages on the calling thread until WM_QUIT. The caller must have locked the goroutine to its OS thread, which owns the hooks and windows the loop serves.
func RunMessageLoop() {
	var msg Msg
	for {
		r, _, _ := ProcGetMessageW.Call(uintptr(unsafe.Pointer(&msg[0])), 0, 0, 0)
		if r == 0 || int32(r) == -1 {
			return
		}
		ProcTranslateMessage.Call(uintptr(unsafe.Pointer(&msg[0])))
		ProcDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg[0])))
	}
}

// StartThread runs setup on a new goroutine locked to its OS thread, then that thread's message loop. It returns the thread ID once setup succeeded, so that the caller can post WM_QUIT to it later.
func StartThread(setup func() error) (threadID uint32, err error) {
	result := make(chan error, 1)
	ids := make(chan uint32, 1)
	go func() {
		runtime.LockOSThread()
		if err := setup(); err != nil {
			result <- err
			return
		}
		ids <- windows.GetCurrentThreadId()
		result <- nil
		RunMessageLoop()
	}()
	if err := <-result; err != nil {
		return 0, err
	}
	return <-ids, nil
}

// QuitThread posts WM_QUIT to a thread started by StartThread.
func QuitThread(threadID uint32) {
	ProcPostThreadMessageW.Call(uintptr(threadID), WM_QUIT, 0, 0)
}

// BringToFront restores and activates the top-level window with the given title. It reports whether such a window exists.
func BringToFront(title string) bool {
	t, err := windows.UTF16PtrFromString(title)
	if err != nil {
		return false
	}
	hwnd, _, _ := ProcFindWindowW.Call(0, uintptr(unsafe.Pointer(t)))
	if hwnd == 0 {
		return false
	}
	if iconic, _, _ := ProcIsIconic.Call(hwnd); iconic != 0 {
		ProcShowWindow.Call(hwnd, SW_RESTORE)
	}
	ProcSetForegroundWindow.Call(hwnd)
	return true
}
