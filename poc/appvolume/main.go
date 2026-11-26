// PoC 5: タスクバーのボタンから UI Automation で AUMID を取り、
// その AUMID のアプリの音声セッションを見つけられるかを確かめる。
//
//	go run .          すべてのタスクバーを左から走査し、ボタンごとに対応するセッションを表示する
//	go run . -set     見つかったセッションの音量を半分にしてから元に戻す
package main

import (
	"flag"
	"fmt"
	"log"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/go-ole/go-ole"
	"github.com/moutend/go-wca/pkg/wca"
	"golang.org/x/sys/windows"
)

var (
	user32                          = windows.NewLazySystemDLL("user32.dll")
	shell32                         = windows.NewLazySystemDLL("shell32.dll")
	kernel32                        = windows.NewLazySystemDLL("kernel32.dll")
	ole32                           = windows.NewLazySystemDLL("ole32.dll")
	procSetDpiAwarenessContext      = user32.NewProc("SetProcessDpiAwarenessContext")
	procFindWindowExW               = user32.NewProc("FindWindowExW")
	procGetWindowRect               = user32.NewProc("GetWindowRect")
	procEnumWindows                 = user32.NewProc("EnumWindows")
	procIsWindowVisible             = user32.NewProc("IsWindowVisible")
	procGetWindowThreadProcessId    = user32.NewProc("GetWindowThreadProcessId")
	procSHGetPropertyStoreForWindow = shell32.NewProc("SHGetPropertyStoreForWindow")
	procGetApplicationUserModelId   = kernel32.NewProc("GetApplicationUserModelId")
	procPropVariantClear            = ole32.NewProc("PropVariantClear")

	clsidCUIAutomation = ole.NewGUID("{ff48dba4-60ef-4201-aa87-54103eef594e}")
	iidIUIAutomation   = ole.NewGUID("{30cbe57d-d9d0-452a-ab13-7ac5ac4825ee}")
	iidIPropertyStore  = ole.NewGUID("{886d8eeb-8cf2-4446-8d02-cdba1dbdcf99}")
	pkeyAppUserModelID = propertyKey{FmtID: *ole.NewGUID("{9F4C2855-9F79-4B39-A8D0-E1D42DE1D5F3}"), PID: 5}
)

const (
	uiaElementFromPoint     = 7
	uiaElementAutomationID  = 29
	uiaElementName          = 23
	propStoreGetValue       = 5
	vtLPWSTR                = 31
	processQueryLimitedInfo = 0x1000
	aumidPrefix             = "Appid: "
)

type point struct{ X, Y int32 }
type rect struct{ Left, Top, Right, Bottom int32 }

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

type taskbarButton struct {
	aumid, name string
	at          point
	lookup      time.Duration
}

type session struct {
	control *wca.IAudioSessionControl2
	volume  *wca.ISimpleAudioVolume
	pid     uint32
	exe     string
	aumid   string
	name    string
}

func main() {
	set := flag.Bool("set", false, "見つかったセッションの音量を半分にしてから戻す")
	flag.Parse()
	perMonitorAwareV2 := ^uintptr(3)
	procSetDpiAwarenessContext.Call(perMonitorAwareV2)

	runtime.LockOSThread()
	if err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED); err != nil {
		log.Fatal(err)
	}
	defer ole.CoUninitialize()

	buttons := scanTaskbar()
	sessions := enumSessions()
	fmt.Printf("\n%d sessions on the default device:\n", len(sessions))
	for _, s := range sessions {
		var v float32
		s.volume.GetMasterVolume(&v)
		fmt.Printf("  pid=%-6d vol=%.2f aumid=%q exe=%s name=%q\n", s.pid, v, s.aumid, s.exe, s.name)
	}

	windowsByAUMID := windowAUMIDs()
	fmt.Println()
	for _, b := range buttons {
		matched := sessionsFor(b.aumid, windowsByAUMID[strings.ToLower(b.aumid)], sessions)
		fmt.Printf("%-40q uia=%-8v -> %d session(s)", b.aumid, b.lookup.Round(time.Microsecond*100), len(matched))
		for _, s := range matched {
			fmt.Printf(" [pid %d %s]", s.pid, shortExe(s.exe))
		}
		fmt.Println()
		if *set && len(matched) > 0 {
			halveAndRestore(matched)
		}
	}
}

