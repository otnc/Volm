// Copyright (C) 2026 otoneko.
// SPDX-License-Identifier: GPL-3.0-only

// Package tray shows Volm's notification area icon, whose picture and tooltip follow the volume, and its menu.
package tray

import (
	"fmt"
	"log"

	"github.com/egoist/mygo"
)

// Actions are the menu commands. They run on MyGo's main thread and must hand slow work to another goroutine, apart from Devices, which the menu needs before it opens.
type Actions struct {
	ToggleMute   func()
	Devices      func() []Device
	SelectDevice func(id string)
	OpenSettings func()
	Quit         func()
}

// Device is a playback device in the menu.
type Device struct {
	ID, Name string
	Default  bool
}

// Tray is the icon. Its methods may be called from any goroutine.
type Tray struct {
	icon    *mygo.Tray
	actions Actions
	level   Level
	muted   bool // touched only on the main thread
}

// New adds the icon. Call it once MyGo is ready (in mygo.App.WhenReady).
func New(actions Actions) (*Tray, error) {
	// No level matches -1, so the first Update always sets the icon.
	t := &Tray{actions: actions, level: -1}
	icon, err := mygo.NewTray(mygo.TrayOptions{
		ID:      "volm",
		Icon:    levelIcon(LevelMid),
		ToolTip: "Volm",
		Menu:    t.menu(nil),
	})
	if err != nil {
		return nil, err
	}
	t.icon = icon
	icon.OnClick(actions.OpenSettings)
	// On Windows the right click arrives before the menu opens, on the main thread, so the menu shows the devices as they are now.
	icon.OnRightClick(func() { icon.SetMenu(t.menu(actions.Devices())) })
	return t, nil
}

func (t *Tray) menu(devices []Device) *mygo.Menu {
	// MyGo toggles a checkbox item before calling Click; Update sets it again from the device, so a failed toggle does not leave the check wrong.
	mute := &mygo.MenuItem{Label: "ミュート", Type: mygo.MenuItemCheckbox, Checked: t.muted, Click: func(*mygo.MenuItem, *mygo.Window) { t.actions.ToggleMute() }}
	output := &mygo.MenuItem{Label: "出力デバイス"}
	for _, d := range devices {
		id := d.ID
		output.Submenu = append(output.Submenu, &mygo.MenuItem{
			Label:   d.Name,
			Type:    mygo.MenuItemRadio,
			Checked: d.Default,
			Click:   func(*mygo.MenuItem, *mygo.Window) { t.actions.SelectDevice(id) },
		})
	}
	if len(output.Submenu) == 0 {
		output.Submenu = []*mygo.MenuItem{{Label: "再生デバイスがありません", Disabled: true}}
	}
	return mygo.NewMenu([]*mygo.MenuItem{
		mute,
		output,
		mygo.Separator(),
		{Label: "設定", Click: func(*mygo.MenuItem, *mygo.Window) { t.actions.OpenSettings() }},
		{Label: "終了", Click: func(*mygo.MenuItem, *mygo.Window) { t.actions.Quit() }},
	})
}

// Update shows the volume of the default device. device is empty when there is no playback device.
func (t *Tray) Update(percent int, muted bool, device string) {
	mygo.RunOnMain(func() {
		if level := LevelOf(percent, muted); level != t.level {
			t.level = level
			if err := t.icon.SetIcon(levelIcon(level), false); err != nil {
				log.Printf("tray: setting icon: %v", err)
			}
		}
		t.icon.SetToolTip(Tooltip(percent, muted, device))
		t.muted = muted
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
