// PoC 2: WH_MOUSE_LL でタスクバー上のホイールと中クリックを判定し、
// 他のアプリに渡さずに捨てられるかを確かめる。
//
//	go run . -duration 60s     手で操作して確かめる
//	go run . -selftest         カーソルをタスクバーに移し、ホイールを合成して確かめる
package main

import (
	"flag"
	"fmt"
	"log"
	"runtime"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32                  = windows.NewLazySystemDLL("user32.dll")
	procSetWindowsHookExW   = user32.NewProc("SetWindowsHookExW")
	procUnhookWindowsHookEx = user32.NewProc("UnhookWindowsHookEx")
	procCallNextHookEx      = user32.NewProc("CallNextHookEx")
	procGetMessageW         = user32.NewProc("GetMessageW")
	procPostThreadMessageW  = user32.NewProc("PostThreadMessageW")
	procWindowFromPoint     = user32.NewProc("WindowFromPoint")
	procGetAncestor         = user32.NewProc("GetAncestor")
	procGetClassNameW       = user32.NewProc("GetClassNameW")
	procGetAsyncKeyState    = user32.NewProc("GetAsyncKeyState")
	procSendInput           = user32.NewProc("SendInput")
	procGetCursorPos        = user32.NewProc("GetCursorPos")
	procSetCursorPos        = user32.NewProc("SetCursorPos")
	procFindWindowW         = user32.NewProc("FindWindowW")
	procGetWindowRect       = user32.NewProc("GetWindowRect")
	procSetDpiAwareness     = user32.NewProc("SetProcessDpiAwarenessContext")
)

const (
	whMouseLL        = 14
	wmQuit           = 0x0012
	wmMouseWheel     = 0x020A
	wmMButtonDown    = 0x0207
	wmMButtonUp      = 0x0208
	gaRoot           = 2
	vkShift          = 0x10
	vkControl        = 0x11
	vkMenu           = 0x12
	llmhfInjected    = 0x01
	inputMouse       = 0
	inputKeyboard    = 1
	mouseeventfWheel = 0x0800
	keyeventfKeyUp   = 0x0002
	// VK_NONAME は何にも割り当てられていないので、送っても副作用がない。
	vkNoName = 0xFC
)

type point struct{ X, Y int32 }
type rect struct{ Left, Top, Right, Bottom int32 }

type msllHookStruct struct {
	Pt        point
	MouseData uint32
	Flags     uint32
	Time      uint32
	ExtraInfo uintptr
}

type mouseEvent struct {
	message   uintptr
	delta     int16
	pt        point
	class     string
	ctrl      bool
	shift     bool
	alt       bool
	injected  bool
	swallowed bool
	elapsed   time.Duration
}

var (
	events  = make(chan mouseEvent, 64)
	dropped atomic.Int64
	slowest atomic.Int64
)

func main() {
	duration := flag.Duration("duration", 60*time.Second, "フックを登録しておく時間")
	selftest := flag.Bool("selftest", false, "ホイールを合成して自動で確かめる")
	flag.Parse()

	// フックが受け取る座標は物理ピクセルなので、DPI 非対応のままだと
	// 拡大率が 100% 以外の画面で WindowFromPoint がずれた位置を見る。
	const perMonitorAwareV2 = ^uintptr(3)
	if r, _, err := procSetDpiAwareness.Call(perMonitorAwareV2); r == 0 {
		log.Fatal("SetProcessDpiAwarenessContext: ", err)
	}

	hookReady := make(chan uint32)
	hookDone := make(chan struct{})
	go runHookThread(hookReady, hookDone)
	threadID := <-hookReady

	go printEvents()

	if *selftest {
		runSelfTest()
		*duration = 2 * time.Second
	}
	time.Sleep(*duration)
	procPostThreadMessageW.Call(uintptr(threadID), wmQuit, 0, 0)
	<-hookDone
	time.Sleep(100 * time.Millisecond)
	fmt.Printf("slowest callback: %v, dropped events: %d\n", time.Duration(slowest.Load()), dropped.Load())
}

// runHookThread は低レベルフックを登録した OS スレッドでメッセージループを回す。
// フックは登録したスレッドがメッセージを処理している間しか呼ばれない。
func runHookThread(ready chan<- uint32, done chan<- struct{}) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer close(done)

	h, _, err := procSetWindowsHookExW.Call(whMouseLL, windows.NewCallback(mouseHook), 0, 0)
	if h == 0 {
		log.Fatal("SetWindowsHookExW: ", err)
	}
	defer procUnhookWindowsHookEx.Call(h)
	ready <- windows.GetCurrentThreadId()

	var msg [48]byte
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg[0])), 0, 0, 0)
		if r == 0 || int32(r) == -1 {
			return
		}
	}
}

