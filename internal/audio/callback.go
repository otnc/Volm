// Copyright (C) 2026 otoneko.
// SPDX-License-Identifier: GPL-3.0-only

package audio

import (
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"

	"github.com/go-ole/go-ole"
	"github.com/moutend/go-wca/pkg/wca"
)

// volumeCallback is a COM object implementing IAudioEndpointVolumeCallback, which go-wca lacks. Its first field must be the vtable pointer, as COM expects of every object.
type volumeCallback struct {
	vtbl     *volumeCallbackVtbl
	refs     atomic.Int32
	onNotify func()
}

type volumeCallbackVtbl struct {
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr
	OnNotify       uintptr
}

// sharedVolumeCallbackVtbl is built once: syscall.NewCallback slots are limited and never freed.
var sharedVolumeCallbackVtbl = sync.OnceValue(func() *volumeCallbackVtbl {
	return &volumeCallbackVtbl{
		QueryInterface: syscall.NewCallback(volumeCallbackQueryInterface),
		AddRef:         syscall.NewCallback(volumeCallbackAddRef),
		Release:        syscall.NewCallback(volumeCallbackRelease),
		OnNotify:       syscall.NewCallback(volumeCallbackOnNotify),
	}
})

// newVolumeCallback returns a callback that calls onNotify, on a Core Audio thread, after any change of volume or mute. The caller keeps the returned object referenced for as long as it is registered, since COM's reference count does not keep Go memory alive.
func newVolumeCallback(onNotify func()) *volumeCallback {
	cb := &volumeCallback{vtbl: sharedVolumeCallbackVtbl(), onNotify: onNotify}
	cb.refs.Store(1)
	return cb
}

func volumeCallbackQueryInterface(this uintptr, riid *ole.GUID, out *uintptr) uintptr {
	if ole.IsEqualGUID(riid, ole.IID_IUnknown) || ole.IsEqualGUID(riid, wca.IID_IAudioEndpointVolumeCallback) {
		volumeCallbackAddRef(this)
		*out = this
		return ole.S_OK
	}
	*out = 0
	return ole.E_NOINTERFACE
}

func volumeCallbackAddRef(this uintptr) uintptr {
	return uintptr((*volumeCallback)(unsafe.Pointer(this)).refs.Add(1))
}

func volumeCallbackRelease(this uintptr) uintptr {
	return uintptr((*volumeCallback)(unsafe.Pointer(this)).refs.Add(-1))
}

func volumeCallbackOnNotify(this uintptr, _ uintptr) uintptr {
	(*volumeCallback)(unsafe.Pointer(this)).onNotify()
	return ole.S_OK
}
