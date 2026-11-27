// Copyright (C) 2026 otoneko.
// SPDX-License-Identifier: GPL-3.0-only

package ipc

import (
	"fmt"
	"os"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestMain(m *testing.M) {
	quitEventName = fmt.Sprintf(`Local\VolmQuitTest%d`, os.Getpid())
	os.Exit(m.Run())
}

func TestRequestQuitReachesListener(t *testing.T) {
	quits := make(chan struct{}, 1)
	stop, err := ListenQuit(func() { quits <- struct{}{} })
	if err != nil {
		t.Fatal(err)
	}
	defer stop()

	listening, err := RequestQuit()
	if err != nil || !listening {
		t.Fatalf("RequestQuit = %v, %v; want true, nil", listening, err)
	}
	select {
	case <-quits:
	case <-time.After(2 * time.Second):
		t.Fatal("the listener was not called within 2s")
	}
}

func TestRequestQuitWithoutListener(t *testing.T) {
	listening, err := RequestQuit()
	if err != nil || listening {
		t.Fatalf("RequestQuit = %v, %v; want false, nil when nothing listens", listening, err)
	}
}

// TestListenWhileAPreviousProcessHoldsTheEvent covers a restart: the old resident process has said it stopped but still holds the event when the new one starts. The new one must still listen, and must not quit on a request that was meant for the old one.
func TestListenWhileAPreviousProcessHoldsTheEvent(t *testing.T) {
	name, err := windows.UTF16PtrFromString(quitEventName)
	if err != nil {
		t.Fatal(err)
	}
	previous, err := windows.CreateEvent(nil, 0, 0, name)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = windows.CloseHandle(previous) }()
	if err := windows.SetEvent(previous); err != nil {
		t.Fatal(err)
	}

	quits := make(chan struct{}, 2)
	stop, err := ListenQuit(func() { quits <- struct{}{} })
	if err != nil {
		t.Fatalf("ListenQuit: %v", err)
	}
	defer stop()
	select {
	case <-quits:
		t.Fatal("quit on a request made before listening")
	case <-time.After(300 * time.Millisecond):
	}

	if listening, err := RequestQuit(); err != nil || !listening {
		t.Fatalf("RequestQuit = %v, %v; want true, nil", listening, err)
	}
	select {
	case <-quits:
	case <-time.After(2 * time.Second):
		t.Fatal("the listener was not called within 2s")
	}
}
