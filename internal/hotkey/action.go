// Copyright (C) 2026 otoneko.
// SPDX-License-Identifier: GPL-3.0-only

package hotkey

import (
	"fmt"
	"log"
)

// Action is what a hotkey does, named as the keys of config.Config.Hotkeys.
type Action string

// The actions, in the order the settings window lists them.
const (
	VolumeUp       Action = "volumeUp"
	VolumeDown     Action = "volumeDown"
	ToggleMute     Action = "toggleMute"
	BrightnessUp   Action = "brightnessUp"
	BrightnessDown Action = "brightnessDown"
	PlayPause      Action = "playPause"
	NextTrack      Action = "nextTrack"
	PreviousTrack  Action = "previousTrack"
	NextDevice     Action = "nextOutputDevice"
)

// Actions lists every action with its name in the settings window.
var Actions = []struct {
	Action Action
	Label  string
}{
	{VolumeUp, "音量を上げる"},
	{VolumeDown, "音量を下げる"},
	{ToggleMute, "ミュートを切り替える"},
	{BrightnessUp, "明るさを上げる"},
	{BrightnessDown, "明るさを下げる"},
	{PlayPause, "再生/一時停止"},
	{NextTrack, "次の曲"},
	{PreviousTrack, "前の曲"},
	{NextDevice, "出力デバイスを順に切り替える"},
}

// MaxKeysPerAction is how many combinations one action can have.
const MaxKeysPerAction = 2

// Bindings maps combinations to the actions they run.
type Bindings map[Combo]Action

// Bind reads the hotkeys of the settings file. A combination that does not parse, or that a second action also claims, is logged and skipped rather than failing the rest; the first action in Actions order keeps a shared combination.
func Bind(hotkeys map[string][]string) Bindings {
	b := Bindings{}
	for _, a := range Actions {
		for i, s := range hotkeys[string(a.Action)] {
			if i >= MaxKeysPerAction {
				break
			}
			c, err := Parse(s)
			if err != nil {
				log.Printf("hotkey: %s: %v", a.Action, err)
				continue
			}
			if other, taken := b[c]; taken {
				log.Printf("hotkey: %s is already %s; skipped for %s", c, other, a.Action)
				continue
			}
			b[c] = a.Action
		}
	}
	return b
}

// Conflict returns the action other than except that already uses c, if any.
func Conflict(hotkeys map[string][]string, c Combo, except Action) (Action, bool) {
	for _, a := range Actions {
		if a.Action == except {
			continue
		}
		for _, s := range hotkeys[string(a.Action)] {
			if other, err := Parse(s); err == nil && other == c {
				return a.Action, true
			}
		}
	}
	return "", false
}

// Label returns the action's name in the settings window.
func (a Action) Label() string {
	for _, x := range Actions {
		if x.Action == a {
			return x.Label
		}
	}
	return fmt.Sprint(string(a))
}
