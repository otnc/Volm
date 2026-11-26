// PoC 3: モニターごとに明るさを変える方法 (WMI / DDC/CI) を調べ、
// HMONITOR と WMI のインスタンスを対応づけ、WMI で明るさを書き換える。
//
//	go run .            調べるだけ
//	go run . -write     WMI で明るさを段階的に下げて時間を測り、元に戻す
package main

import (
	"flag"
	"fmt"
	"log"
	"runtime"
	"strings"
	"time"
	"unsafe"

	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
	"golang.org/x/sys/windows"
)

var (
	user32                     = windows.NewLazySystemDLL("user32.dll")
	procEnumDisplayMonitors    = user32.NewProc("EnumDisplayMonitors")
	procGetMonitorInfoW        = user32.NewProc("GetMonitorInfoW")
	procEnumDisplayDevicesW    = user32.NewProc("EnumDisplayDevicesW")
	procSetDpiAwarenessContext = user32.NewProc("SetProcessDpiAwarenessContext")

	dxva2                                       = windows.NewLazySystemDLL("dxva2.dll")
	procGetNumberOfPhysicalMonitorsFromHMONITOR = dxva2.NewProc("GetNumberOfPhysicalMonitorsFromHMONITOR")
	procGetPhysicalMonitorsFromHMONITOR         = dxva2.NewProc("GetPhysicalMonitorsFromHMONITOR")
	procDestroyPhysicalMonitors                 = dxva2.NewProc("DestroyPhysicalMonitors")
	procGetMonitorBrightness                    = dxva2.NewProc("GetMonitorBrightness")
	procGetVCPFeatureAndVCPFeatureReply         = dxva2.NewProc("GetVCPFeatureAndVCPFeatureReply")
)

const (
	eddGetDeviceInterfaceName = 1
	vcpBrightness             = 0x10
)

type rect struct{ Left, Top, Right, Bottom int32 }

type monitorInfoEx struct {
	Size    uint32
	Monitor rect
	Work    rect
	Flags   uint32
	Device  [32]uint16
}

type displayDevice struct {
	Size         uint32
	DeviceName   [32]uint16
	DeviceString [128]uint16
	StateFlags   uint32
	DeviceID     [128]uint16
	DeviceKey    [128]uint16
}

type physicalMonitor struct {
	Handle      windows.Handle
	Description [128]uint16
}

type monitor struct {
	handle      uintptr
	bounds      rect
	gdiName     string
	name        string
	interfaceID string
}

func main() {
	write := flag.Bool("write", false, "WMI で明るさを書き換えて時間を測る")
	flag.Parse()
	perMonitorAwareV2 := ^uintptr(3)
	procSetDpiAwarenessContext.Call(perMonitorAwareV2)

	monitors := enumMonitors()
	for _, m := range monitors {
		fmt.Printf("== %s %q bounds=%+v\n   interface=%s\n   wmi key=%s\n", m.gdiName, m.name, m.bounds, m.interfaceID, wmiKey(m.interfaceID))
		probeDDC(m.handle)
	}

	runtime.LockOSThread()
	if err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED); err != nil {
		log.Fatal(err)
	}
	defer ole.CoUninitialize()
	wmi := connectWMI()
	defer wmi.Release()

	current := readWMIBrightness(wmi)
	for instance, value := range current {
		owner := "(対応する HMONITOR なし)"
		for _, m := range monitors {
			if matchesWMIInstance(m.interfaceID, instance) {
				owner = m.gdiName
			}
		}
		fmt.Printf("WMI %s = %d -> %s\n", instance, value, owner)
	}

	if *write {
		for instance, original := range current {
			measureWMIWrites(wmi, instance, original)
		}
	}
}

func enumMonitors() []monitor {
	var monitors []monitor
	cb := windows.NewCallback(func(h, _, _ uintptr, _ uintptr) uintptr {
		info := monitorInfoEx{Size: uint32(unsafe.Sizeof(monitorInfoEx{}))}
		procGetMonitorInfoW.Call(h, uintptr(unsafe.Pointer(&info)))
		m := monitor{handle: h, bounds: info.Monitor, gdiName: windows.UTF16ToString(info.Device[:])}
		dev := displayDevice{Size: uint32(unsafe.Sizeof(displayDevice{}))}
		r, _, _ := procEnumDisplayDevicesW.Call(uintptr(unsafe.Pointer(&info.Device[0])), 0, uintptr(unsafe.Pointer(&dev)), eddGetDeviceInterfaceName)
		if r != 0 {
			m.name = windows.UTF16ToString(dev.DeviceString[:])
			m.interfaceID = windows.UTF16ToString(dev.DeviceID[:])
		}
		monitors = append(monitors, m)
		return 1
	})
	procEnumDisplayMonitors.Call(0, 0, cb, 0)
	return monitors
}

