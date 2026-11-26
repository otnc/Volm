// Copyright (C) 2026 otoneko.
// SPDX-License-Identifier: GPL-3.0-only

// Package ipc lets the settings process ask the resident process to quit, through a named event of the user's session.
package ipc

import (
	"errors"

	"golang.org/x/sys/windows"
)

// quitEventName is a variable so that tests can use a name of their own while a real Volm runs.
var quitEventName = `Local\VolmQuit`

// ListenQuit creates the quit event and calls onQuit, on a goroutine, each time another process signals it. stop ends the listening and closes the event.
//
//	stop, err := ipc.ListenQuit(func() { mygo.App.Quit() })
//	if err != nil {
//		return err
//	}
//	defer stop()
func ListenQuit(onQuit func()) (stop func(), err error) {
	name, err := windows.UTF16PtrFromString(quitEventName)
	if err != nil {
		return nil, err
	}
	quit, err := windows.CreateEvent(nil, 0, 0, name)
	if err != nil {
		return nil, err
	}
	stopEvent, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		_ = windows.CloseHandle(quit)
		return nil, err
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		handles := []windows.Handle{quit, stopEvent}
		for {
			r, err := windows.WaitForMultipleObjects(handles, false, windows.INFINITE)
			if err != nil || r != windows.WAIT_OBJECT_0 {
				return
			}
			onQuit()
		}
	}()
	return func() {
		_ = windows.SetEvent(stopEvent)
		<-done
		_ = windows.CloseHandle(quit)
		_ = windows.CloseHandle(stopEvent)
	}, nil
}

// RequestQuit signals the resident process to quit. It reports false, with a nil error, when no resident process is listening.
func RequestQuit() (bool, error) {
	name, err := windows.UTF16PtrFromString(quitEventName)
	if err != nil {
		return false, err
	}
	h, err := windows.OpenEvent(windows.EVENT_MODIFY_STATE, false, name)
	if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer func() { _ = windows.CloseHandle(h) }()
	return true, windows.SetEvent(h)
}
