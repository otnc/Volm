// Copyright (C) 2026 otoneko.
// SPDX-License-Identifier: GPL-3.0-only

package brightness

import "testing"

func TestWMIKeyMatchesInstanceName(t *testing.T) {
	// Taken from the built-in panel of the development machine (PoC 3).
	id := `\\?\DISPLAY#SHP158C#4&1227b4ea&0&UID8388688#{e6f07b5f-ee97-4a90-b076-33f57bf4eaa7}`
	instance := `DISPLAY\SHP158C\4&1227b4ea&0&UID8388688_0`
	if got, want := WMIKey(id), WMIInstanceKey(instance); got != want {
		t.Fatalf("WMIKey = %q, WMIInstanceKey = %q; want them equal", got, want)
	}
	if WMIKey(`\\?\DISPLAY#HWP2827#4&1227b4ea&0&UID8261#{e6f07b5f-ee97-4a90-b076-33f57bf4eaa7}`) == WMIInstanceKey(instance) {
		t.Fatal("another monitor matched the panel's instance")
	}
}

func TestEffectiveMethod(t *testing.T) {
	tests := []struct {
		configured string
		detected   Method
		want       Method
	}{
		{"auto", MethodWMI, MethodWMI},
		{"", MethodDDC, MethodDDC},
		{"unknown", MethodSoftDim, MethodSoftDim},
		{"softDim", MethodWMI, MethodSoftDim},
		{"ddcci", MethodSoftDim, MethodDDC},
		{"disabled", MethodWMI, MethodDisabled},
	}
	for _, tt := range tests {
		if got := EffectiveMethod(tt.configured, tt.detected); got != tt.want {
			t.Errorf("EffectiveMethod(%q, %q) = %q, want %q", tt.configured, tt.detected, got, tt.want)
		}
	}
}

func TestStepTarget(t *testing.T) {
	tests := []struct {
		name                   string
		current, notches, step int
		method                 Method
		softDimMin, want       int
	}{
		{"hardware goes down to 0", 5, -2, 5, MethodWMI, 20, 0},
		{"overlay stops at its floor", 25, -2, 5, MethodSoftDim, 20, 20},
		{"overlay goes up to no overlay", 95, 2, 5, MethodSoftDim, 20, 100},
		{"hardware step up", 50, 1, 5, MethodDDC, 20, 55},
	}
	for _, tt := range tests {
		if got := stepTarget(tt.current, tt.notches, tt.step, tt.method, tt.softDimMin); got != tt.want {
			t.Errorf("%s: got %d, want %d", tt.name, got, tt.want)
		}
	}
}

func TestOverlayAlpha(t *testing.T) {
	tests := []struct {
		percent int
		want    byte
	}{{100, 0}, {50, 128}, {20, 204}, {0, 255}, {120, 0}, {-5, 255}}
	for _, tt := range tests {
		if got := overlayAlpha(tt.percent); got != tt.want {
			t.Errorf("overlayAlpha(%d) = %d, want %d", tt.percent, got, tt.want)
		}
	}
}

func TestDDCRange(t *testing.T) {
	r := ddcRange{min: 0, current: 30, max: 60}
	if got := r.percent(); got != 50 {
		t.Errorf("percent = %d, want 50", got)
	}
	if got := r.value(100); got != 60 {
		t.Errorf("value(100) = %d, want 60", got)
	}
	if got := (ddcRange{}).value(40); got != 40 {
		t.Errorf("value of an unread range = %d, want 40", got)
	}
}
