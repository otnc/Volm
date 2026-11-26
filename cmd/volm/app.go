// Copyright (C) 2026 otoneko.
// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"context"
	"errors"
	"log"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/otnc/Volm/internal/audio"
	"github.com/otnc/Volm/internal/config"
	"github.com/otnc/Volm/internal/control"
	"github.com/otnc/Volm/internal/hook"
	"github.com/otnc/Volm/internal/osd"
	"github.com/otnc/Volm/internal/tray"
	"github.com/otnc/Volm/internal/win32"
)

// wheelGestureGap is how long the wheel must rest before a partial notch is dropped, so that a leftover from an earlier gesture does not shift the first notch of the next one.
const wheelGestureGap = 500 * time.Millisecond

type app struct {
	configPath string
	cfg        atomic.Pointer[config.Config]
	events     chan hook.Event
	stopWatch  context.CancelFunc

	audio *audio.Endpoint
	osd   *osd.OSD
	tray  *tray.Tray
	mouse *hook.Mouse
}

func newApp(configPath string, cfg config.Config) *app {
	a := &app{configPath: configPath, events: make(chan hook.Event, 64), stopWatch: func() {}}
	a.cfg.Store(&cfg)
	return a
}

// start runs on MyGo's main thread once the app is ready.
func (a *app) start() error {
	var err error
	if a.audio, err = audio.Open(); err != nil {
		return err
	}
	if a.osd, err = osd.New(); err != nil {
		return err
	}
	if a.tray, err = tray.New(tray.Actions{
		ToggleMute:   func() { go a.toggleMute(win32.CursorPos()) },
		OpenSettings: openSettings,
		Quit:         mygo.App.Quit,
	}); err != nil {
		return err
	}
	if a.mouse, err = hook.StartMouse(a.takes, a.events); err != nil {
		return err
	}
	go a.followVolume()
	go a.handleInput()
	a.watchConfig()
	return nil
}

func (a *app) stop() {
	a.stopWatch()
	if a.mouse != nil {
		a.mouse.Stop()
	}
	if a.osd != nil {
		a.osd.Close()
	}
	if a.audio != nil {
		a.audio.Close()
	}
}

// takes runs inside the mouse hook callback and must return at once. Ctrl and Alt with the wheel are left to other apps until brightness and per-app volume take them.
func (a *app) takes(ev hook.Event) bool {
	if !ev.OnTaskbar {
		return false
	}
	switch ev.Kind {
	case hook.Wheel:
		return !ev.Ctrl && !ev.Alt
	case hook.MiddleClick:
		return a.cfg.Load().Volume.MiddleClickMute
	}
	return false
}

func (a *app) handleInput() {
	var notches control.NotchCounter
	var lastWheel time.Time
	for ev := range a.events {
		switch ev.Kind {
		case hook.Wheel:
			if time.Since(lastWheel) > wheelGestureGap {
				notches.Reset()
			}
			lastWheel = time.Now()
			if n := notches.Add(ev.Delta); n != 0 {
				a.stepVolume(n, ev.Pos)
			}
		case hook.MiddleClick:
			a.toggleMute(ev.Pos)
		}
	}
}

func (a *app) stepVolume(notches int, at win32.Point) {
	vol := a.cfg.Load().Volume
	st, err := a.audio.Step(notches, vol.Step, vol.UnmuteOnIncrease)
	a.showVolume(st, err, at)
}

func (a *app) toggleMute(at win32.Point) {
	st, err := a.audio.ToggleMute()
	a.showVolume(st, err, at)
}

func (a *app) showVolume(st audio.State, err error, at win32.Point) {
	if err != nil && !errors.Is(err, audio.ErrNoDevice) {
		log.Printf("volume: %v", err)
	}
	cfg := a.cfg.Load()
	if !cfg.OSD.Enabled {
		return
	}
	content := osd.Content{Icon: osd.SpeakerIcon(st.Percent, st.Muted), Title: st.Device, Percent: st.Percent, Muted: st.Muted}
	if err != nil {
		content = osd.Content{Icon: osd.IconMute, Title: "再生デバイスがありません"}
	}
	a.osd.Show(content, osd.Placement{
		Near:     at,
		Position: cfg.OSD.Position,
		Duration: time.Duration(cfg.OSD.DurationMs) * time.Millisecond,
	})
}

// followVolume keeps the tray in step with the device, including changes made by other apps and a switch of the default device.
func (a *app) followVolume() {
	for st := range a.audio.Changes() {
		a.tray.Update(st.Percent, st.Muted, st.Device)
	}
}

// watchConfig applies the settings file again whenever the settings window, or anyone else, changes it. A file that does not load keeps the current settings: the settings window writes whole files, so a broken one comes from elsewhere and should not reset what the user chose.
func (a *app) watchConfig() {
	ctx, cancel := context.WithCancel(context.Background())
	w, err := mygo.WatchFiles(ctx, a.configPath, mygo.FileWatchOptions{})
	if err != nil {
		cancel()
		log.Printf("watching settings: %v", err)
		return
	}
	a.stopWatch = cancel
	go func() {
		for {
			if _, err := w.Next(ctx); err != nil {
				return
			}
			cfg, err := config.Read(a.configPath)
			if err != nil {
				log.Printf("reloading settings: %v", err)
				continue
			}
			a.cfg.Store(&cfg)
			log.Print("settings reloaded")
		}
	}()
}
