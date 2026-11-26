// Copyright (C) 2026 otoneko.
// SPDX-License-Identifier: GPL-3.0-only

package audio

import (
	"errors"
	"fmt"
	"syscall"
	"unsafe"

	"github.com/go-ole/go-ole"
	"github.com/moutend/go-wca/pkg/wca"
	"golang.org/x/sys/windows"
)

// Device is an active playback device.
type Device struct {
	ID      string
	Name    string
	Default bool
}

var (
	clsidPolicyConfigClient = ole.NewGUID("{870af99c-171d-4f9e-af0d-e63df40c2bc9}")
	iidPolicyConfig         = ole.NewGUID("{f8679f50-850a-41cf-9c72-430f290290c8}")
)

// policyConfigSetDefaultEndpoint is the slot of SetDefaultEndpoint in the vtable of IPolicyConfig, which Windows does not document; an update of Windows could move it.
const policyConfigSetDefaultEndpoint = 13

// ErrSwitchUnsupported reports that Windows refused to change the default device through IPolicyConfig.
var ErrSwitchUnsupported = errors.New("audio: Windows refused to switch the default device")

// Devices lists the active playback devices, the default one marked.
func (e *Endpoint) Devices() (devices []Device, err error) {
	e.do(func() { devices, err = e.devices() })
	return devices, err
}

func (e *Endpoint) devices() ([]Device, error) {
	var collection *wca.IMMDeviceCollection
	if err := e.enumerator.EnumAudioEndpoints(wca.ERender, wca.DEVICE_STATE_ACTIVE, &collection); err != nil {
		return nil, err
	}
	defer collection.Release()
	var count uint32
	if err := collection.GetCount(&count); err != nil {
		return nil, err
	}
	defaultID := e.defaultID()
	devices := make([]Device, 0, count)
	for i := range count {
		var d *wca.IMMDevice
		if err := collection.Item(i, &d); err != nil {
			continue
		}
		var id string
		if err := d.GetId(&id); err == nil {
			devices = append(devices, Device{ID: id, Name: friendlyName(d), Default: id == defaultID})
		}
		d.Release()
	}
	return devices, nil
}

func (e *Endpoint) defaultID() string {
	var d *wca.IMMDevice
	if err := e.enumerator.GetDefaultAudioEndpoint(wca.ERender, wca.EMultimedia, &d); err != nil {
		return ""
	}
	defer d.Release()
	var id string
	_ = d.GetId(&id)
	return id
}

// SetDefault makes the device the default for every role (console, multimedia and communications), as the Windows sound settings do. Changes reports the new device once Windows has switched.
func (e *Endpoint) SetDefault(id string) (err error) {
	e.do(func() { err = setDefaultEndpoint(id) })
	return err
}

// NextDevice makes the device after the current default, in the order Devices lists them, the default, and returns it.
func (e *Endpoint) NextDevice() (next Device, err error) {
	e.do(func() {
		var devices []Device
		if devices, err = e.devices(); err != nil {
			return
		}
		if len(devices) == 0 {
			err = ErrNoDevice
			return
		}
		next = devices[0]
		for i, d := range devices {
			if d.Default {
				next = devices[(i+1)%len(devices)]
			}
		}
		err = setDefaultEndpoint(next.ID)
	})
	return next, err
}

func setDefaultEndpoint(id string) error {
	policy, err := ole.CreateInstance(clsidPolicyConfigClient, iidPolicyConfig)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrSwitchUnsupported, err)
	}
	defer policy.Release()
	idPtr, err := windows.UTF16PtrFromString(id)
	if err != nil {
		return err
	}
	vtbl := *(*[16]uintptr)(unsafe.Pointer(policy.RawVTable))
	for _, role := range []uintptr{wca.EConsole, wca.EMultimedia, wca.ECommunications} {
		if hr, _, _ := syscall.SyscallN(vtbl[policyConfigSetDefaultEndpoint], uintptr(unsafe.Pointer(policy)), uintptr(unsafe.Pointer(idPtr)), role); hr != 0 {
			return fmt.Errorf("%w: SetDefaultEndpoint (role %d): 0x%08x", ErrSwitchUnsupported, role, hr)
		}
	}
	return nil
}
