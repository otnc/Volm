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
	"github.com/otnc/Volm/internal/appid"
	"github.com/otnc/Volm/internal/audio"
	"github.com/otnc/Volm/internal/brightness"
	"github.com/otnc/Volm/internal/config"
	"github.com/otnc/Volm/internal/control"
	"github.com/otnc/Volm/internal/hook"
	"github.com/otnc/Volm/internal/hotkey"
	"github.com/otnc/Volm/internal/osd"
	"github.com/otnc/Volm/internal/taskbar"
	"github.com/otnc/Volm/internal/tray"
	"github.com/otnc/Volm/internal/win32"
)

// wheelGestureGap is how long the wheel must rest before a partial notch is dropped, so that a leftover from an earlier gesture does not shift the first notch of the next one.
const wheelGestureGap = 500 * time.Millisecond

type app struct {
	configPath string
	cfg        atomic.Pointer[config.Config]
	bindings   atomic.Pointer[hotkey.Bindings]
	events     chan hook.Event
	keys       chan hook.KeyEvent
	stopWatch  context.CancelFunc

	audio      *audio.Endpoint
	mic        *audio.Endpoint     // nil when the recording side failed to start; playback still works
	brightness *brightness.Manager // nil when it failed to start; volume still works
	osd        *osd.OSD
	taskbar    *taskbar.Finder // nil when UI Automation failed to start; per-app volume is then off
	tray       *tray.Tray
	mouse      *hook.Mouse
	keyboard   *hook.Keyboard
}

func newApp(configPath string, cfg config.Config) *app {
	a := &app{configPath: configPath, events: make(chan hook.Event, 64), keys: make(chan hook.KeyEvent, 16), stopWatch: func() {}}
	a.setConfig(cfg)
	return a
}

// start runs on MyGo's main thread once the app is ready.
func (a *app) start() error {
	var err error
	if a.audio, err = audio.Open(audio.Playback); err != nil {
		return err
	}
	if a.mic, err = audio.Open(audio.Recording); err != nil {
		log.Printf("microphone: %v", err)
	}
	if a.osd, err = osd.New(); err != nil {
		return err
	}
	if a.brightness, err = brightness.Start(); err != nil {
		log.Printf("brightness: %v", err)
	}
	if a.taskbar, err = taskbar.NewFinder(); err != nil {
		log.Printf("taskbar: %v", err)
	}
	if a.tray, err = tray.New(tray.Actions{
		ToggleMute:        func() { go a.toggleMute(win32.CursorPos()) },
		ToggleMicMute:     func() { go a.toggleMicMute(win32.CursorPos()) },
		Devices:           func() []tray.Device { return trayDevices(a.audio) },
		SelectDevice:      func(id string) { go a.selectDevice(a.audio, id) },
		InputDevices:      func() []tray.Device { return trayDevices(a.mic) },
		SelectInputDevice: func(id string) { go a.selectDevice(a.mic, id) },
		OpenSettings:      openSettings,
		Quit:              mygo.App.Quit,
	}); err != nil {
		return err
	}
	if a.mouse, err = hook.StartMouse(a.takes, a.events); err != nil {
		return err
	}
	if a.keyboard, err = hook.StartKeyboard(a.hotkeyAction, a.keys); err != nil {
		return err
	}
	go a.handleHotkeys()
	go a.followVolume()
	if a.mic != nil {
		go a.followMic()
	}
	go a.handleInput()
	a.watchConfig()
	return nil
}

func (a *app) stop() {
	a.stopWatch()
	if a.mouse != nil {
		a.mouse.Stop()
	}
	if a.keyboard != nil {
		a.keyboard.Stop()
	}
	if a.osd != nil {
		a.osd.Close()
	}
	if a.brightness != nil {
		a.brightness.Close()
	}
	if a.taskbar != nil {
		a.taskbar.Close()
	}
	if a.audio != nil {
		a.audio.Close()
	}
	if a.mic != nil {
		a.mic.Close()
	}
}

