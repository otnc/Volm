// Copyright (C) 2026 otoneko.
// SPDX-License-Identifier: GPL-3.0-only

// Package taskbar finds the app of a taskbar button with UI Automation, for changing that app's volume.
package taskbar

import (
	"cmp"
	"errors"
	"runtime"
	"strings"
	"syscall"
	"unsafe"

	"github.com/go-ole/go-ole"
	"github.com/otnc/Volm/internal/appid"
	"github.com/otnc/Volm/internal/win32"
	"golang.org/x/sys/windows"
)

var (
	clsidCUIAutomation = ole.NewGUID("{ff48dba4-60ef-4201-aa87-54103eef594e}")
	iidIUIAutomation   = ole.NewGUID("{30cbe57d-d9d0-452a-ab13-7ac5ac4825ee}")
	iidIPropertyStore  = ole.NewGUID("{886d8eeb-8cf2-4446-8d02-cdba1dbdcf99}")
	pkeyAppUserModelID = propertyKey{FmtID: *ole.NewGUID("{9F4C2855-9F79-4B39-A8D0-E1D42DE1D5F3}"), PID: 5}

	user32                          = windows.NewLazySystemDLL("user32.dll")
	shell32                         = windows.NewLazySystemDLL("shell32.dll")
	ole32                           = windows.NewLazySystemDLL("ole32.dll")
	procEnumWindows                 = user32.NewProc("EnumWindows")
	procIsWindowVisible             = user32.NewProc("IsWindowVisible")
	procGetWindowThreadProcessID    = user32.NewProc("GetWindowThreadProcessId")
	procSHGetPropertyStoreForWindow = shell32.NewProc("SHGetPropertyStoreForWindow")
	procPropVariantClear            = ole32.NewProc("PropVariantClear")
)

// Slots in the vtables of IUIAutomation, IUIAutomationElement and IPropertyStore, counting the three of IUnknown.
const (
	uiaElementFromPoint    = 7
	uiaElementName         = 23
	uiaElementAutomationID = 29
	propStoreGetValue      = 5
	vtLPWSTR               = 31
)

// automationIDPrefix starts the AutomationId of an app's button on the Windows 11 taskbar, as in "Appid: Brave".
const automationIDPrefix = "Appid: "

type propertyKey struct {
	FmtID ole.GUID
	PID   uint32
}

type propVariant struct {
	VT  uint16
	_   [3]uint16
	Val uintptr
	_   uintptr
}

// Button is an app's button on the taskbar.
type Button struct {
	AUMID string
	// Name is the app's name for the OSD, without what the taskbar adds to it, such as the count of windows.
	Name string
}

// ErrClosed reports a call after Close.
var ErrClosed = errors.New("taskbar: finder closed")

// Finder owns a UI Automation client on a COM thread; its methods run there and wait for the result.
type Finder struct {
	calls      chan func()
	done       chan struct{}
	automation *ole.IUnknown // touched only on the COM thread
}

// NewFinder starts the COM thread.
func NewFinder() (*Finder, error) {
	f := &Finder{calls: make(chan func()), done: make(chan struct{})}
	ready := make(chan error, 1)
	go f.run(ready)
	if err := <-ready; err != nil {
		return nil, err
	}
	return f, nil
}

func (f *Finder) run(ready chan<- error) {
	runtime.LockOSThread()
	if err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED); err != nil {
		ready <- err
		return
	}
	defer ole.CoUninitialize()
	automation, err := ole.CreateInstance(clsidCUIAutomation, iidIUIAutomation)
	if err != nil {
		ready <- err
		return
	}
	f.automation = automation
	defer automation.Release()
	ready <- nil
	for {
		select {
		case fn := <-f.calls:
			fn()
		case <-f.done:
			return
		}
	}
}

func (f *Finder) do(fn func()) error {
	finished := make(chan struct{})
	select {
	case f.calls <- func() { fn(); close(finished) }:
		<-finished
		return nil
	case <-f.done:
		return ErrClosed
	}
}

// Close stops the COM thread.
func (f *Finder) Close() { close(f.done) }

// ButtonAt returns the app button under pt, in physical pixels, or false when pt is not on one, as over the Start button, the clock or an empty part of the taskbar. UI Automation asks Explorer across processes, which takes a few milliseconds: never call it from a hook callback.
func (f *Finder) ButtonAt(pt win32.Point) (b Button, ok bool, err error) {
	err = f.do(func() {
		var element *ole.IUnknown
		if hr := callVtbl(f.automation, uiaElementFromPoint, pt.Packed(), uintptr(unsafe.Pointer(&element))); hr != 0 || element == nil {
			return
		}
		defer element.Release()
		id := bstrProperty(element, uiaElementAutomationID)
		aumid, found := strings.CutPrefix(id, automationIDPrefix)
		if !found {
			return
		}
		b, ok = Button{AUMID: aumid, Name: AppName(bstrProperty(element, uiaElementName))}, true
	})
	return b, ok, err
}