// scanTaskbar はすべてのタスクバーの中央の高さを左から 20px ずつ ElementFromPoint で調べ、
// Volm の本番と同じ方法でアプリのボタンの AUMID を集める。
func scanTaskbar() []taskbarButton {
	unk, err := ole.CreateInstance(clsidCUIAutomation, iidIUIAutomation)
	if err != nil {
		log.Fatal("CUIAutomation: ", err)
	}
	defer unk.Release()

	seen := map[string]bool{}
	var buttons []taskbarButton
	for _, tray := range taskbars() {
		var r rect
		procGetWindowRect.Call(tray, uintptr(unsafe.Pointer(&r)))
		y := (r.Top + r.Bottom) / 2
		for x := r.Left + 10; x < r.Right; x += 20 {
			pt := point{x, y}
			start := time.Now()
			id, name := elementAt(unk, pt)
			elapsed := time.Since(start)
			if !strings.HasPrefix(id, aumidPrefix) {
				continue
			}
			aumid := strings.TrimPrefix(id, aumidPrefix)
			if seen[aumid] {
				continue
			}
			seen[aumid] = true
			buttons = append(buttons, taskbarButton{aumid: aumid, name: name, at: pt, lookup: elapsed})
		}
	}
	return buttons
}

func taskbars() []uintptr {
	var found []uintptr
	for _, class := range []string{"Shell_TrayWnd", "Shell_SecondaryTrayWnd"} {
		name, _ := windows.UTF16PtrFromString(class)
		var prev uintptr
		for {
			h, _, _ := procFindWindowExW.Call(0, prev, uintptr(unsafe.Pointer(name)), 0)
			if h == 0 {
				break
			}
			found = append(found, h)
			prev = h
		}
	}
	return found
}

func elementAt(automation *ole.IUnknown, pt point) (automationID, name string) {
	packed := uintptr(uint32(pt.X)) | uintptr(uint32(pt.Y))<<32
	var element *ole.IUnknown
	if hr := callVtbl(automation, uiaElementFromPoint, packed, uintptr(unsafe.Pointer(&element))); hr != 0 || element == nil {
		return "", ""
	}
	defer element.Release()
	return bstrProperty(element, uiaElementAutomationID), bstrProperty(element, uiaElementName)
}

func bstrProperty(element *ole.IUnknown, slot int) string {
	var bstr *uint16
	if hr := callVtbl(element, slot, uintptr(unsafe.Pointer(&bstr))); hr != 0 || bstr == nil {
		return ""
	}
	defer ole.SysFreeString((*int16)(unsafe.Pointer(bstr)))
	return windows.UTF16PtrToString(bstr)
}

func callVtbl(obj *ole.IUnknown, slot int, args ...uintptr) uintptr {
	vtbl := *(*[64]uintptr)(unsafe.Pointer(obj.RawVTable))
	hr, _, _ := syscall.SyscallN(vtbl[slot], append([]uintptr{uintptr(unsafe.Pointer(obj))}, args...)...)
	return hr
}

func enumSessions() []session {
	var enumerator *wca.IMMDeviceEnumerator
	if err := wca.CoCreateInstance(wca.CLSID_MMDeviceEnumerator, 0, wca.CLSCTX_ALL, wca.IID_IMMDeviceEnumerator, &enumerator); err != nil {
		log.Fatal(err)
	}
	var device *wca.IMMDevice
	if err := enumerator.GetDefaultAudioEndpoint(wca.ERender, wca.EMultimedia, &device); err != nil {
		log.Fatal(err)
	}
	var manager *wca.IAudioSessionManager2
	if err := device.Activate(wca.IID_IAudioSessionManager2, wca.CLSCTX_ALL, nil, &manager); err != nil {
		log.Fatal(err)
	}
	var list *wca.IAudioSessionEnumerator
	if err := manager.GetSessionEnumerator(&list); err != nil {
		log.Fatal(err)
	}
	var count int
	list.GetCount(&count)

	var sessions []session
	for i := 0; i < count; i++ {
		var control *wca.IAudioSessionControl
		if err := list.GetSession(i, &control); err != nil {
			continue
		}
		dispatch, err := control.QueryInterface(wca.IID_IAudioSessionControl2)
		if err != nil {
			continue
		}
		control2 := (*wca.IAudioSessionControl2)(unsafe.Pointer(dispatch))
		volumeUnk, err := control.QueryInterface(wca.IID_ISimpleAudioVolume)
		if err != nil {
			continue
		}
		s := session{control: control2, volume: (*wca.ISimpleAudioVolume)(unsafe.Pointer(volumeUnk))}
		control2.GetProcessId(&s.pid)
		control2.GetDisplayName(&s.name)
		if s.pid != 0 {
			s.exe, s.aumid = processIdentity(s.pid)
		}
		sessions = append(sessions, s)
	}
	return sessions
}