// takes runs inside the mouse hook callback and must return at once. Alt with the wheel is left to other apps until per-app volume takes it.
func (a *app) takes(ev hook.Event) bool {
	overTitleBarWheel := ev.OnTitleBar && ev.Kind == hook.Wheel
	if !ev.OnTaskbar && !overTitleBarWheel {
		return false
	}
	cfg := a.cfg.Load()
	if a.pausedForFullscreen(cfg) {
		return false
	}
	switch ev.Kind {
	case hook.Wheel:
		if ev.OnTitleBar {
			// Over a title bar only Alt+wheel is Volm's: a plain wheel there switches a browser's tabs.
			return ev.Alt && !ev.Ctrl && cfg.Volume.AppVolumeOnTitleBar && a.taskbar != nil
		}
		switch {
		case ev.Alt:
			return cfg.Volume.AppVolumeWithAlt && a.taskbar != nil
		case ev.Ctrl:
			return cfg.Brightness.WheelEnabled
		default:
			return cfg.Volume.WheelEnabled
		}
	case hook.MiddleClick:
		return cfg.Volume.MiddleClickMute
	}
	return false
}

func (a *app) handleInput() {
	// Volume and brightness count notches apart, so that pressing Ctrl in the middle of a gesture does not carry a partial notch over.
	var volumeNotches, brightnessNotches, appNotches control.NotchCounter
	var volumeAccel, appAccel control.Accelerator
	var apps appCache
	var lastWheel time.Time
	for ev := range a.events {
		switch ev.Kind {
		case hook.Wheel:
			if time.Since(lastWheel) > wheelGestureGap {
				volumeNotches.Reset()
				brightnessNotches.Reset()
				appNotches.Reset()
			}
			lastWheel = time.Now()
			if ev.Alt {
				// Alt is still held; without this, the foreground app focuses its menu bar when it is released.
				win32.SendMaskKey()
				if n := appNotches.Add(ev.Delta); n != 0 {
					a.stepAppVolume(n, a.wheelStep(false, appAccel.Factor(ev.Time, n)), ev, &apps)
				}
			} else if ev.Ctrl {
				if n := brightnessNotches.Add(ev.Delta); n != 0 {
					a.stepBrightness(n, ev.Shift, ev.Pos)
				}
			} else if n := volumeNotches.Add(ev.Delta); n != 0 {
				a.stepVolume(n, a.wheelStep(ev.Shift, volumeAccel.Factor(ev.Time, n)), ev.Pos)
			}
		case hook.MiddleClick:
			a.toggleMute(ev.Pos)
		}
	}
}

// wheelStep returns the percent one notch of the wheel moves a volume by: 1 for fine adjustment with Shift, else the configured step, multiplied by the accelerator's factor when acceleration is on.
func (a *app) wheelStep(fine bool, factor int) int {
	vol := a.cfg.Load().Volume
	switch {
	case fine:
		return 1
	case vol.Acceleration:
		return vol.Step * factor
	default:
		return vol.Step
	}
}

func (a *app) stepVolume(notches, step int, at win32.Point) {
	st, err := a.audio.Step(notches, step, a.cfg.Load().Volume.UnmuteOnIncrease)
	a.showVolume(st, err, at)
}

// stepBrightness changes the monitor under the cursor, or every monitor with Shift.
func (a *app) stepBrightness(notches int, all bool, at win32.Point) {
	if a.brightness == nil {
		return
	}
	changes, err := a.brightness.Step(at, all, notches, a.cfg.Load().Brightness)
	switch {
	case errors.Is(err, brightness.ErrDetecting):
		a.show(osd.Content{Icon: osd.IconBrightness, Title: "モニターを調べています"}, at)
	case err != nil:
		log.Printf("brightness: %v", err)
	case len(changes) > 0:
		title := changes[0].Name
		if all {
			title = "すべてのモニター"
		}
		a.show(osd.Content{Icon: osd.IconBrightness, Title: title, Percent: changes[0].Percent}, at)
	}
}

func (a *app) toggleMute(at win32.Point) {
	st, err := a.audio.ToggleMute()
	a.showVolume(st, err, at)
}

