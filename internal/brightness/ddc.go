// Copyright (C) 2026 otoneko.
// SPDX-License-Identifier: GPL-3.0-only

package brightness

import (
	"errors"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	dxva2                                       = windows.NewLazySystemDLL("dxva2.dll")
	procGetNumberOfPhysicalMonitorsFromHMONITOR = dxva2.NewProc("GetNumberOfPhysicalMonitorsFromHMONITOR")
	procGetPhysicalMonitorsFromHMONITOR         = dxva2.NewProc("GetPhysicalMonitorsFromHMONITOR")
	procDestroyPhysicalMonitors                 = dxva2.NewProc("DestroyPhysicalMonitors")
	procGetMonitorBrightness                    = dxva2.NewProc("GetMonitorBrightness")
	procSetMonitorBrightness                    = dxva2.NewProc("SetMonitorBrightness")
)

type physicalMonitor struct {
	Handle      windows.Handle
	Description [128]uint16
}

var errNoPhysicalMonitor = errors.New("brightness: no physical monitor behind this display")

// ddcRange is a monitor's brightness scale as DDC/CI reports it, which need not run from 0 to 100.
type ddcRange struct{ min, current, max uint32 }

func (r ddcRange) percent() int {
	if r.max <= r.min {
		return 100
	}
	return int((r.current - r.min) * 100 / (r.max - r.min))
}

// value returns the monitor's value for percent. A range never read, as when DDC/CI is chosen by hand for a monitor that did not answer, counts as 0 to 100.
func (r ddcRange) value(percent int) uint32 {
	if r.max <= r.min {
		return uint32(percent)
	}
	return r.min + uint32(percent)*(r.max-r.min)/100
}

// withPhysicalMonitor runs fn with the first physical monitor of an HMONITOR, which is the one DDC/CI reaches on an ordinary display.
func withPhysicalMonitor(hmonitor uintptr, fn func(h windows.Handle) error) error {
	var count uint32
	if r, _, err := procGetNumberOfPhysicalMonitorsFromHMONITOR.Call(hmonitor, uintptr(unsafe.Pointer(&count))); r == 0 {
		return err
	}
	if count == 0 {
		return errNoPhysicalMonitor
	}
	physical := make([]physicalMonitor, count)
	if r, _, err := procGetPhysicalMonitorsFromHMONITOR.Call(hmonitor, uintptr(count), uintptr(unsafe.Pointer(&physical[0]))); r == 0 {
		return err
	}
	defer procDestroyPhysicalMonitors.Call(uintptr(count), uintptr(unsafe.Pointer(&physical[0])))
	return fn(physical[0].Handle)
}

// ddcRead asks the monitor for its brightness. A monitor that does not answer takes about 250ms to fail.
func ddcRead(hmonitor uintptr) (ddcRange, error) {
	var r ddcRange
	err := withPhysicalMonitor(hmonitor, func(h windows.Handle) error {
		if ok, _, err := procGetMonitorBrightness.Call(uintptr(h), uintptr(unsafe.Pointer(&r.min)), uintptr(unsafe.Pointer(&r.current)), uintptr(unsafe.Pointer(&r.max))); ok == 0 {
			return err
		}
		return nil
	})
	return r, err
}

func ddcWrite(hmonitor uintptr, value uint32) error {
	return withPhysicalMonitor(hmonitor, func(h windows.Handle) error {
		if ok, _, err := procSetMonitorBrightness.Call(uintptr(h), uintptr(value)); ok == 0 {
			return err
		}
		return nil
	})
}