// processIdentity は実行ファイルのパスと、パッケージアプリなら AUMID を返す。
func processIdentity(pid uint32) (exe, aumid string) {
	h, err := windows.OpenProcess(processQueryLimitedInfo, false, pid)
	if err != nil {
		return "", ""
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, windows.MAX_LONG_PATH)
	size := uint32(len(buf))
	if windows.QueryFullProcessImageName(h, 0, &buf[0], &size) == nil {
		exe = windows.UTF16ToString(buf[:size])
	}
	size = uint32(len(buf))
	if r, _, _ := procGetApplicationUserModelId.Call(uintptr(h), uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&buf[0]))); r == 0 {
		aumid = windows.UTF16ToString(buf[:size])
	}
	return exe, aumid
}

// windowAUMIDs は表示中のトップレベルウィンドウを AUMID ごとにまとめ、プロセスの実行ファイルを集める。
// ウィンドウに明示的な AUMID がなければ、パッケージアプリの AUMID か実行ファイルのパスで代える。
// タスクバーも、明示的な AUMID がない Win32 アプリのボタンには実行ファイルのパスを AutomationId に出す。
func windowAUMIDs() map[string]map[string]bool {
	result := map[string]map[string]bool{}
	cb := windows.NewCallback(func(hwnd, _ uintptr) uintptr {
		if visible, _, _ := procIsWindowVisible.Call(hwnd); visible == 0 {
			return 1
		}
		var pid uint32
		procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
		exe, packaged := processIdentity(pid)
		aumid := windowAUMID(hwnd)
		if aumid == "" {
			aumid = packaged
		}
		if aumid == "" {
			aumid = exe
		}
		if aumid == "" || exe == "" {
			return 1
		}
		key := strings.ToLower(aumid)
		if result[key] == nil {
			result[key] = map[string]bool{}
		}
		result[key][strings.ToLower(exe)] = true
		return 1
	})
	procEnumWindows.Call(cb, 0)
	return result
}

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

// sessionsFor は、セッションのプロセスの AUMID が一致するか、
// その AUMID のウィンドウを持つプロセスと実行ファイルが同じセッションを返す。
// ブラウザーは音声を別の子プロセスで鳴らすので、プロセス ID ではなく実行ファイルで比べる。
func sessionsFor(aumid string, exes map[string]bool, sessions []session) []session {
	if exes == nil {
		exes = map[string]bool{}
	}
	if path := expandKnownFolder(aumid); strings.Contains(path, `\`) {
		exes[strings.ToLower(path)] = true
	}
	var matched []session
	for _, s := range sessions {
		if s.pid == 0 {
			continue
		}
		if strings.EqualFold(s.aumid, aumid) || exes[strings.ToLower(s.exe)] {
			matched = append(matched, s)
		}
	}
	sort.Slice(matched, func(i, j int) bool { return matched[i].pid < matched[j].pid })
	return matched
}

// expandKnownFolder は、明示的な AUMID がない Win32 アプリにタスクバーが付ける
// {既知フォルダーの GUID}\相対パス の形を、実行ファイルの絶対パスに戻す。
// 例: {6D809377-6AF0-444B-8957-A3773F02200E}\VideoLAN\VLClc.exe
func expandKnownFolder(aumid string) string {
	if !strings.HasPrefix(aumid, "{") {
		return aumid
	}
	end := strings.Index(aumid, "}")
	if end < 0 {
		return aumid
	}
	guid := ole.NewGUID(aumid[:end+1])
	if guid == nil {
		return aumid
	}
	folder, err := windows.KnownFolderPath((*windows.KNOWNFOLDERID)(unsafe.Pointer(guid)), 0)
	if err != nil {
		return aumid
	}
	return folder + aumid[end+1:]
}

func halveAndRestore(matched []session) {
	for _, s := range matched {
		var original, halved float32
		s.volume.GetMasterVolume(&original)
		s.volume.SetMasterVolume(original/2, nil)
		s.volume.GetMasterVolume(&halved)
		s.volume.SetMasterVolume(original, nil)
		fmt.Printf("    pid %d: %.2f -> %.2f -> restored\n", s.pid, original, halved)
	}
}

func shortExe(path string) string {
	if i := strings.LastIndex(path, `\`); i >= 0 {
		return path[i+1:]
	}
	return path
}
