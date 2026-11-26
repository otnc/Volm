// Copyright (C) 2026 otoneko.
// SPDX-License-Identifier: GPL-3.0-only

// Volm changes the volume, and with Ctrl the brightness of the monitors, with the mouse wheel over the taskbar.
//
//	volm.exe              the resident process: tray icon, hooks, OSD
//	volm.exe --settings   the settings window, in a process of its own
package main

import (
	"errors"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"slices"

	"github.com/egoist/mygo"
	"github.com/otnc/Volm/internal/config"
	"github.com/otnc/Volm/internal/settings"
	"github.com/otnc/Volm/internal/win32"
	"golang.org/x/sys/windows"
)

const (
	settingsFlag = "--settings"
	maxLogSize   = 1 << 20
)

func main() {
	// mygo build links the name and version of mygo.json; SetName gives go run and go build the same name, data directory and instance lock. The settings process needs the same name too, as MyGo names the Run entry of SetOpenAtLogin after it.
	mygo.App.SetName("Volm")

	configPath, err := config.Path()
	if err != nil {
		log.Fatal(err)
	}
	if slices.Contains(os.Args[1:], settingsFlag) {
		runSettings(configPath)
		return
	}
	runResident(configPath)
}

func runResident(configPath string) {
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}
	closeLog := openLog(filepath.Join(filepath.Dir(configPath), "volm.log"))
	defer closeLog()
	log.Printf("Volm %s starting", mygo.App.Version())

	cfg, err := config.Load(configPath)
	settingsReset := errors.Is(err, config.ErrRecovered)
	switch {
	case settingsReset:
		log.Printf("settings reset to defaults: %v", err)
	case err != nil:
		log.Printf("reading settings: %v; using defaults", err)
	}
	if _, statErr := os.Stat(configPath); errors.Is(statErr, os.ErrNotExist) {
		if err := config.Save(configPath, cfg); err != nil {
			log.Printf("writing default settings: %v", err)
		}
	}

	a := newApp(configPath, cfg)
	mygo.App.OnWindowAllClosed(func() {}) // Volm lives in the tray and opens no windows of its own.
	mygo.App.OnSecondInstance(func([]string, string) { openSettings() })
	mygo.App.WhenReady(func() {
		if err := a.start(); err != nil {
			log.Printf("starting: %v", err)
			mygo.App.Quit()
			return
		}
		if settingsReset {
			go notifySettingsReset(configPath)
		}
	})
	if err := mygo.App.Run(); err != nil {
		log.Print(err)
	}
	a.stop()
	log.Print("Volm stopped")
}

// settingsMutex keeps one settings window per user session.
const settingsMutex = `Local\VolmSettings`

func runSettings(configPath string) {
	name, _ := windows.UTF16PtrFromString(settingsMutex)
	mutex, err := windows.CreateMutex(nil, false, name)
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		win32.BringToFront(settings.WindowTitle)
		return
	}
	if err == nil {
		defer func() { _ = windows.CloseHandle(mutex) }()
	}
	closeLog := openLog(filepath.Join(filepath.Dir(configPath), "volm.log"))
	defer closeLog()

	// Read, not Load: the resident process owns recovering a broken file.
	cfg, err := config.Read(configPath)
	if err != nil {
		log.Printf("settings: reading settings: %v", err)
	}
	mygo.App.WhenReady(func() { settings.Open(configPath, cfg) })
	if err := mygo.App.Run(); err != nil {
		log.Print(err)
	}
}

// openSettings starts the settings process, which brings an open settings window to the front instead of opening a second one.
func openSettings() {
	exe, err := os.Executable()
	if err != nil {
		log.Printf("opening settings: %v", err)
		return
	}
	cmd := exec.Command(exe, settingsFlag)
	if err := cmd.Start(); err != nil {
		log.Printf("opening settings: %v", err)
		return
	}
	// Never waited for: the window lives on its own.
	if err := cmd.Process.Release(); err != nil {
		log.Printf("opening settings: %v", err)
	}
}

func notifySettingsReset(configPath string) {
	n := mygo.NewNotification(mygo.NotificationOptions{
		Title: "設定を初期値に戻しました",
		Body:  "設定ファイルを読み込めなかったため、初期値で起動しました。元のファイルは " + configPath + ".bak に残してあります。",
	})
	if err := n.Show(); err != nil {
		log.Printf("notifying settings reset: %v", err)
	}
}

// openLog sends the standard logger to path, keeping one older generation as path+".1" once the file passes maxLogSize. The resident and settings processes append to the same file.
func openLog(path string) (closeLog func()) {
	if info, err := os.Stat(path); err == nil && info.Size() > maxLogSize {
		_ = os.Rename(path, path+".1") // without rotation the log only keeps growing
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return func() {}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return func() {}
	}
	log.SetOutput(f)
	return func() { _ = f.Close() }
}

// openURL opens a URL, such as a page of the Windows settings app, with its registered handler.
func openURL(url string) error {
	verb, _ := windows.UTF16PtrFromString("open")
	target, err := windows.UTF16PtrFromString(url)
	if err != nil {
		return err
	}
	return windows.ShellExecute(0, verb, target, nil, nil, windows.SW_SHOWNORMAL)
}
