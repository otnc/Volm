// Copyright (C) 2026 otoneko.
// SPDX-License-Identifier: GPL-3.0-only

package hook

import (
	"unsafe"

	"github.com/otnc/Volm/internal/hotkey"
	"github.com/otnc/Volm/internal/win32"
	"golang.org/x/sys/windows"
)

type kbdllHookStruct struct {
	VkCode    uint32
	ScanCode  uint32
	Flags     uint32
	Time      uint32
	ExtraInfo uintptr
}

const (
	wmKeyDown    = 0x0100
	wmKeyUp      = 0x0101
	wmSysKeyDown = 0x0104
	wmSysKeyUp   = 0x0105
)

// Keyboard is a running low-level keyboard hook.
type Keyboard struct {
	threadID uint32
}

// Lookup returns the action of a combination, inside the hook callback. It must return at once.
type Lookup func(hotkey.Combo) (hotkey.Action, bool)

// KeyEvent is a hotkey the hook took.
type KeyEvent struct {
	Action hotkey.Action
	Combo  hotkey.Combo
}

// StartKeyboard installs a hook that takes every key press whose combination lookup knows, and its release, from all other apps, and sends the action to out. Key repeats while held send the action again. Input that Volm sends itself (win32.InputMarker) passes through.
//
// Windows itself handles Win+Up and Win+Down before RegisterHotKey sees them, which is why Volm uses a low-level hook. After an action whose combination holds Win or Alt, the caller sends win32.SendMaskKey, or the lone Win opens the Start menu.
func StartKeyboard(lookup Lookup, out chan<- KeyEvent) (*Keyboard, error) {
	h := &keyboardHook{taken: map[uint32]bool{}}
	h.handle = func(vk uint32, mods hotkey.Modifiers) bool {
		c := hotkey.Combo{Mods: mods, Key: uint16(vk)}
		a, ok := lookup(c)
		if !ok {
			return false
		}
		select {
		case out <- KeyEvent{Action: a, Combo: c}:
		default:
		}
		return true
	}
	return h.start()
}

// Capture installs a hook that takes every key pressed while it runs, and sends the combination of the first one with any modifiers, other than a modifier key itself, to out, which must have room for it. Modifier keys pass through, so that the caller sees them held and can send win32.SendMaskKey. Stop it when done; it captures only the first key.
//
// The settings window uses it to record a hotkey: a hook installed later is called first, so it sees the keys before the resident process's hotkeys do.
func Capture(out chan<- hotkey.Combo) (*Keyboard, error) {
	h := &keyboardHook{taken: map[uint32]bool{}}
	sent := false
	h.handle = func(vk uint32, mods hotkey.Modifiers) bool {
		if !sent {
			sent = true
			select {
			case out <- hotkey.Combo{Mods: mods, Key: uint16(vk)}:
			default: // out must be buffered; the callback cannot wait
			}
		}
		return true
	}
	return h.start()
}

// Stop removes the hook.
func (k *Keyboard) Stop() { win32.QuitThread(k.threadID) }

// keyboardHook's fields are only touched by its hook thread, apart from handle, set before the hook starts.
type keyboardHook struct {
	handle func(vk uint32, mods hotkey.Modifiers) bool
	// taken holds the keys whose press the hook withheld, so that their release is withheld too.
	taken map[uint32]bool
}

func (h *keyboardHook) start() (*Keyboard, error) {
	id, err := win32.StartThread(func() error {
		r, _, err := win32.ProcSetWindowsHookExW.Call(win32.WH_KEYBOARD_LL, windows.NewCallback(h.callback), 0, 0)
		if r == 0 {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &Keyboard{threadID: id}, nil
}

func (h *keyboardHook) callback(code int32, wParam, lParam uintptr) uintptr {
	if code >= 0 {
		info := (*kbdllHookStruct)(unsafe.Pointer(lParam))
		// Input that Volm sent itself passes, so that a media key it sends never runs a hotkey bound to that key. Input other apps inject, as remote desktop tools do, is handled like typing.
		if info.ExtraInfo != win32.InputMarker && h.takes(wParam, info.VkCode) {
			return 1
		}
	}
	r, _, _ := win32.ProcCallNextHookEx.Call(0, uintptr(code), wParam, lParam)
	return r
}

func (h *keyboardHook) takes(message uintptr, vk uint32) bool {
	switch message {
	case wmKeyUp, wmSysKeyUp:
		taken := h.taken[vk]
		delete(h.taken, vk)
		return taken
	case wmKeyDown, wmSysKeyDown:
		if hotkey.IsModifierKey(uint16(vk)) {
			return false
		}
		if h.handle(vk, heldModifiers()) {
			h.taken[vk] = true
			return true
		}
	}
	return false
}

func heldModifiers() hotkey.Modifiers {
	var m hotkey.Modifiers
	if win32.KeyDown(win32.VK_CONTROL) {
		m |= hotkey.Ctrl
	}
	if win32.KeyDown(win32.VK_MENU) {
		m |= hotkey.Alt
	}
	if win32.KeyDown(win32.VK_SHIFT) {
		m |= hotkey.Shift
	}
	if win32.KeyDown(win32.VK_LWIN) || win32.KeyDown(win32.VK_RWIN) {
		m |= hotkey.Win
	}
	return m
}
