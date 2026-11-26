// Copyright (C) 2026 otoneko.
// SPDX-License-Identifier: GPL-3.0-only

package audio

import (
	"os"
	"testing"
	"time"
)

// These tests change the real system volume for a moment and restore it. They run only with VOLM_INTEGRATION=1 on a machine with a playback device.
func openForTest(t *testing.T) *Endpoint {
	t.Helper()
	if os.Getenv("VOLM_INTEGRATION") != "1" {
		t.Skip("set VOLM_INTEGRATION=1 to change the real system volume")
	}
	ep, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ep.Close)
	drain(ep)
	return ep
}

func TestStepChangesVolumeAndNotifies(t *testing.T) {
	ep := openForTest(t)
	before, err := ep.Current()
	if err != nil {
		t.Fatal(err)
	}
	notches := 1
	if before.Percent >= 98 {
		notches = -1
	}
	t.Cleanup(func() { ep.Step(-notches, 2, false) })

	after, err := ep.Step(notches, 2, false)
	if err != nil {
		t.Fatal(err)
	}
	if want := before.Percent + notches*2; after.Percent != want {
		t.Fatalf("percent = %d, want %d", after.Percent, want)
	}
	waitForChange(t, ep, func(st State) bool { return st.Percent == after.Percent })
}

func TestToggleMuteFlipsAndNotifies(t *testing.T) {
	ep := openForTest(t)
	before, err := ep.Current()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if st, _ := ep.Current(); st.Muted != before.Muted {
			ep.ToggleMute()
		}
	})

	toggled, err := ep.ToggleMute()
	if err != nil {
		t.Fatal(err)
	}
	if toggled.Muted == before.Muted {
		t.Fatalf("muted stayed %v", before.Muted)
	}
	waitForChange(t, ep, func(st State) bool { return st.Muted == toggled.Muted })
}

func TestStepUpUnmutesOnlyWhenAsked(t *testing.T) {
	ep := openForTest(t)
	before, err := ep.Current()
	if err != nil {
		t.Fatal(err)
	}
	if before.Percent >= 96 {
		t.Skip("volume too close to 100% to step up twice")
	}
	t.Cleanup(func() {
		ep.Step(-2, 2, false)
		if st, _ := ep.Current(); st.Muted != before.Muted {
			ep.ToggleMute()
		}
	})
	if !before.Muted {
		if _, err := ep.ToggleMute(); err != nil {
			t.Fatal(err)
		}
	}

	st, err := ep.Step(1, 2, false)
	if err != nil {
		t.Fatal(err)
	}
	if !st.Muted {
		t.Fatal("stepping up unmuted although unmuteOnIncrease was false")
	}
	st, err = ep.Step(1, 2, true)
	if err != nil {
		t.Fatal(err)
	}
	if st.Muted {
		t.Fatal("stepping up stayed muted although unmuteOnIncrease was true")
	}
}

func drain(ep *Endpoint) {
	for {
		select {
		case <-ep.Changes():
		default:
			return
		}
	}
}

func waitForChange(t *testing.T, ep *Endpoint, match func(State) bool) {
	t.Helper()
	timeout := time.After(2 * time.Second)
	for {
		select {
		case st := <-ep.Changes():
			if match(st) {
				return
			}
		case <-timeout:
			t.Fatal("no matching change notification within 2s")
		}
	}
}
