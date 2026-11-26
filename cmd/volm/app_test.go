// Copyright (C) 2026 otoneko.
// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"testing"

	"github.com/otnc/Volm/internal/config"
	"github.com/otnc/Volm/internal/hook"
)

func TestTakes(t *testing.T) {
	withMiddle := config.Default()
	withoutMiddle := config.Default()
	withoutMiddle.Volume.MiddleClickMute = false

	tests := []struct {
		name string
		cfg  config.Config
		ev   hook.Event
		want bool
	}{
		{"wheel on taskbar", withMiddle, hook.Event{Kind: hook.Wheel, OnTaskbar: true}, true},
		{"wheel with shift on taskbar", withMiddle, hook.Event{Kind: hook.Wheel, OnTaskbar: true, Shift: true}, true},
		{"wheel elsewhere", withMiddle, hook.Event{Kind: hook.Wheel}, false},
		{"ctrl wheel for brightness", withMiddle, hook.Event{Kind: hook.Wheel, OnTaskbar: true, Ctrl: true}, true},
		{"ctrl shift wheel for every monitor", withMiddle, hook.Event{Kind: hook.Wheel, OnTaskbar: true, Ctrl: true, Shift: true}, true},
		{"alt wheel is left for per-app volume", withMiddle, hook.Event{Kind: hook.Wheel, OnTaskbar: true, Alt: true}, false},
		{"middle click on taskbar", withMiddle, hook.Event{Kind: hook.MiddleClick, OnTaskbar: true}, true},
		{"middle click with the setting off", withoutMiddle, hook.Event{Kind: hook.MiddleClick, OnTaskbar: true}, false},
		{"middle click elsewhere", withMiddle, hook.Event{Kind: hook.MiddleClick}, false},
	}
	for _, tt := range tests {
		a := newApp("", tt.cfg)
		if got := a.takes(tt.ev); got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}
