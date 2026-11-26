// PoC 4: ソフト減光の覆い (レイヤードウィンドウ) を外部モニターに重ね、
// 画面が暗くなるか、クリックとカーソル位置の判定が覆いを素通りするかを確かめる。
//
//	go run .                  覆いを 3 秒出して自動で確かめる
//	go run . -hold 30s        覆いを 30 秒出したままにする (目で見て確かめる)
package main

import (
	"flag"
	"fmt"
	"log"
	"runtime"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32                         = windows.NewLazySystemDLL("user32.dll")
	gdi32                          = windows.NewLazySystemDLL("gdi32.dll")
	procSetDpiAwarenessContext     = user32.NewProc("SetProcessDpiAwarenessContext")
	procEnumDisplayMonitors        = user32.NewProc("EnumDisplayMonitors")
	procGetMonitorInfoW            = user32.NewProc("GetMonitorInfoW")
	procRegisterClassExW           = user32.NewProc("RegisterClassExW")
	procCreateWindowExW            = user32.NewProc("CreateWindowExW")
	procDestroyWindow              = user32.NewProc("DestroyWindow")
	procDefWindowProcW             = user32.NewProc("DefWindowProcW")
	procShowWindow                 = user32.NewProc("ShowWindow")
	procSetLayeredWindowAttributes = user32.NewProc("SetLayeredWindowAttributes")
	procGetMessageW                = user32.NewProc("GetMessageW")
	procDispatchMessageW           = user32.NewProc("DispatchMessageW")
	procPostThreadMessageW         = user32.NewProc("PostThreadMessageW")
	procWindowFromPoint            = user32.NewProc("WindowFromPoint")
	procGetAncestor                = user32.NewProc("GetAncestor")
	procGetClassNameW              = user32.NewProc("GetClassNameW")
	procGetDC                      = user32.NewProc("GetDC")
	procReleaseDC                  = user32.NewProc("ReleaseDC")
	procGetStockObject             = gdi32.NewProc("GetStockObject")
	procCreateCompatibleDC         = gdi32.NewProc("CreateCompatibleDC")
	procCreateCompatibleBitmap     = gdi32.NewProc("CreateCompatibleBitmap")
	procSelectObject               = gdi32.NewProc("SelectObject")
	procBitBlt                     = gdi32.NewProc("BitBlt")
	procGetPixel                   = gdi32.NewProc("GetPixel")
	procDeleteObject               = gdi32.NewProc("DeleteObject")
	procDeleteDC                   = gdi32.NewProc("DeleteDC")
)

const (
	wsPopup            = 0x80000000
	wsExLayered        = 0x00080000
	wsExTransparent    = 0x00000020
	wsExTopmost        = 0x00000008
	wsExToolWindow     = 0x00000080
	wsExNoActivate     = 0x08000000
	swShowNoActivate   = 4
	lwaAlpha           = 0x2
	blackBrush         = 4
	wmQuit             = 0x0012
	gaRoot             = 2
	monitorInfoPrimary = 1
	srcCopy            = 0x00CC0020
	captureBlt         = 0x40000000
)

type point struct{ X, Y int32 }
type rect struct{ Left, Top, Right, Bottom int32 }

type monitorInfo struct {
	Size    uint32
	Monitor rect
	Work    rect
	Flags   uint32
}