// wmiKey はモニターのデバイスインターフェース名
// (\\?\DISPLAY#SHP158C#4&1227b4ea&0&UID8388688#{e6f07b5f-...}) を、
// WMI の InstanceName から末尾の _0 を除いた形 (DISPLAY\SHP158C\4&...&UID8388688) に直す。
func wmiKey(interfaceID string) string {
	s := strings.TrimPrefix(interfaceID, `\\?\`)
	if i := strings.LastIndex(s, "#{"); i >= 0 {
		s = s[:i]
	}
	return strings.ToUpper(strings.ReplaceAll(s, "#", `\`))
}

func matchesWMIInstance(interfaceID, instance string) bool {
	if interfaceID == "" {
		return false
	}
	if i := strings.LastIndex(instance, "_"); i >= 0 {
		instance = instance[:i]
	}
	return wmiKey(interfaceID) == strings.ToUpper(instance)
}

func probeDDC(h uintptr) {
	var count uint32
	if r, _, err := procGetNumberOfPhysicalMonitorsFromHMONITOR.Call(h, uintptr(unsafe.Pointer(&count))); r == 0 {
		fmt.Println("   DDC: GetNumberOfPhysicalMonitorsFromHMONITOR failed:", err)
		return
	}
	physical := make([]physicalMonitor, count)
	if r, _, err := procGetPhysicalMonitorsFromHMONITOR.Call(h, uintptr(count), uintptr(unsafe.Pointer(&physical[0]))); r == 0 {
		fmt.Println("   DDC: GetPhysicalMonitorsFromHMONITOR failed:", err)
		return
	}
	defer procDestroyPhysicalMonitors.Call(uintptr(count), uintptr(unsafe.Pointer(&physical[0])))
	for _, p := range physical {
		var lo, cur, hi uint32
		start := time.Now()
		r, _, err := procGetMonitorBrightness.Call(uintptr(p.Handle), uintptr(unsafe.Pointer(&lo)), uintptr(unsafe.Pointer(&cur)), uintptr(unsafe.Pointer(&hi)))
		if r != 0 {
			fmt.Printf("   DDC %q: GetMonitorBrightness %d (%d-%d) in %v\n", windows.UTF16ToString(p.Description[:]), cur, lo, hi, time.Since(start))
			continue
		}
		fmt.Printf("   DDC %q: GetMonitorBrightness failed in %v: %v\n", windows.UTF16ToString(p.Description[:]), time.Since(start), err)
		var kind uint32
		start = time.Now()
		r, _, err = procGetVCPFeatureAndVCPFeatureReply.Call(uintptr(p.Handle), vcpBrightness, uintptr(unsafe.Pointer(&kind)), uintptr(unsafe.Pointer(&cur)), uintptr(unsafe.Pointer(&hi)))
		if r != 0 {
			fmt.Printf("   DDC VCP 0x10: %d / %d in %v\n", cur, hi, time.Since(start))
		} else {
			fmt.Printf("   DDC VCP 0x10 failed in %v: %v\n", time.Since(start), err)
		}
	}
}

func connectWMI() *ole.IDispatch {
	locator, err := oleutil.CreateObject("WbemScripting.SWbemLocator")
	if err != nil {
		log.Fatal(err)
	}
	defer locator.Release()
	disp, err := locator.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		log.Fatal(err)
	}
	defer disp.Release()
	svc, err := oleutil.CallMethod(disp, "ConnectServer", nil, `root\wmi`)
	if err != nil {
		log.Fatal("ConnectServer: ", err)
	}
	return svc.ToIDispatch()
}

func eachWMIObject(wmi *ole.IDispatch, query string, fn func(obj *ole.IDispatch)) {
	result, err := oleutil.CallMethod(wmi, "ExecQuery", query)
	if err != nil {
		log.Fatal("ExecQuery: ", err)
	}
	set := result.ToIDispatch()
	defer set.Release()
	err = oleutil.ForEach(set, func(v *ole.VARIANT) error {
		obj := v.ToIDispatch()
		defer obj.Release()
		fn(obj)
		return nil
	})
	if err != nil {
		log.Fatal("ForEach: ", err)
	}
}

func readWMIBrightness(wmi *ole.IDispatch) map[string]uint8 {
	values := map[string]uint8{}
	eachWMIObject(wmi, "SELECT InstanceName, CurrentBrightness FROM WmiMonitorBrightness", func(obj *ole.IDispatch) {
		name := oleutil.MustGetProperty(obj, "InstanceName").ToString()
		values[name] = uint8(oleutil.MustGetProperty(obj, "CurrentBrightness").Val)
	})
	return values
}

func setWMIBrightness(wmi *ole.IDispatch, instance string, value uint8) error {
	var callErr error
	query := fmt.Sprintf(`SELECT * FROM WmiMonitorBrightnessMethods WHERE InstanceName = '%s'`, strings.ReplaceAll(instance, `\`, `\\`))
	eachWMIObject(wmi, query, func(obj *ole.IDispatch) {
		// SWbemObject の IDispatch は VT_UI1 や VT_UI4 を型の不一致として拒むので、VT_I4 で渡す。
		_, callErr = oleutil.CallMethod(obj, "WmiSetBrightness", int32(0), int32(value))
	})
	return callErr
}

func measureWMIWrites(wmi *ole.IDispatch, instance string, original uint8) {
	fmt.Printf("writing %s (original %d)\n", instance, original)
	for step := 1; step <= 10; step++ {
		target := uint8(max(int(original)-step*5, 0))
		start := time.Now()
		if err := setWMIBrightness(wmi, instance, target); err != nil {
			fmt.Println("  set failed:", err)
			break
		}
		fmt.Printf("  set %3d in %v\n", target, time.Since(start))
	}
	readBack := readWMIBrightness(wmi)[instance]
	fmt.Printf("  read back %d\n", readBack)
	start := time.Now()
	if err := setWMIBrightness(wmi, instance, original); err != nil {
		fmt.Println("  restore failed:", err)
	}
	time.Sleep(500 * time.Millisecond)
	fmt.Printf("  restored to %d in %v, now %d\n", original, time.Since(start), readWMIBrightness(wmi)[instance])
}
