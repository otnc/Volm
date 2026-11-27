// Copyright (C) 2026 otoneko.
// SPDX-License-Identifier: GPL-3.0-only

// Package tray shows Volm's notification area icon, which shows the volume as a number and its device in the tooltip, and its menu.
package tray

import (
	"fmt"
	"log"

	"github.com/egoist/mygo"

	"github.com/otnc/Volm/internal/win32"
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
	// shown is what the icon shows now, and muted whether the device is muted; both are touched only on the main thread.
	shown iconKey
	muted bool
}

type iconKey struct {
	label string
	muted bool
	size  int
}

// New adds the icon. Call it once MyGo is ready (in mygo.App.WhenReady).
func New(actions Actions) (*Tray, error) {
	t := &Tray{actions: actions, shown: iconKey{label: noDeviceLabel, size: win32.TrayIconSize()}}
	icon, err := mygo.NewTray(mygo.TrayOptions{
		ID:      "volm",
		Icon:    encodePNG(Draw(t.shown.label, t.shown.muted, t.shown.size)),
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
		// The size is looked up again on every change, so that a new display scale takes effect with the next change of the volume.
		key := iconKey{label: Label(percent, device != ""), muted: muted, size: win32.TrayIconSize()}
		if key != t.shown {
			if err := t.icon.SetIcon(encodePNG(Draw(key.label, key.muted, key.size)), false); err != nil {
				log.Printf("tray: setting icon: %v", err)
			} else {
				t.shown = key
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
