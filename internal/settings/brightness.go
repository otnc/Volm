// Copyright (C) 2026 otoneko.
// SPDX-License-Identifier: GPL-3.0-only

package settings

import (
	"fmt"
	"log"
	"maps"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/otnc/Volm/internal/brightness"
	"github.com/otnc/Volm/internal/config"
)

// methodLabels names the brightness methods in the menu of each monitor.
var methodLabels = []struct {
	method brightness.Method
	label  string
}{
	{brightness.MethodAuto, "自動"},
	{brightness.MethodWMI, "WMI"},
	{brightness.MethodDDC, "DDC/CI"},
	{brightness.MethodSoftDim, "画面を暗く重ねる"},
	{brightness.MethodDisabled, "使わない"},
}

func methodLabel(m brightness.Method) string {
	for _, l := range methodLabels {
		if l.method == m {
			return l.label
		}
	}
	return string(m)
}

// detectMonitors looks at the monitors on a goroutine, as monitors that do not answer DDC/CI take a while, and shows them when done.
func (w *window) detectMonitors(win *mygo.Window) {
	w.detecting = true
	go func() {
		displays, err := brightness.Detect()
		if err != nil {
			log.Printf("settings: detecting monitors: %v", err)
		}
		win.Update(func() {
			w.displays = displays
			w.detecting = false
			w.methodChoice = map[string]string{}
			for _, d := range displays {
				method := brightness.Method(w.cfg.Brightness.Monitors[d.ID].Method)
				if method == "" {
					method = brightness.MethodAuto
				}
				w.methodChoice[d.ID] = methodLabel(method)
			}
		})
	}()
}

func (w *window) brightnessPage(c *ui.Context) {
	t := c.Theme()
	def := config.Default()
	pageTitle(c, "明るさ")
	row(c, w.icons.mouse, "ホイール 1 段で変わる明るさ", "Ctrl を押しながらタスクバーの上で回します。", func() {
		if ui.StepSlider(c, &w.brightnessStep, config.MinStep, config.MaxStep, 1).Width(140).Label("ホイール 1 段で変わる明るさ").Changed() {
			w.cfg.Brightness.Step = int(w.brightnessStep)
			w.save()
		}
		ui.Text(c, fmt.Sprintf("%d%%", int(w.brightnessStep))).Width(44).TextAlign(ui.End)
		w.resetButton(c, "ホイール 1 段で変わる明るさ", w.cfg.Brightness.Step == def.Brightness.Step, func() { w.cfg.Brightness.Step = def.Brightness.Step })
	})
	row(c, w.icons.brightnessLow, "暗く重ねるときの下限", "画面を暗く重ねる方法で、これより暗くしません。", func() {
		if ui.StepSlider(c, &w.softDimMin, 5, 100, 5).Width(140).Label("暗く重ねるときの下限").Changed() {
			w.cfg.Brightness.SoftDimMin = int(w.softDimMin)
			w.save()
		}
		ui.Text(c, fmt.Sprintf("%d%%", int(w.softDimMin))).Width(44).TextAlign(ui.End)
		w.resetButton(c, "暗く重ねるときの下限", w.cfg.Brightness.SoftDimMin == def.Brightness.SoftDimMin, func() {
			w.cfg.Brightness.SoftDimMin = def.Brightness.SoftDimMin
		})
	})

	ui.Text(c, "モニターごとの方法").FontSize(16).Bold().Margin(12, 0, 0, 0)
	ui.Text(c, "自動では、内蔵ディスプレイは WMI、DDC/CI に応える外部モニターは DDC/CI、どちらでもないモニターは画面を暗く重ねる方法を使います。").FontSize(12).TextColor(t.TextMuted).Margin(0, 0, 4, 0)
	if w.detecting {
		ui.Text(c, "モニターを調べています…").TextColor(t.TextMuted)
		return
	}
	if len(w.displays) == 0 {
		ui.Text(c, "モニターが見つかりませんでした。").TextColor(t.TextMuted)
		return
	}
	labels := make([]string, len(methodLabels))
	for i, l := range methodLabels {
		labels[i] = l.label
	}
	for _, d := range w.displays {
		description := "自動で選ばれる方法: " + methodLabel(d.Detected)
		if d.Primary {
			description = "メインのモニター。" + description
		}
		choice := w.methodChoice[d.ID]
		row(c, w.icons.monitor, d.Name, description, func() {
			if ui.Select(c, &choice, labels).Width(170).Label(d.Name + " の明るさを変える方法").Changed() {
				w.methodChoice[d.ID] = choice
				w.setMonitorMethod(d.ID, choice)
			}
			auto := methodLabel(brightness.MethodAuto)
			w.resetButton(c, d.Name+" の明るさを変える方法", choice == auto, func() {
				w.methodChoice[d.ID] = auto
				w.setMonitorMethod(d.ID, auto)
			})
		})
	}
}

func (w *window) setMonitorMethod(id, label string) {
	method := brightness.MethodAuto
	for _, l := range methodLabels {
		if l.label == label {
			method = l.method
		}
	}
	// A copy, so that the map in w.cfg is never shared with a Config saved earlier.
	monitors := maps.Clone(w.cfg.Brightness.Monitors)
	if monitors == nil {
		monitors = map[string]config.Monitor{}
	}
	monitors[id] = config.Monitor{Method: string(method)}
	w.cfg.Brightness.Monitors = monitors
	w.save()
}