// Target returns the app of a button: its AUMID and the executables of the processes whose visible windows carry it.
func (f *Finder) Target(b Button) (t appid.Target, err error) {
	err = f.do(func() { t = appid.NewTarget(b.AUMID, executablesFor(b.AUMID)) })
	return t, err
}

// WindowButton returns the app of a top-level window as a Button, for changing its volume from its title bar: the window's AUMID, else its packaged app's, else its executable's path, and the app's name from the window's title.
func (f *Finder) WindowButton(hwnd uintptr) (b Button, err error) {
	err = f.do(func() {
		exe, packaged := appid.ProcessIdentity(win32.WindowProcess(hwnd))
		b = Button{AUMID: cmp.Or(windowAUMID(hwnd), packaged, exe), Name: TitleAppName(win32.WindowTitle(hwnd))}
	})
	return b, err
}

// TitleAppName takes the app's name from a window title, which most apps end with " - " and their name, as in "YouTube - Brave".
func TitleAppName(title string) string {
	if _, app, ok := strings.CutLast(title, " - "); ok && app != "" {
		return app
	}
	return title
}

// AppName cuts what the taskbar adds to an app's name for screen readers: the count of windows after " - ", and the state at the end, as in "Brave - 1 個の実行中ウィンドウ 固定済み".
func AppName(buttonName string) string {
	name, _, _ := strings.Cut(buttonName, " - ")
	for _, suffix := range []string{" 固定済み", " Pinned", " pinned"} {
		name = strings.TrimSuffix(name, suffix)
	}
	return strings.TrimSpace(name)
}

// executablesFor collects the executables of the visible windows whose AUMID is aumid. A window without an AUMID of its own counts with its packaged app's AUMID, or its executable's path, as the taskbar does. Runs on the COM thread, which SHGetPropertyStoreForWindow needs.
func executablesFor(aumid string) map[string]bool {
	state := enumState{want: strings.ToLower(appid.ExpandKnownFolder(aumid)), exes: map[string]bool{}}
	procEnumWindows.Call(enumWindowsCallback, uintptr(unsafe.Pointer(&state)))
	return state.exes
}

type enumState struct {
	want string
	exes map[string]bool
}

// enumWindowsCallback is made once: windows.NewCallback slots are limited and never freed, and EnumWindows runs on every Alt+wheel.
var enumWindowsCallback = windows.NewCallback(func(hwnd, lParam uintptr) uintptr {
	state := (*enumState)(unsafe.Pointer(lParam))
	if visible, _, _ := procIsWindowVisible.Call(hwnd); visible == 0 {
		return 1
	}
	var pid uint32
	procGetWindowThreadProcessID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	exe, packaged := appid.ProcessIdentity(pid)
	id := cmp.Or(windowAUMID(hwnd), packaged, exe)
	if exe != "" && strings.EqualFold(id, state.want) {
		state.exes[strings.ToLower(exe)] = true
	}
	return 1
})

func windowAUMID(hwnd uintptr) string {
	var store *ole.IUnknown
	if hr, _, _ := procSHGetPropertyStoreForWindow.Call(hwnd, uintptr(unsafe.Pointer(iidIPropertyStore)), uintptr(unsafe.Pointer(&store))); hr != 0 || store == nil {
		return ""
	}
	defer store.Release()
	var pv propVariant
	if hr := callVtbl(store, propStoreGetValue, uintptr(unsafe.Pointer(&pkeyAppUserModelID)), uintptr(unsafe.Pointer(&pv))); hr != 0 {
		return ""
	}
	defer procPropVariantClear.Call(uintptr(unsafe.Pointer(&pv)))
	if pv.VT != vtLPWSTR || pv.Val == 0 {
		return ""
	}
	return windows.UTF16PtrToString((*uint16)(unsafe.Pointer(pv.Val)))
}

func bstrProperty(element *ole.IUnknown, slot int) string {
	var bstr *uint16
	if hr := callVtbl(element, slot, uintptr(unsafe.Pointer(&bstr))); hr != 0 || bstr == nil {
		return ""
	}
	defer func() { _ = ole.SysFreeString((*int16)(unsafe.Pointer(bstr))) }()
	return windows.UTF16PtrToString(bstr)
}

func callVtbl(obj *ole.IUnknown, slot int, args ...uintptr) uintptr {
	vtbl := (*[64]uintptr)(unsafe.Pointer(obj.RawVTable))
	hr, _, _ := syscall.SyscallN(vtbl[slot], append([]uintptr{uintptr(unsafe.Pointer(obj))}, args...)...)
	return hr
}
