// Copyright (C) 2026 otoneko.
// SPDX-License-Identifier: GPL-3.0-only

// Package brightness finds the monitors, works out how each one's brightness can change (WMI, DDC/CI, or a dimming overlay), and changes it.
package brightness

import (
	"strings"
	"unsafe"

	"github.com/otnc/Volm/internal/win32"
	"golang.org/x/sys/windows"
)

// Monitor is one display as Windows enumerates it.
type Monitor struct {
	// ID is the monitor's device interface name, such as \\?\DISPLAY#SHP158C#4&1227b4ea&0&UID8388688#{e6f07b5f-...}. It stays the same across restarts, so settings are keyed on it.
	ID string
	// Name is what the user knows the monitor as, such as "LCB2005V".
	Name    string
	Handle  uintptr
	Bounds  win32.Rect
	Primary bool
}

type displayDevice struct {
	Size         uint32
	DeviceName   [32]uint16
	DeviceString [128]uint16
	StateFlags   uint32
	DeviceID     [128]uint16
	DeviceKey    [128]uint16
}

type monitorInfoEx struct {
	win32.MonitorInfo
	Device [32]uint16
}

const (
	eddGetDeviceInterfaceName = 1
	monitorInfoPrimary        = 1
)

var (
	enumCallback = windows.NewCallback(func(h, _, _ uintptr, list uintptr) uintptr {
		ms := (*[]Monitor)(unsafe.Pointer(list))
		*ms = append(*ms, describeMonitor(h))
		return 1
	})
)

// Enumerate lists the monitors attached now, with names from EDID where WMI has them. names may be nil.
func Enumerate(names map[string]string) []Monitor {
	var ms []Monitor
	win32.ProcEnumDisplayMonitors.Call(0, 0, enumCallback, uintptr(unsafe.Pointer(&ms)))
	for i := range ms {
		if n := names[WMIKey(ms[i].ID)]; n != "" {
			ms[i].Name = n
		}
	}
	return ms
}

func describeMonitor(h uintptr) Monitor {
	var info monitorInfoEx
	info.Size = uint32(unsafe.Sizeof(info))
	win32.ProcGetMonitorInfoW.Call(h, uintptr(unsafe.Pointer(&info)))
	m := Monitor{Handle: h, Bounds: info.Monitor, Primary: info.Flags&monitorInfoPrimary != 0}
	dev := displayDevice{Size: uint32(unsafe.Sizeof(displayDevice{}))}
	if r, _, _ := win32.ProcEnumDisplayDevicesW.Call(uintptr(unsafe.Pointer(&info.Device[0])), 0, uintptr(unsafe.Pointer(&dev)), eddGetDeviceInterfaceName); r != 0 {
		m.ID = windows.UTF16ToString(dev.DeviceID[:])
		m.Name = windows.UTF16ToString(dev.DeviceString[:])
	}
	if m.ID == "" {
		m.ID = windows.UTF16ToString(info.Device[:])
	}
	if m.Primary && (m.Name == "" || strings.Contains(m.Name, "Generic")) {
		m.Name = "メインのモニター"
	}
	return m
}

// WMIKey turns a device interface name (\\?\DISPLAY#SHP158C#4&1227b4ea&0&UID8388688#{e6f07b5f-...}) into the form WMI's InstanceName takes without its trailing "_0" (DISPLAY\SHP158C\4&1227B4EA&0&UID8388688), upper-cased, so that the two can be compared.
func WMIKey(interfaceID string) string {
	s := strings.TrimPrefix(interfaceID, `\\?\`)
	if before, _, ok := strings.CutLast(s, "#{"); ok {
		s = before
	}
	return strings.ToUpper(strings.ReplaceAll(s, "#", `\`))
}

// WMIInstanceKey turns a WMI InstanceName (DISPLAY\SHP158C\4&1227b4ea&0&UID8388688_0) into the form WMIKey returns.
func WMIInstanceKey(instance string) string {
	if before, _, ok := strings.CutLast(instance, "_"); ok {
		instance = before
	}
	return strings.ToUpper(instance)
}

// MonitorAt returns the monitor nearest to pt among ms, or false when ms is empty.
func MonitorAt(ms []Monitor, pt win32.Point) (Monitor, bool) {
	h, _, _ := win32.ProcMonitorFromPoint.Call(pt.Packed(), win32.MONITOR_DEFAULTTONEAREST)
	for _, m := range ms {
		if m.Handle == h {
			return m, true
		}
	}
	return Monitor{}, false
}