// mouseHook は Windows が LowLevelHooksTimeout 以内に戻ることを求めるので、
// 判定だけを行い、表示はチャネルの先に任せる。
func mouseHook(code int32, wParam uintptr, lParam uintptr) uintptr {
	if code < 0 || (wParam != wmMouseWheel && wParam != wmMButtonDown && wParam != wmMButtonUp) {
		r, _, _ := procCallNextHookEx.Call(0, uintptr(code), wParam, lParam)
		return r
	}
	start := time.Now()
	info := (*msllHookStruct)(unsafe.Pointer(lParam))
	class := rootClassAt(info.Pt)
	onTaskbar := class == "Shell_TrayWnd" || class == "Shell_SecondaryTrayWnd"
	ev := mouseEvent{
		message:   wParam,
		delta:     int16(info.MouseData >> 16),
		pt:        info.Pt,
		class:     class,
		ctrl:      keyDown(vkControl),
		shift:     keyDown(vkShift),
		alt:       keyDown(vkMenu),
		injected:  info.Flags&llmhfInjected != 0,
		swallowed: onTaskbar,
	}
	ev.elapsed = time.Since(start)
	if int64(ev.elapsed) > slowest.Load() {
		slowest.Store(int64(ev.elapsed))
	}
	select {
	case events <- ev:
	default:
		dropped.Add(1)
	}
	if onTaskbar {
		return 1
	}
	r, _, _ := procCallNextHookEx.Call(0, uintptr(code), wParam, lParam)
	return r
}

func rootClassAt(pt point) string {
	packed := uintptr(uint32(pt.X)) | uintptr(uint32(pt.Y))<<32
	hwnd, _, _ := procWindowFromPoint.Call(packed)
	if hwnd == 0 {
		return ""
	}
	root, _, _ := procGetAncestor.Call(hwnd, gaRoot)
	var buf [64]uint16
	n, _, _ := procGetClassNameW.Call(root, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return windows.UTF16ToString(buf[:n])
}

func keyDown(vk uintptr) bool {
	r, _, _ := procGetAsyncKeyState.Call(vk)
	return r&0x8000 != 0
}

func printEvents() {
	names := map[uintptr]string{wmMouseWheel: "wheel", wmMButtonDown: "mdown", wmMButtonUp: "mup"}
	for ev := range events {
		mods := ""
		if ev.ctrl {
			mods += "Ctrl+"
		}
		if ev.shift {
			mods += "Shift+"
		}
		if ev.alt {
			mods += "Alt+"
		}
		fmt.Printf("%-6s %s delta=%4d at (%d,%d) root=%q swallowed=%v injected=%v callback=%v\n",
			names[ev.message], mods, ev.delta, ev.pt.X, ev.pt.Y, ev.class, ev.swallowed, ev.injected, ev.elapsed)
		if ev.swallowed && ev.alt && ev.message == wmMouseWheel {
			maskAltRelease()
		}
	}
}

// maskAltRelease は Alt の押下と解放の間に別のキー入力を挟む。
// ホイールを捨てたままだと、前面のアプリは Alt だけが押されたと見なし、
// 離した時点でメニューバーにフォーカスを移す。
func maskAltRelease() {
	type keybdInput struct {
		Type      uint32
		_         uint32
		Vk        uint16
		Scan      uint16
		Flags     uint32
		Time      uint32
		ExtraInfo uintptr
		_         [8]byte
	}
	inputs := [2]keybdInput{
		{Type: inputKeyboard, Vk: vkNoName},
		{Type: inputKeyboard, Vk: vkNoName, Flags: keyeventfKeyUp},
	}
	procSendInput.Call(2, uintptr(unsafe.Pointer(&inputs[0])), unsafe.Sizeof(inputs[0]))
}

// runSelfTest はカーソルをメインのタスクバーの中央付近に動かし、
// ホイールを上下に 1 ノッチずつ合成してから、カーソルを元の位置に戻す。
func runSelfTest() {
	trayClass, _ := windows.UTF16PtrFromString("Shell_TrayWnd")
	tray, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(trayClass)), 0)
	var r rect
	procGetWindowRect.Call(tray, uintptr(unsafe.Pointer(&r)))
	fmt.Printf("Shell_TrayWnd rect: %+v\n", r)

	var saved point
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&saved)))
	defer procSetCursorPos.Call(uintptr(saved.X), uintptr(saved.Y))

	target := point{X: r.Right - 300, Y: (r.Top + r.Bottom) / 2}
	procSetCursorPos.Call(uintptr(target.X), uintptr(target.Y))
	time.Sleep(100 * time.Millisecond)
	for _, delta := range []int32{120, -120, 40, 40, 40} {
		sendWheel(delta)
		time.Sleep(50 * time.Millisecond)
	}
	time.Sleep(200 * time.Millisecond)
}

func sendWheel(delta int32) {
	type mouseInput struct {
		Type      uint32
		_         uint32
		Dx, Dy    int32
		MouseData uint32
		Flags     uint32
		Time      uint32
		ExtraInfo uintptr
	}
	in := mouseInput{Type: inputMouse, MouseData: uint32(delta), Flags: mouseeventfWheel}
	procSendInput.Call(1, uintptr(unsafe.Pointer(&in)), unsafe.Sizeof(in))
}
