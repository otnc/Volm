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
	wmKeyDown     = 0x0100
	wmKeyUp       = 0x0101
	wmSysKeyDown  = 0x0104
	wmSysKeyUp    = 0x0105
	llkhfExtended = 0x01
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
	h.handle = func(c hotkey.Combo) bool {
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
	h.handle = func(c hotkey.Combo) bool {
		if !sent {
			sent = true
			select {
			case out <- c:
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
	handle func(hotkey.Combo) bool
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
		if info.ExtraInfo != win32.InputMarker && h.takes(wParam, info) {
			return 1
		}
	}
	r, _, _ := win32.ProcCallNextHookEx.Call(0, uintptr(code), wParam, lParam)
	return r
}

func (h *keyboardHook) takes(message uintptr, info *kbdllHookStruct) bool {
	c := comboKey(info)
	if c.Key == 0 && c.Scan == 0 {
		return false
	}
	// Keys without a virtual-key code all share one, so the key is remembered with its scan code.
	id := uint32(c.Key)<<16 | uint32(c.Scan)
	switch message {
	case wmKeyUp, wmSysKeyUp:
		taken := h.taken[id]
		delete(h.taken, id)
		return taken
	case wmKeyDown, wmSysKeyDown:
		if hotkey.IsModifierKey(c.Key) {
			return false
		}
		c.Mods = heldModifiers()
		if h.handle(c) {
			h.taken[id] = true
			return true
		}
	}
	return false
}

// comboKey is the key of a key event, without modifiers. A key of an extra keyboard or a macro pad that Windows gives no virtual-key code (0, or 0xFF, "no mapping") is known by its scan code, with 0xE000 for an extended key.
func comboKey(info *kbdllHookStruct) hotkey.Combo {
	if info.VkCode != 0 && info.VkCode != 0xFF {
		return hotkey.Combo{Key: uint16(info.VkCode)}
	}
	scan := uint16(info.ScanCode)
	if info.Flags&llkhfExtended != 0 {
		scan |= 0xE000
	}
	return hotkey.Combo{Scan: scan}
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
