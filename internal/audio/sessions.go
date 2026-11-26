// Copyright (C) 2026 otoneko.
// SPDX-License-Identifier: GPL-3.0-only

package audio

import (
	"math"
	"syscall"
	"unsafe"

	"github.com/go-ole/go-ole"
	"github.com/moutend/go-wca/pkg/wca"
	"github.com/otnc/Volm/internal/appid"
	"github.com/otnc/Volm/internal/control"
)

// AppState is the volume of an app's audio sessions on the default device.
type AppState struct {
	Percent int
	Muted   bool
	// Found is false when the app has no audio session, as an app that has not played sound since it started.
	Found bool
}

// StepApp moves the volume of every audio session of the target app by notches steps of step percent, starting from the first session's volume, so that the app's sessions end up at one level. Raising it while muted also unmutes when unmuteOnIncrease is set.
func (e *Endpoint) StepApp(target appid.Target, notches, step int, unmuteOnIncrease bool) (st AppState, err error) {
	e.do(func() {
		var sessions []*wca.ISimpleAudioVolume
		sessions, err = e.appSessions(target)
		if err != nil || len(sessions) == 0 {
			return
		}
		defer func() {
			for _, s := range sessions {
				s.Release()
			}
		}()
		var level float32
		if err = sessions[0].GetMasterVolume(&level); err != nil {
			return
		}
		muted, muteErr := getSessionMute(sessions[0])
		if muteErr != nil {
			err = muteErr
			return
		}
		percent := control.StepPercent(int(math.Round(float64(level)*100)), notches, step)
		unmute := notches > 0 && muted && unmuteOnIncrease
		for _, s := range sessions {
			if err = s.SetMasterVolume(float32(percent)/100, nil); err != nil {
				return
			}
			if unmute {
				if err = s.SetMute(false, nil); err != nil {
					return
				}
			}
		}
		st = AppState{Percent: percent, Muted: muted && !unmute, Found: true}
	})
	return st, err
}

// appSessions returns the volume controls of the target's sessions on the default device. The caller releases them.
func (e *Endpoint) appSessions(target appid.Target) ([]*wca.ISimpleAudioVolume, error) {
	var device *wca.IMMDevice
	if err := e.enumerator.GetDefaultAudioEndpoint(wca.ERender, wca.EMultimedia, &device); err != nil {
		return nil, ErrNoDevice
	}
	defer device.Release()
	var manager *wca.IAudioSessionManager2
	if err := device.Activate(wca.IID_IAudioSessionManager2, wca.CLSCTX_ALL, nil, &manager); err != nil {
		return nil, err
	}
	defer manager.Release()
	var list *wca.IAudioSessionEnumerator
	if err := manager.GetSessionEnumerator(&list); err != nil {
		return nil, err
	}
	defer list.Release()
	var count int
	if err := list.GetCount(&count); err != nil {
		return nil, err
	}
	var volumes []*wca.ISimpleAudioVolume
	for i := range count {
		var session *wca.IAudioSessionControl
		if err := list.GetSession(i, &session); err != nil {
			continue
		}
		if v := matchingVolume(session, target); v != nil {
			volumes = append(volumes, v)
		}
		session.Release()
	}
	return volumes, nil
}

func matchingVolume(session *wca.IAudioSessionControl, target appid.Target) *wca.ISimpleAudioVolume {
	unk, err := session.QueryInterface(wca.IID_IAudioSessionControl2)
	if err != nil {
		return nil
	}
	control2 := (*wca.IAudioSessionControl2)(unsafe.Pointer(unk))
	defer control2.Release()
	var pid uint32
	if control2.GetProcessId(&pid) != nil || pid == 0 {
		return nil // the system sounds session
	}
	exe, aumid := appid.ProcessIdentity(pid)
	if !target.Matches(exe, aumid) {
		return nil
	}
	v, err := session.QueryInterface(wca.IID_ISimpleAudioVolume)
	if err != nil {
		return nil
	}
	return (*wca.ISimpleAudioVolume)(unsafe.Pointer(v))
}

// getSessionMute replaces go-wca's ISimpleAudioVolume.GetMute, which passes a Go bool (1 byte) where Windows writes a BOOL (4 bytes).
func getSessionMute(v *wca.ISimpleAudioVolume) (bool, error) {
	var muted int32
	hr, _, _ := syscall.SyscallN(v.VTable().GetMute, uintptr(unsafe.Pointer(v)), uintptr(unsafe.Pointer(&muted)))
	if hr != 0 {
		return false, ole.NewError(hr)
	}
	return muted != 0, nil
}