func (a *app) showVolume(st audio.State, err error, at win32.Point) {
	if err != nil && !errors.Is(err, audio.ErrNoDevice) {
		log.Printf("volume: %v", err)
	}
	content := osd.Content{Icon: osd.SpeakerIcon(st.Percent, st.Muted), Title: st.Device, Percent: st.Percent, Muted: st.Muted}
	if err != nil {
		content = osd.Content{Icon: osd.IconMute, Title: "再生デバイスがありません"}
	}
	a.show(content, at)
}

// pausedForFullscreen reports whether the wheel and the OSD should stay out of a full-screen app's way. It takes about 0.1 ms, quick enough for the mouse hook.
func (a *app) pausedForFullscreen(cfg *config.Config) bool {
	return cfg.PauseInFullscreen && win32.FullscreenAppInFront()
}

func (a *app) show(content osd.Content, at win32.Point) {
	cfg := a.cfg.Load()
	if !cfg.OSD.Enabled || a.pausedForFullscreen(cfg) {
		return
	}
	a.osd.Show(content, osd.Placement{
		Near:     at,
		Position: cfg.OSD.Position,
		Duration: time.Duration(cfg.OSD.DurationMs) * time.Millisecond,
	})
}

// stepMic changes the microphone's volume by the configured step. It never unmutes: a microphone that turns on by surprise is worse than one that stays muted.
func (a *app) stepMic(notches int, at win32.Point) {
	if a.mic == nil {
		return
	}
	st, err := a.mic.Step(notches, a.cfg.Load().Volume.Step, false)
	a.showMic(st, err, at)
}

func (a *app) toggleMicMute(at win32.Point) {
	if a.mic == nil {
		return
	}
	st, err := a.mic.ToggleMute()
	a.showMic(st, err, at)
}

func (a *app) showMic(st audio.State, err error, at win32.Point) {
	if err != nil && !errors.Is(err, audio.ErrNoDevice) {
		log.Printf("microphone: %v", err)
	}
	content := osd.Content{Icon: osd.MicrophoneIcon(st.Muted), Title: st.Device, Percent: st.Percent, Muted: st.Muted}
	if err != nil {
		content = osd.Content{Icon: osd.IconMicOff, Title: "録音デバイスがありません"}
	}
	a.show(content, at)
}

// followMic keeps the tray menu's microphone check in step with the device.
func (a *app) followMic() {
	for st := range a.mic.Changes() {
		a.tray.UpdateMic(st.Muted, st.Device != "")
	}
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
			a.setConfig(cfg)
			if a.brightness != nil {
				a.brightness.ApplyConfig(cfg.Brightness)
			}
			log.Print("settings reloaded")
		}
	}()
}

// setConfig publishes new settings, and the hotkeys they bind, to the hooks and the input handlers.
func (a *app) setConfig(cfg config.Config) {
	bindings := hotkey.Bind(cfg.Hotkeys)
	a.bindings.Store(&bindings)
	a.cfg.Store(&cfg)
}

// hotkeyAction runs inside the keyboard hook callback and must return at once.
func (a *app) hotkeyAction(c hotkey.Combo) (hotkey.Action, bool) {
	action, ok := (*a.bindings.Load())[c]
	return action, ok
}

// The virtual-key codes of the media keys.
const (
	vkMediaNext      = 0xB0
	vkMediaPrevious  = 0xB1
	vkMediaPlayPause = 0xB3
)

func (a *app) handleHotkeys() {
	for ev := range a.keys {
		// The keys are still held: a lone Win would open the Start menu once released, and a lone Alt would focus the menu bar of the foreground app.
		if ev.Combo.Mods&(hotkey.Win|hotkey.Alt) != 0 {
			win32.SendMaskKey()
		}
		at := win32.CursorPos()
		switch ev.Action {
		case hotkey.VolumeUp:
			a.stepVolume(1, a.cfg.Load().Volume.Step, at)
		case hotkey.VolumeDown:
			a.stepVolume(-1, a.cfg.Load().Volume.Step, at)
		case hotkey.ToggleMute:
			a.toggleMute(at)
		case hotkey.BrightnessUp:
			a.stepBrightness(1, false, at)
		case hotkey.BrightnessDown:
			a.stepBrightness(-1, false, at)
		case hotkey.PlayPause:
			win32.SendKeyPress(vkMediaPlayPause)
		case hotkey.NextTrack:
			win32.SendKeyPress(vkMediaNext)
		case hotkey.PreviousTrack:
			win32.SendKeyPress(vkMediaPrevious)
		case hotkey.NextDevice:
			a.nextDevice(at)
		case hotkey.MicVolumeUp:
			a.stepMic(1, at)
		case hotkey.MicVolumeDown:
			a.stepMic(-1, at)
		case hotkey.ToggleMicMute:
			a.toggleMicMute(at)
		}
	}
}