type wndClassEx struct {
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

type monitor struct {
	bounds, work rect
	primary      bool
}

func main() {
	hold := flag.Duration("hold", 3*time.Second, "覆いを出しておく時間")
	alpha := flag.Int("alpha", 128, "覆いの不透明度 (0-255)。128 で明るさ約 50%")
	flag.Parse()
	perMonitorAwareV2 := ^uintptr(3)
	procSetDpiAwarenessContext.Call(perMonitorAwareV2)

	var targets []monitor
	for _, m := range enumMonitors() {
		if !m.primary {
			targets = append(targets, m)
		}
	}

	probes := probePoints(targets)
	before := samplePixels(probes)

	ready := make(chan uint32)
	done := make(chan struct{})
	go runOverlayThread(targets, byte(*alpha), ready, done)
	threadID := <-ready
	time.Sleep(500 * time.Millisecond)

	after := samplePixels(probes)
	for i, p := range probes {
		fmt.Printf("(%5d,%5d) root=%-24q pixel %06x -> %06x\n", p.X, p.Y, rootClassAt(p), before[i], after[i])
	}

	time.Sleep(*hold)
	procPostThreadMessageW.Call(uintptr(threadID), wmQuit, 0, 0)
	<-done
}

// probePoints は各モニターの中央と、作業領域の外 (タスクバーがある帯) の点を返す。
func probePoints(ms []monitor) []point {
	var pts []point
	for _, m := range ms {
		pts = append(pts, point{(m.bounds.Left + m.bounds.Right) / 2, (m.bounds.Top + m.bounds.Bottom) / 2})
		if m.work.Bottom < m.bounds.Bottom {
			pts = append(pts, point{(m.bounds.Left + m.bounds.Right) / 2, (m.work.Bottom + m.bounds.Bottom) / 2})
		}
	}
	return pts
}

func runOverlayThread(targets []monitor, alpha byte, ready chan<- uint32, done chan<- struct{}) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer close(done)

	className, _ := windows.UTF16PtrFromString("VolmPoCDimOverlay")
	brush, _, _ := procGetStockObject.Call(blackBrush)
	wc := wndClassEx{
		WndProc:    windows.NewCallback(defWindowProc),
		Background: windows.Handle(brush),
		ClassName:  className,
	}
	wc.Size = uint32(unsafe.Sizeof(wc))
	if r, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		log.Fatal("RegisterClassExW: ", err)
	}

	var windowsCreated []uintptr
	for _, m := range targets {
		exStyle := uintptr(wsExLayered | wsExTransparent | wsExTopmost | wsExToolWindow | wsExNoActivate)
		hwnd, _, err := procCreateWindowExW.Call(exStyle, uintptr(unsafe.Pointer(className)), 0, wsPopup,
			uintptr(m.bounds.Left), uintptr(m.bounds.Top),
			uintptr(m.bounds.Right-m.bounds.Left), uintptr(m.bounds.Bottom-m.bounds.Top),
			0, 0, 0, 0)
		if hwnd == 0 {
			log.Fatal("CreateWindowExW: ", err)
		}
		procSetLayeredWindowAttributes.Call(hwnd, 0, uintptr(alpha), lwaAlpha)
		procShowWindow.Call(hwnd, swShowNoActivate)
		windowsCreated = append(windowsCreated, hwnd)
	}
	defer func() {
		for _, h := range windowsCreated {
			procDestroyWindow.Call(h)
		}
	}()
	ready <- windows.GetCurrentThreadId()

	var msg [48]byte
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg[0])), 0, 0, 0)
		if r == 0 || int32(r) == -1 {
			return
		}
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg[0])))
	}
}

func defWindowProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	r, _, _ := procDefWindowProcW.Call(hwnd, msg, wParam, lParam)
	return r
}

func enumMonitors() []monitor {
	var ms []monitor
	cb := windows.NewCallback(func(h, _, _ uintptr, _ uintptr) uintptr {
		info := monitorInfo{Size: uint32(unsafe.Sizeof(monitorInfo{}))}
		procGetMonitorInfoW.Call(h, uintptr(unsafe.Pointer(&info)))
		ms = append(ms, monitor{bounds: info.Monitor, work: info.Work, primary: info.Flags&monitorInfoPrimary != 0})
		return 1
	})
	procEnumDisplayMonitors.Call(0, 0, cb, 0)
	return ms
}

func rootClassAt(pt point) string {
	packed := uintptr(uint32(pt.X)) | uintptr(uint32(pt.Y))<<32
	hwnd, _, _ := procWindowFromPoint.Call(packed)
	root, _, _ := procGetAncestor.Call(hwnd, gaRoot)
	var buf [64]uint16
	n, _, _ := procGetClassNameW.Call(root, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return windows.UTF16ToString(buf[:n])
}

// samplePixels は画面を 1 ピクセルずつ写し取る。CAPTUREBLT を付けないと
// レイヤードウィンドウが写らず、覆いの効果を測れない。
func samplePixels(pts []point) []uint32 {
	screen, _, _ := procGetDC.Call(0)
	defer procReleaseDC.Call(0, screen)
	mem, _, _ := procCreateCompatibleDC.Call(screen)
	defer procDeleteDC.Call(mem)
	bmp, _, _ := procCreateCompatibleBitmap.Call(screen, 1, 1)
	defer procDeleteObject.Call(bmp)
	procSelectObject.Call(mem, bmp)

	values := make([]uint32, len(pts))
	for i, p := range pts {
		procBitBlt.Call(mem, 0, 0, 1, 1, screen, uintptr(p.X), uintptr(p.Y), srcCopy|captureBlt)
		c, _, _ := procGetPixel.Call(mem, 0, 0)
		values[i] = uint32(c)
	}
	return values
}
