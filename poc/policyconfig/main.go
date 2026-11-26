// PoC 6: 非公開の IPolicyConfig で既定の再生デバイスを切り替えられるかを確かめる。
//
//	go run .          有効な再生デバイスを表示し、別のデバイスに切り替えてから元に戻す
package main

import (
	"fmt"
	"log"
	"runtime"
	"syscall"
	"time"
	"unsafe"

	"github.com/go-ole/go-ole"
	"github.com/moutend/go-wca/pkg/wca"
	"golang.org/x/sys/windows"
)

var (
	clsidPolicyConfigClient = ole.NewGUID("{870af99c-171d-4f9e-af0d-e63df40c2bc9}")
	iidPolicyConfig         = ole.NewGUID("{f8679f50-850a-41cf-9c72-430f290290c8}")
)

// IPolicyConfig の vtable の 14 番目 (IUnknown の 3 つを含めて 0 から数えて 13)。
// 公開されていないインターフェースなので、Windows のアップデートで位置が変わりうる。
const policyConfigSetDefaultEndpoint = 13

type device struct{ id, name string }

func main() {
	runtime.LockOSThread()
	if err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED); err != nil {
		log.Fatal(err)
	}
	defer ole.CoUninitialize()

	var enumerator *wca.IMMDeviceEnumerator
	if err := wca.CoCreateInstance(wca.CLSID_MMDeviceEnumerator, 0, wca.CLSCTX_ALL, wca.IID_IMMDeviceEnumerator, &enumerator); err != nil {
		log.Fatal(err)
	}
	defer enumerator.Release()

	devices := activeRenderDevices(enumerator)
	original := defaultDeviceID(enumerator)
	for _, d := range devices {
		mark := " "
		if d.id == original {
			mark = "*"
		}
		fmt.Printf("%s %s  %s\n", mark, d.name, d.id)
	}

	policy, err := ole.CreateInstance(clsidPolicyConfigClient, iidPolicyConfig)
	if err != nil {
		log.Fatal("CPolicyConfigClient: ", err)
	}
	defer policy.Release()

	target := original
	for _, d := range devices {
		if d.id != original {
			target = d.id
			break
		}
	}
	if target == original {
		fmt.Println("only one active device: setting it as default again")
	}

	start := time.Now()
	if err := setDefault(policy, target); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("switched in %v, default is now %s\n", time.Since(start), defaultDeviceID(enumerator))
	time.Sleep(2 * time.Second)
	if err := setDefault(policy, original); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("restored, default is now %s\n", defaultDeviceID(enumerator))
}

func setDefault(policy *ole.IUnknown, id string) error {
	idPtr, _ := windows.UTF16PtrFromString(id)
	vtbl := *(*[16]uintptr)(unsafe.Pointer(policy.RawVTable))
	for _, role := range []uintptr{wca.EConsole, wca.EMultimedia, wca.ECommunications} {
		hr, _, _ := syscall.SyscallN(vtbl[policyConfigSetDefaultEndpoint], uintptr(unsafe.Pointer(policy)), uintptr(unsafe.Pointer(idPtr)), role)
		if hr != 0 {
			return fmt.Errorf("SetDefaultEndpoint(role %d): 0x%08x", role, hr)
		}
	}
	return nil
}

func defaultDeviceID(enumerator *wca.IMMDeviceEnumerator) string {
	var d *wca.IMMDevice
	if err := enumerator.GetDefaultAudioEndpoint(wca.ERender, wca.EMultimedia, &d); err != nil {
		return ""
	}
	defer d.Release()
	var id string
	d.GetId(&id)
	return id
}

func activeRenderDevices(enumerator *wca.IMMDeviceEnumerator) []device {
	var collection *wca.IMMDeviceCollection
	if err := enumerator.EnumAudioEndpoints(wca.ERender, wca.DEVICE_STATE_ACTIVE, &collection); err != nil {
		log.Fatal(err)
	}
	defer collection.Release()
	var count uint32
	collection.GetCount(&count)
	var devices []device
	for i := uint32(0); i < count; i++ {
		var d *wca.IMMDevice
		if err := collection.Item(i, &d); err != nil {
			continue
		}
		var dev device
		d.GetId(&dev.id)
		var store *wca.IPropertyStore
		if d.OpenPropertyStore(wca.STGM_READ, &store) == nil {
			var pv wca.PROPVARIANT
			if store.GetValue(&wca.PKEY_Device_FriendlyName, &pv) == nil {
				dev.name = pv.String()
			}
			store.Release()
		}
		d.Release()
		devices = append(devices, dev)
	}
	return devices
}
