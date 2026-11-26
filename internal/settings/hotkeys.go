// Copyright (C) 2026 otoneko.
// SPDX-License-Identifier: GPL-3.0-only

package settings

import (
	"log"
	"maps"
	"slices"

	"github.com/egoist/mygo/ui"
	"github.com/otnc/Volm/internal/config"
	"github.com/otnc/Volm/internal/hook"
	"github.com/otnc/Volm/internal/hotkey"
	"github.com/otnc/Volm/internal/win32"
)

const vkEscape = 0x1B

// hotkeyEditor is the state of the hotkeys page.
type hotkeyEditor struct {
	// capturing is the action waiting for a key, or "" when none is.
	capturing hotkey.Action
	// notice tells why the last captured key was refused.
	notice string
	// pending is a captured combination that another action already uses, waiting for the user to confirm taking it over.
	pending         hotkey.Combo
	pendingFor      hotkey.Action
	pendingFrom     hotkey.Action
	confirmOverride bool
}

// startCapture records the next key combination for the action. It installs a low-level hook in this process, which Windows calls before the resident process's hook, so that a combination already bound to an action is recorded instead of run.
func (w *window) startCapture(action hotkey.Action) {
	combos := make(chan hotkey.Combo, 1)
	kb, err := hook.Capture(combos)
	if err != nil {
		log.Printf("settings: capturing a key: %v", err)
		w.errorText = "キーを読み取れませんでした。" + err.Error()
		return
	}
	w.hotkeys.capturing = action
	w.hotkeys.notice = ""
	go func() {
		c := <-combos
		kb.Stop()
		// The modifiers are still held; see win32.SendMaskKey.
		if c.Mods&(hotkey.Win|hotkey.Alt) != 0 {
			win32.SendMaskKey()
		}
		w.win.Update(func() { w.captured(action, c) })
	}()
}

func (w *window) captured(action hotkey.Action, c hotkey.Combo) {
	w.hotkeys.capturing = ""
	if c.Mods == 0 && c.Key == vkEscape {
		return
	}
	if c.NeedsModifier() {
		w.hotkeys.notice = "「" + c.String() + "」だけでは割り当てられません。このキーはほかのアプリでも使うので、Ctrl、Alt、Shift、Win のどれかと一緒に押してください。"
		return
	}
	w.hotkeys.notice = ""
	if other, taken := hotkey.Conflict(w.cfg.Hotkeys, c, action); taken {
		w.hotkeys.pending, w.hotkeys.pendingFor, w.hotkeys.pendingFrom = c, action, other
		w.hotkeys.confirmOverride = true
		return
	}
	w.addHotkey(action, c)
}

func (w *window) addHotkey(action hotkey.Action, c hotkey.Combo) {
	keys := maps.Clone(w.cfg.Hotkeys)
	if keys == nil {
		keys = map[string][]string{}
	}
	// Take the combination away from every other action, then add it here unless it is already there.
	for a, list := range keys {
		keys[a] = slices.DeleteFunc(slices.Clone(list), func(s string) bool {
			parsed, err := hotkey.Parse(s)
			return err == nil && parsed == c && a != string(action)
		})
	}
	list := keys[string(action)]
	if !slices.ContainsFunc(list, func(s string) bool { p, err := hotkey.Parse(s); return err == nil && p == c }) && len(list) < hotkey.MaxKeysPerAction {
		list = append(slices.Clone(list), c.String())
	}
	keys[string(action)] = list
	w.cfg.Hotkeys = keys
	w.save()
}

// resetHotkey gives the action its initial keys back, taking them away from any other action that uses them now.
func (w *window) resetHotkey(action hotkey.Action) {
	keys := maps.Clone(w.cfg.Hotkeys)
	if keys == nil {
		keys = map[string][]string{}
	}
	// An empty list, not a missing key: Load would give a missing action its default keys back.
	keys[string(action)] = []string{}
	w.cfg.Hotkeys = keys
	for _, s := range config.Default().Hotkeys[string(action)] {
		if c, err := hotkey.Parse(s); err == nil {
			w.addHotkey(action, c)
		}
	}
}

func (w *window) removeHotkey(action hotkey.Action, index int) {
	keys := maps.Clone(w.cfg.Hotkeys)
	list := slices.Clone(keys[string(action)])
	if index < len(list) {
		list = slices.Delete(list, index, index+1)
	}
	// An empty list, not a missing key: Load would give a missing action its default keys back.
	keys[string(action)] = list
	w.cfg.Hotkeys = keys
	w.save()
}

func (w *window) hotkeysPage(c *ui.Context) {
	t := c.Theme()
	pageTitle(c, "ホットキー")
	ui.Text(c, "1 つの操作に、キーの組み合わせを 2 つまで割り当てられます。どのアプリが前面にあっても使えます。F13〜F24、メディアキー、拡張キーボードやマクロパッドの専用キーは、それだけでも割り当てられます。").FontSize(12).TextColor(t.TextMuted).Margin(0, 0, 4, 0)
	if w.hotkeys.notice != "" {
		ui.Text(c, w.hotkeys.notice).TextColor(t.Warning)
	}
	for _, a := range hotkey.Actions {
		keys := w.cfg.Hotkeys[string(a.Action)]
		row(c, w.icons.keyboard, a.Label, "", func() {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				if w.hotkeys.capturing == a.Action {
					ui.Text(c, "キーを押してください (Esc で取り消し)").FontSize(12).TextColor(t.Accent)
					return
				}
				for i, k := range keys {
					ui.Row(c).Gap(2).Padding(2, 4, 2, 10).Radius(6).Border(1, t.Border).AlignItems(ui.Center).Children(func() {
						ui.Text(c, k).FontSize(13)
						if ui.Button(c, "").Label(k + " の割り当てを外す").Padding(2).Children(func() {
							ui.Icon(c, w.icons.close).FontSize(14)
						}).Clicked() {
							w.removeHotkey(a.Action, i)
						}
					})
				}
				if w.hotkeys.capturing != "" {
					return
				}
				if len(keys) < hotkey.MaxKeysPerAction {
					if ui.Button(c, "").Label(a.Label + " にキーを割り当てる").Children(func() {
						ui.Icon(c, w.icons.add)
						ui.Text(c, "追加").SingleLine()
					}).Clicked() {
						w.startCapture(a.Action)
					}
				}
				defaults := config.Default().Hotkeys[string(a.Action)]
				w.resetButton(c, a.Label+" のキー", slices.Equal(keys, defaults), func() { w.resetHotkey(a.Action) })
			})
		})
	}
	h := &w.hotkeys
	if h.confirmOverride {
		if ui.AlertDialog(c, &h.confirmOverride,
			h.pending.String()+" はほかの操作に割り当て済みです",
			"「"+h.pendingFrom.Label()+"」から外して、「"+h.pendingFor.Label()+"」に割り当てますか？",
			"キャンセル", "割り当てる") == 1 {
			w.addHotkey(h.pendingFor, h.pending)
		}
	}
}
