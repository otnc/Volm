// Copyright (C) 2026 otoneko.
// SPDX-License-Identifier: GPL-3.0-only

// Package audio reads and changes the master volume and mute of the default playback device through Core Audio.
package audio

import (
	"errors"
	"math"
	"runtime"
	"syscall"
	"unsafe"

	"github.com/go-ole/go-ole"
	"github.com/moutend/go-wca/pkg/wca"
	"github.com/otnc/Volm/internal/control"
)

// State is the volume of the default playback device.
type State struct {
	Percent int
	Muted   bool
	// Device is the device's friendly name, such as "スピーカー (Realtek(R) Audio)".
	Device string
}

// ErrNoDevice reports that there is no active playback device.
var ErrNoDevice = errors.New("audio: no default playback device")

// Endpoint owns every Core Audio object on one COM thread; its methods run there and wait for the result.
type Endpoint struct {
	calls   chan func()
	changes chan State
	done    chan struct{}

	// Touched only on the COM thread.
	enumerator *wca.IMMDeviceEnumerator
	notifier   *wca.IMMNotificationClient
	volume     *wca.IAudioEndpointVolume
	callback   *volumeCallback
	device     string
}

// Open starts the COM thread and binds to the current default device. Changes reports every later change of volume, mute or default device, including those Volm makes itself.
//
//	ep, err := audio.Open()
//	if err != nil {
//		return err
//	}
//	defer ep.Close()
//	st, err := ep.Step(1, 2, true) // up one notch of 2%
func Open() (*Endpoint, error) {
	e := &Endpoint{
		calls:   make(chan func()),
		changes: make(chan State, 8),
		done:    make(chan struct{}),
	}
	ready := make(chan error, 1)
	go e.run(ready)
	if err := <-ready; err != nil {
		return nil, err
	}
	return e, nil
}

func (e *Endpoint) run(ready chan<- error) {
	runtime.LockOSThread()
	if err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED); err != nil {
		ready <- err
		return
	}
	defer ole.CoUninitialize()
	if err := wca.CoCreateInstance(wca.CLSID_MMDeviceEnumerator, 0, wca.CLSCTX_ALL, wca.IID_IMMDeviceEnumerator, &e.enumerator); err != nil {
		ready <- err
		return
	}
	defer e.enumerator.Release()

	e.notifier = wca.NewIMMNotificationClient(wca.IMMNotificationClientCallback{
		OnDefaultDeviceChanged: func(flow wca.EDataFlow, role wca.ERole, _ string) error {
			if flow == wca.ERender && role == wca.EMultimedia {
				// Core Audio calls this on its own thread, which must not block on ours, and the enumerator may not be re-entered from here.
				go e.do(e.rebind)
			}
			return nil
		},
	})
	if err := e.enumerator.RegisterEndpointNotificationCallback(e.notifier); err != nil {
		ready <- err
		return
	}
	defer func() { _ = e.enumerator.UnregisterEndpointNotificationCallback(e.notifier) }()

	e.rebind()
	defer e.unbind()
	ready <- nil

	for {
		select {
		case fn := <-e.calls:
			fn()
		case <-e.done:
			return
		}
	}
}

// do runs fn on the COM thread and waits for it.
func (e *Endpoint) do(fn func()) {
	finished := make(chan struct{})
	select {
	case e.calls <- func() { fn(); close(finished) }:
		<-finished
	case <-e.done:
	}
}

// rebind switches to the current default device. Without one, the endpoint stays unbound and every call returns ErrNoDevice until a device appears.
func (e *Endpoint) rebind() {
	e.unbind()
	var device *wca.IMMDevice
	if err := e.enumerator.GetDefaultAudioEndpoint(wca.ERender, wca.EMultimedia, &device); err != nil {
		e.publish()
		return
	}
	defer device.Release()
	if err := device.Activate(wca.IID_IAudioEndpointVolume, wca.CLSCTX_ALL, nil, &e.volume); err != nil {
		e.volume = nil
		e.publish()
		return
	}
	e.device = friendlyName(device)
	if e.callback == nil {
		e.callback = newVolumeCallback(func() { go e.do(e.publish) })
	}
	registerControlChangeNotify(e.volume, e.callback)
	e.publish()
}

