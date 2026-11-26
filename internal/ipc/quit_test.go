// Copyright (C) 2026 otoneko.
// SPDX-License-Identifier: GPL-3.0-only

package ipc

import (
	"fmt"
	"os"
	"testing"
	"time"
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
