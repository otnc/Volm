// PoC 7: WH_KEYBOARD_LL で Win+↑/↓ を捨てたとき、ウィンドウの最大化やスナップと、
// Win キーを離したときのスタートメニューが起きないかを確かめる。
//
//	go run . -selftest        メモ帳を開き、Win+↑ と Win+↓ を合成して確かめる
//	go run . -selftest -nohook   フックなしで同じ入力を送る (対照)
//	go run . -duration 60s    手で Win+↑/↓ を押して確かめる
package main

import (
	"flag"
	"fmt"
	"log"
	"os/exec"
	"runtime"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32                     = windows.NewLazySystemDLL("user32.dll")
	procSetWindowsHookExW      = user32.NewProc("SetWindowsHookExW")
	procUnhookWindowsHookEx    = user32.NewProc("UnhookWindowsHookEx")
	procCallNextHookEx         = user32.NewProc("CallNextHookEx")
	procGetMessageW            = user32.NewProc("GetMessageW")
	procPostThreadMessageW     = user32.NewProc("PostThreadMessageW")
	procGetAsyncKeyState       = user32.NewProc("GetAsyncKeyState")
	procSendInput              = user32.NewProc("SendInput")
	procGetForegroundWindow    = user32.NewProc("GetForegroundWindow")
	procGetClassNameW          = user32.NewProc("GetClassNameW")
	procGetWindowTextW         = user32.NewProc("GetWindowTextW")
	procGetWindowPlacement     = user32.NewProc("GetWindowPlacement")
	procSetDpiAwarenessContext = user32.NewProc("SetProcessDpiAwarenessContext")
)

const (
	whKeyboardLL         = 13
	wmQuit               = 0x0012
	wmKeyDown            = 0x0100
	wmSysKeyDown         = 0x0104
	vkLWin               = 0x5B
	vkRWin               = 0x5C
	vkUp                 = 0x26
	vkDown               = 0x28
	vkNoName             = 0xFC
	inputKeyboard        = 1
	keyeventfExtendedKey = 0x0001
	keyeventfKeyUp       = 0x0002
)

type kbdllHookStruct struct {
	VkCode    uint32
	ScanCode  uint32
	Flags     uint32
	Time      uint32
	ExtraInfo uintptr
}

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

type windowPlacement struct {
	Length, Flags, ShowCmd uint32
	MinPos, MaxPos         [2]int32
	Normal                 [4]int32
	Device                 [4]int32
}

var handled = make(chan uint32, 16)

func main() {
	duration := flag.Duration("duration", 60*time.Second, "フックを登録しておく時間")
	selftest := flag.Bool("selftest", false, "メモ帳でキー入力を合成して確かめる")
	noHook := flag.Bool("nohook", false, "フックを登録せずに -selftest を行う (テスト自体が最大化を検出できるかの対照)")
	flag.Parse()
	perMonitorAwareV2 := ^uintptr(3)
	procSetDpiAwarenessContext.Call(perMonitorAwareV2)

	if *noHook {
		runSelfTest()
		return
	}
	ready := make(chan uint32)
	done := make(chan struct{})
	go runHookThread(ready, done)
	threadID := <-ready
	go maskWinRelease()

	if *selftest {
		runSelfTest()
	} else {
		time.Sleep(*duration)
	}
	procPostThreadMessageW.Call(uintptr(threadID), wmQuit, 0, 0)
	<-done
}

func runHookThread(ready chan<- uint32, done chan<- struct{}) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer close(done)
	h, _, err := procSetWindowsHookExW.Call(whKeyboardLL, windows.NewCallback(keyboardHook), 0, 0)
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

// keyboardHook は Win を押している間の ↑/↓ を捨てる。押下だけでなく解放も捨てないと、
// 前面のアプリに対になっていない KEYUP が届く。
func keyboardHook(code int32, wParam uintptr, lParam uintptr) uintptr {
	if code >= 0 {
		info := (*kbdllHookStruct)(unsafe.Pointer(lParam))
		if (info.VkCode == vkUp || info.VkCode == vkDown) && winDown() {
			if wParam == wmKeyDown || wParam == wmSysKeyDown {
				select {
				case handled <- info.VkCode:
				default:
				}
			}
			return 1
		}
	}
	r, _, _ := procCallNextHookEx.Call(0, uintptr(code), wParam, lParam)
	return r
}

func winDown() bool {
	l, _, _ := procGetAsyncKeyState.Call(vkLWin)
	r, _, _ := procGetAsyncKeyState.Call(vkRWin)
	return (l|r)&0x8000 != 0
}

// maskWinRelease は、Win が押されている間に VK_NONAME の押下と解放を挟む。
// 挟まないと Windows は Win だけが押されて離されたと見なし、スタートメニューを開く。
func maskWinRelease() {
	for vk := range handled {
		name := map[uint32]string{vkUp: "Win+Up", vkDown: "Win+Down"}[vk]
		fmt.Println("handled", name)
		sendKeys(keybdInput{Vk: vkNoName}, keybdInput{Vk: vkNoName, Flags: keyeventfKeyUp})
	}
}

func sendKeys(keys ...keybdInput) {
	for i := range keys {
		keys[i].Type = inputKeyboard
	}
	procSendInput.Call(uintptr(len(keys)), uintptr(unsafe.Pointer(&keys[0])), unsafe.Sizeof(keys[0]))
}

func runSelfTest() {
	notepad := exec.Command("notepad.exe")
	if err := notepad.Start(); err != nil {
		log.Fatal(err)
	}
	time.Sleep(2 * time.Second)
	target := foreground()
	fmt.Printf("foreground before: %s\n", describe(target))
	before := placement(target)

	for _, arrow := range []uint16{vkUp, vkDown} {
		sendKeys(keybdInput{Vk: vkLWin, Flags: keyeventfExtendedKey})
		time.Sleep(50 * time.Millisecond)
		sendKeys(keybdInput{Vk: arrow, Flags: keyeventfExtendedKey})
		sendKeys(keybdInput{Vk: arrow, Flags: keyeventfExtendedKey | keyeventfKeyUp})
		time.Sleep(100 * time.Millisecond)
		sendKeys(keybdInput{Vk: vkLWin, Flags: keyeventfExtendedKey | keyeventfKeyUp})
		time.Sleep(700 * time.Millisecond)
		after := placement(target)
		fmt.Printf("after Win+%s: foreground=%s showCmd %d->%d rect %v->%v\n",
			map[uint16]string{vkUp: "Up", vkDown: "Down"}[arrow], describe(foreground()), before.ShowCmd, after.ShowCmd, before.Normal, after.Normal)
	}
	notepad.Process.Kill()
}

func foreground() uintptr {
	h, _, _ := procGetForegroundWindow.Call()
	return h
}

func describe(hwnd uintptr) string {
	var class, title [128]uint16
	procGetClassNameW.Call(hwnd, uintptr(unsafe.Pointer(&class[0])), 128)
	procGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&title[0])), 128)
	return fmt.Sprintf("%q %q", windows.UTF16ToString(class[:]), windows.UTF16ToString(title[:]))
}

func placement(hwnd uintptr) windowPlacement {
	p := windowPlacement{Length: uint32(unsafe.Sizeof(windowPlacement{}))}
	procGetWindowPlacement.Call(hwnd, uintptr(unsafe.Pointer(&p)))
	return p
}
