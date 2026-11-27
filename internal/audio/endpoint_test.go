// Copyright (C) 2026 otoneko.
// SPDX-License-Identifier: GPL-3.0-only

package audio

import (
	"errors"
	"os"
	"slices"
	"testing"
	"time"
)

// These tests change the real system volume for a moment and restore it. They run only with VOLM_INTEGRATION=1 on a machine with a playback device, and the recording tests also need a microphone.
func openForTest(t *testing.T, flow Flow) *Endpoint {
	t.Helper()
	if os.Getenv("VOLM_INTEGRATION") != "1" {
		t.Skip("set VOLM_INTEGRATION=1 to change the real system volume")
	}
	ep, err := Open(flow)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ep.Close)
	drain(ep)
	return ep
}

func TestStepChangesVolumeAndNotifies(t *testing.T) {
	ep := openForTest(t, Playback)
	before, err := ep.Current()
	if err != nil {
		t.Fatal(err)
	}
	notches := 1
	if before.Percent >= 98 {
		notches = -1
	}
	t.Cleanup(func() {
		if _, err := ep.Step(-notches, 2, false); err != nil {
			t.Errorf("restoring volume: %v", err)
		}
	})

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
	ep := openForTest(t, Playback)
	before, err := ep.Current()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		restoreMute(t, ep, before.Muted)
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
	ep := openForTest(t, Playback)
	before, err := ep.Current()
	if err != nil {
		t.Fatal(err)
	}
	if before.Percent >= 96 {
		t.Skip("volume too close to 100% to step up twice")
	}
	t.Cleanup(func() {
		if _, err := ep.Step(-2, 2, false); err != nil {
			t.Errorf("restoring volume: %v", err)
		}
		restoreMute(t, ep, before.Muted)
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

func restoreMute(t *testing.T, ep *Endpoint, muted bool) {
	t.Helper()
	st, err := ep.Current()
	if err != nil {
		t.Errorf("reading mute: %v", err)
		return
	}
	if st.Muted != muted {
		if _, err := ep.ToggleMute(); err != nil {
			t.Errorf("restoring mute: %v", err)
		}
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

func TestNextDeviceCyclesAndRestores(t *testing.T) {
	ep := openForTest(t, Playback)
	devices, err := ep.Devices()
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) < 2 {
		t.Skip("needs two active playback devices")
	}
	var original Device
	for _, d := range devices {
		if d.Default {
			original = d
		}
	}
	if original.ID == "" {
		t.Fatal("no device is marked default")
	}
	t.Cleanup(func() {
		if err := ep.SetDefault(original.ID); err != nil {
			t.Errorf("restoring the default device: %v", err)
		}
	})

	next, err := ep.NextDevice()
	if err != nil {
		t.Fatal(err)
	}
	if next.ID == original.ID {
		t.Fatalf("NextDevice stayed on %s", original.Name)
	}
	waitForChange(t, ep, func(st State) bool { return st.Device == next.Name })
}

// TestRecordingFollowsTheMicrophone checks that a recording endpoint reads and changes the default microphone, not the speakers.
func TestRecordingFollowsTheMicrophone(t *testing.T) {
	mic := openForTest(t, Recording)
	speakers := openForTest(t, Playback)
	before, err := mic.Current()
	if errors.Is(err, ErrNoDevice) {
		t.Skip("no microphone")
	}
	if err != nil {
		t.Fatal(err)
	}
	speakersBefore, err := speakers.Current()
	if err != nil {
		t.Fatal(err)
	}
	notches := 1
	if before.Percent >= 98 {
		notches = -1
	}
	t.Cleanup(func() {
		if _, err := mic.Step(-notches, 2, false); err != nil {
			t.Errorf("restoring microphone volume: %v", err)
		}
	})
	after, err := mic.Step(notches, 2, false)
	if err != nil {
		t.Fatal(err)
	}
	if want := before.Percent + notches*2; after.Percent != want {
		t.Fatalf("microphone percent = %d, want %d", after.Percent, want)
	}
	if st, err := speakers.Current(); err != nil || st.Percent != speakersBefore.Percent {
		t.Fatalf("speakers changed to %d (%v) from %d", st.Percent, err, speakersBefore.Percent)
	}
	devices, err := mic.Devices()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(devices, func(d Device) bool { return d.Default && d.Name == before.Device }) {
		t.Fatalf("devices %+v do not mark %q as the default", devices, before.Device)
	}
}