func (e *Endpoint) unbind() {
	if e.volume == nil {
		return
	}
	unregisterControlChangeNotify(e.volume, e.callback)
	e.volume.Release()
	e.volume = nil
	e.device = ""
}

// publish sends the current state to Changes, dropping the oldest pending state when the reader is behind: only the latest matters.
func (e *Endpoint) publish() {
	st, err := e.current()
	if err != nil {
		st = State{}
	}
	for {
		select {
		case e.changes <- st:
			return
		default:
			select {
			case <-e.changes:
			default:
			}
		}
	}
}

func (e *Endpoint) current() (State, error) {
	if e.volume == nil {
		return State{}, ErrNoDevice
	}
	var level float32
	if err := e.volume.GetMasterVolumeLevelScalar(&level); err != nil {
		return State{}, err
	}
	muted, err := getMute(e.volume)
	if err != nil {
		return State{}, err
	}
	return State{Percent: int(math.Round(float64(level) * 100)), Muted: muted, Device: e.device}, nil
}

// Current returns the state of the default device.
func (e *Endpoint) Current() (st State, err error) {
	e.do(func() { st, err = e.current() })
	return st, err
}

// Step moves the volume by notches steps of step percent, within 0 to 100. Raising the volume while muted also unmutes when unmuteOnIncrease is set.
func (e *Endpoint) Step(notches, step int, unmuteOnIncrease bool) (st State, err error) {
	e.do(func() {
		if st, err = e.current(); err != nil {
			return
		}
		target := control.StepPercent(st.Percent, notches, step)
		if err = e.volume.SetMasterVolumeLevelScalar(float32(target)/100, nil); err != nil {
			return
		}
		if notches > 0 && st.Muted && unmuteOnIncrease {
			if err = e.volume.SetMute(false, nil); err != nil {
				return
			}
		}
		st, err = e.current()
	})
	return st, err
}

// ToggleMute flips the mute of the default device.
func (e *Endpoint) ToggleMute() (st State, err error) {
	e.do(func() {
		if st, err = e.current(); err != nil {
			return
		}
		if err = e.volume.SetMute(!st.Muted, nil); err != nil {
			return
		}
		st, err = e.current()
	})
	return st, err
}

// Changes delivers the latest state after each change. A reader that falls behind sees only the most recent one.
func (e *Endpoint) Changes() <-chan State { return e.changes }

// Close stops the COM thread and releases the device.
func (e *Endpoint) Close() { close(e.done) }

func friendlyName(device *wca.IMMDevice) string {
	var store *wca.IPropertyStore
	if err := device.OpenPropertyStore(wca.STGM_READ, &store); err != nil {
		return ""
	}
	defer store.Release()
	var pv wca.PROPVARIANT
	if err := store.GetValue(&wca.PKEY_Device_FriendlyName, &pv); err != nil {
		return ""
	}
	return pv.String()
}

// getMute replaces go-wca's GetMute, which passes a Go bool (1 byte) where Windows writes a BOOL (4 bytes) and so overwrites the 3 bytes after it.
func getMute(v *wca.IAudioEndpointVolume) (bool, error) {
	var muted int32
	hr, _, _ := syscall.SyscallN(v.VTable().GetMute, uintptr(unsafe.Pointer(v)), uintptr(unsafe.Pointer(&muted)))
	if hr != 0 {
		return false, ole.NewError(hr)
	}
	return muted != 0, nil
}

// go-wca's RegisterControlChangeNotify takes no callback, so these call the IAudioEndpointVolume vtable directly.
func registerControlChangeNotify(v *wca.IAudioEndpointVolume, cb *volumeCallback) {
	syscall.SyscallN(v.VTable().RegisterControlChangeNotify, uintptr(unsafe.Pointer(v)), uintptr(unsafe.Pointer(cb)))
}

func unregisterControlChangeNotify(v *wca.IAudioEndpointVolume, cb *volumeCallback) {
	syscall.SyscallN(v.VTable().UnregisterControlChangeNotify, uintptr(unsafe.Pointer(v)), uintptr(unsafe.Pointer(cb)))
}
