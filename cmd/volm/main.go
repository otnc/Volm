// Copyright (C) 2026 otoneko.
// SPDX-License-Identifier: GPL-3.0-only

// Volm changes the volume, and with Ctrl the brightness of the monitors, with the mouse wheel over the taskbar.
//
//	volm.exe              the resident process: tray icon, hooks, OSD
//	volm.exe --settings   the settings window, in a process of its own
package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"time"

	"github.com/egoist/mygo"
	"github.com/otnc/Volm/internal/config"
	"github.com/otnc/Volm/internal/ipc"
	"github.com/otnc/Volm/internal/settings"
	"github.com/otnc/Volm/internal/update"
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

	_, statErr := os.Stat(configPath)
	firstRun := errors.Is(statErr, os.ErrNotExist)
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

	// The installer waits for this mutex to go before it replaces Volm.exe; see installer/volm.iss.
	if name, err := windows.UTF16PtrFromString(runningMutex); err == nil {
		if mutex, err := windows.CreateMutex(nil, false, name); err == nil {
			defer func() { _ = windows.CloseHandle(mutex) }()
		}
	}

	stopListening, err := ipc.ListenQuit(mygo.App.Quit)
	if err != nil {
		log.Printf("listening for quit requests: %v", err)
	} else {
		defer stopListening()
	}

	a := newApp(configPath, cfg)
	mygo.App.OnWindowAllClosed(func() {}) // Volm lives in the tray and opens no windows of its own.
	mygo.App.OnSecondInstance(func([]string, string) { openSettings() })
	mygo.App.OnNotificationClick(func(id string) {
		if id == updateNotificationID {
			openSettings()
		}
	})
	mygo.App.WhenReady(func() {
		if err := a.start(); err != nil {
			log.Printf("starting: %v", err)
			mygo.App.Quit()
			return
		}
		if settingsReset {
			go notifySettingsReset(configPath)
		}
		go checkForUpdateAtStart(configPath, cfg.Update)
		// Starting at login is on by default. The installer sets it; a copy run without one sets it on its first start. A development build would register its temporary executable, so it does not.
		if firstRun && !mygo.IsDev() {
			if err := mygo.App.SetOpenAtLogin(true); err != nil {
				log.Printf("turning on start at login: %v", err)
			}
		}
	})
	if err := mygo.App.Run(); err != nil {
		log.Print(err)
	}
	a.stop()
	log.Print("Volm stopped")
}

// runningMutex exists while the resident process runs, for the installer's AppMutex.
const runningMutex = `Local\VolmRunning`

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

const updateNotificationID = "volm-update"

// checkForUpdateAtStart looks for a newer release at most once a day, when the settings allow it, and tells the user with a notification that opens the settings window.
func checkForUpdateAtStart(configPath string, settings config.Update) {
	if !settings.CheckOnStartup || !update.DueForCheck(settings.LastCheckedAt, time.Now()) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	rel, err := update.Latest(ctx, settings.Prerelease)
	switch {
	case errors.Is(err, update.ErrNoRelease):
	case err != nil:
		log.Printf("checking for updates: %v", err)
		return // try again at the next start rather than in a day
	}
	recordUpdateCheck(configPath)
	current, verErr := update.ParseVersion(mygo.App.Version())
	if err != nil || verErr != nil || rel.Version.Compare(current) <= 0 {
		return
	}
	n := mygo.NewNotification(mygo.NotificationOptions{
		ID:    updateNotificationID,
		Title: "Volm " + rel.Version.String() + " があります",
		Body:  "クリックすると設定画面を開きます。「更新」のページから更新できます。",
	})
	if err := n.Show(); err != nil {
		log.Printf("notifying update: %v", err)
	}
}

// recordUpdateCheck writes the time of the check into the settings file, reading it again first so that changes made since start are kept.
func recordUpdateCheck(configPath string) {
	cfg, err := config.Read(configPath)
	if err != nil {
		log.Printf("recording the update check: %v", err)
		return
	}
	cfg.Update.LastCheckedAt = time.Now().UTC()
	if err := config.Save(configPath, cfg); err != nil {
		log.Printf("recording the update check: %v", err)
	}
}
