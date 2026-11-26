// Copyright (C) 2026 otoneko.
// SPDX-License-Identifier: GPL-3.0-only

package settings

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
	"golang.org/x/sys/windows/registry"
)

// uninstallKey is where the installer records the installation; its name is the installer's AppId with "_is1".
const uninstallKey = `Software\Microsoft\Windows\CurrentVersion\Uninstall\Volm_is1`

// findUninstaller returns the uninstaller of the installation this executable belongs to, or "" when Volm runs from somewhere else, as a build or the zip does.
func findUninstaller() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	k, err := registry.OpenKey(registry.CURRENT_USER, uninstallKey, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer func() { _ = k.Close() }()
	location, _, err := k.GetStringValue("InstallLocation")
	if err != nil {
		return ""
	}
	command, _, err := k.GetStringValue("UninstallString")
	if err != nil {
		return ""
	}
	return uninstallerFor(filepath.Dir(exe), location, command)
}

// uninstallerFor returns the uninstaller named by command, the quoted path the installer records, when exeDir is the installation's folder location.
func uninstallerFor(exeDir, location, command string) string {
	if location == "" || !strings.EqualFold(filepath.Clean(exeDir), filepath.Clean(location)) {
		return ""
	}
	return strings.Trim(strings.TrimSpace(command), `"`)
}

// uninstall starts the uninstaller and closes this window, whose process would otherwise keep Volm.exe from being deleted. The uninstaller asks for confirmation and quits the resident process itself.
func (w *window) uninstall(path string) {
	cmd := exec.Command(path)
	err := cmd.Start()
	if err == nil {
		err = cmd.Process.Release()
	}
	if err != nil {
		log.Printf("settings: starting the uninstaller: %v", err)
		w.errorText = "アンインストーラーを起動できませんでした。" + err.Error()
		return
	}
	mygo.App.Quit()
}