// trayDevices runs on MyGo's main thread as the tray menu opens. ep may be nil, when the microphone side failed to start.
func trayDevices(ep *audio.Endpoint) []tray.Device {
	if ep == nil {
		return nil
	}
	devices, err := ep.Devices()
	if err != nil {
		log.Printf("listing devices: %v", err)
		return nil
	}
	out := make([]tray.Device, len(devices))
	for i, d := range devices {
		out[i] = tray.Device{ID: d.ID, Name: d.Name, Default: d.Default}
	}
	return out
}

func (a *app) selectDevice(ep *audio.Endpoint, id string) {
	if err := ep.SetDefault(id); err != nil {
		kind := "出力デバイス"
		if ep == a.mic {
			kind = "入力デバイス"
		}
		a.deviceSwitchFailed(kind, err)
	}
}

func (a *app) nextDevice(at win32.Point) {
	if _, err := a.audio.NextDevice(); err != nil {
		if errors.Is(err, audio.ErrNoDevice) {
			a.showVolume(audio.State{}, err, at)
			return
		}
		a.deviceSwitchFailed("出力デバイス", err)
		return
	}
	st, err := a.audio.Current()
	a.showVolume(st, err, at)
}

// deviceSwitchFailed tells the user and opens the sound settings of Windows, where the device can be chosen by hand. IPolicyConfig, which Volm switches with, is undocumented and could stop working after an update of Windows. kind is "出力デバイス" or "入力デバイス", as the tray menu names them.
func (a *app) deviceSwitchFailed(kind string, err error) {
	log.Printf("switching device: %v", err)
	n := mygo.NewNotification(mygo.NotificationOptions{
		Title: kind + "を切り替えられませんでした",
		Body:  "Windows のサウンドの設定を開きます。そこで" + kind + "を選んでください。",
	})
	if err := n.Show(); err != nil {
		log.Printf("notifying: %v", err)
	}
	if err := openURL("ms-settings:sound"); err != nil {
		log.Printf("opening sound settings: %v", err)
	}
}

// appCache keeps the app found under the cursor for a moment, as UI Automation and the window enumeration take milliseconds and a wheel sends many notches over the same button.
type appCache struct {
	button taskbar.Button
	target appid.Target
	found  time.Time
}

const appCacheLifetime = 2 * time.Second

// stepAppVolume changes the volume of the app whose taskbar button, or window title bar, is under the cursor. Over anything else on the taskbar it does nothing.
func (a *app) stepAppVolume(notches, step int, ev hook.Event, cache *appCache) {
	at := ev.Pos
	var button taskbar.Button
	var err error
	if ev.OnTitleBar {
		button, err = a.taskbar.WindowButton(ev.Window)
	} else {
		var ok bool
		button, ok, err = a.taskbar.ButtonAt(at)
		if !ok {
			return
		}
	}
	if err != nil || button.AUMID == "" {
		return
	}
	if button.AUMID != cache.button.AUMID || time.Since(cache.found) > appCacheLifetime {
		target, err := a.taskbar.Target(button)
		if err != nil {
			log.Printf("app volume: %v", err)
			return
		}
		*cache = appCache{button: button, target: target, found: time.Now()}
	}
	st, err := a.audio.StepApp(cache.target, notches, step, a.cfg.Load().Volume.UnmuteOnIncrease)
	if err != nil {
		log.Printf("app volume: %v", err)
		return
	}
	content := osd.Content{Icon: osd.SpeakerIcon(st.Percent, st.Muted), Title: button.Name, Percent: st.Percent, Muted: st.Muted}
	if !st.Found {
		content = osd.Content{Icon: osd.IconSpeakerZero, Title: button.Name, Status: "音声なし"}
	}
	a.show(content, at)
}
