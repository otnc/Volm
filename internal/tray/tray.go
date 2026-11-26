// Copyright (C) 2026 otoneko.
// SPDX-License-Identifier: GPL-3.0-only

// Package tray shows Volm's notification area icon, whose picture and tooltip follow the volume, and its menu.
package tray

import (
	"fmt"

	"github.com/egoist/mygo"
)

// Actions are the menu commands. They run on MyGo's main thread and must hand slow work to another goroutine.
type Actions struct {
	ToggleMute   func()
	OpenSettings func()
	Quit         func()
}

// Tray is the icon. Its methods may be called from any goroutine.
type Tray struct {
	icon  *mygo.Tray
	mute  *mygo.MenuItem
	level Level
}

// New adds the icon. Call it once MyGo is ready (in mygo.App.WhenReady).
func New(actions Actions) (*Tray, error) {
	// No level matches -1, so the first Update always sets the icon.
	t := &Tray{level: -1}
	// MyGo toggles a checkbox item before calling Click; Update sets it again from the device, so a failed toggle does not leave the check wrong.
	t.mute = &mygo.MenuItem{Label: "ミュート", Type: mygo.MenuItemCheckbox, Click: func(*mygo.MenuItem, *mygo.Window) { actions.ToggleMute() }}
	menu := mygo.NewMenu([]*mygo.MenuItem{
		t.mute,
		mygo.Separator(),
		{Label: "設定", Click: func(*mygo.MenuItem, *mygo.Window) { actions.OpenSettings() }},
		{Label: "終了", Click: func(*mygo.MenuItem, *mygo.Window) { actions.Quit() }},
	})
	icon, err := mygo.NewTray(mygo.TrayOptions{
		ID:      "volm",
		Icon:    levelIcon(LevelMid),
		ToolTip: "Volm",
		Menu:    menu,
	})
	if err != nil {
		return nil, err
	}
	t.icon = icon
	icon.OnClick(actions.OpenSettings)
	return t, nil
}

// Update shows the volume of the default device. device is empty when there is no playback device.
func (t *Tray) Update(percent int, muted bool, device string) {
	mygo.RunOnMain(func() {
		if level := LevelOf(percent, muted); level != t.level {
			t.level = level
			t.icon.SetIcon(levelIcon(level), false)
		}
		t.icon.SetToolTip(Tooltip(percent, muted, device))
		t.mute.SetChecked(muted)
	})
}

// Tooltip returns the hover text, such as "音量 40% / スピーカー (Realtek(R) Audio)".
func Tooltip(percent int, muted bool, device string) string {
	if device == "" {
		return "再生デバイスがありません"
	}
	level := fmt.Sprintf("音量 %d%%", percent)
	if muted {
		level = "ミュート中"
	}
	return level + " / " + device
}
