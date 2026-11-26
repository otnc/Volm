// Copyright (C) 2026 otoneko.
// SPDX-License-Identifier: GPL-3.0-only

// Package appid works out which app a process belongs to, by its executable and its AppUserModelID (AUMID), the ID by which the taskbar groups windows into one button.
package appid

import (
	"strings"
	"unsafe"

	"github.com/go-ole/go-ole"
	"golang.org/x/sys/windows"
)

var (
	kernel32                      = windows.NewLazySystemDLL("kernel32.dll")
	procGetApplicationUserModelID = kernel32.NewProc("GetApplicationUserModelId")
)

// Target is an app as the taskbar shows it: its AUMID, and the executables of the processes whose windows carry that AUMID.
type Target struct {
	AUMID string
	// Executables holds full paths in lower case.
	Executables map[string]bool
}

// NewTarget returns the target of a taskbar button's AUMID. A Win32 app without an AUMID of its own has its executable's path for one, possibly with a known folder's GUID in place of the folder (see ExpandKnownFolder), and that executable belongs to the target too.
func NewTarget(aumid string, executables map[string]bool) Target {
	t := Target{AUMID: aumid, Executables: map[string]bool{}}
	for exe := range executables {
		t.Executables[strings.ToLower(exe)] = true
	}
	if path := ExpandKnownFolder(aumid); strings.Contains(path, `\`) {
		t.Executables[strings.ToLower(path)] = true
	}
	return t
}

// Matches reports whether a process with the given executable and AUMID plays the target's sound. A browser plays sound from a child process, not from the one that owns the window, so processes are compared by executable rather than by ID: every process of the same executable matches.
func (t Target) Matches(exe, aumid string) bool {
	if aumid != "" && strings.EqualFold(aumid, t.AUMID) {
		return true
	}
	return exe != "" && t.Executables[strings.ToLower(exe)]
}

// ExpandKnownFolder turns the form the taskbar gives Win32 apps without an AUMID, {known folder GUID}\relative path, into an absolute path. Other AUMIDs come back unchanged.
//
//	appid.ExpandKnownFolder(`{6D809377-6AF0-444B-8957-A3773F02200E}\VideoLAN\VLC\vlc.exe`)
//	// C:\Program Files\VideoLAN\VLC\vlc.exe
func ExpandKnownFolder(aumid string) string {
	if !strings.HasPrefix(aumid, "{") {
		return aumid
	}
	guid, rest, ok := strings.Cut(aumid, "}")
	if !ok {
		return aumid
	}
	id := ole.NewGUID(guid + "}")
	if id == nil {
		return aumid
	}
	folder, err := windows.KnownFolderPath((*windows.KNOWNFOLDERID)(unsafe.Pointer(id)), 0)
	if err != nil {
		return aumid
	}
	return folder + rest
}

// ProcessIdentity returns the full path of a process's executable and, for a packaged app, its AUMID. Either is empty when Windows does not tell, as for a process of another user.
func ProcessIdentity(pid uint32) (exe, aumid string) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return "", ""
	}
	defer func() { _ = windows.CloseHandle(h) }()
	buf := make([]uint16, windows.MAX_LONG_PATH)
	size := uint32(len(buf))
	if windows.QueryFullProcessImageName(h, 0, &buf[0], &size) == nil {
		exe = windows.UTF16ToString(buf[:size])
	}
	size = uint32(len(buf))
	if r, _, _ := procGetApplicationUserModelID.Call(uintptr(h), uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&buf[0]))); r == 0 {
		aumid = windows.UTF16ToString(buf[:size])
	}
	return exe, aumid
}
