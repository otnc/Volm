// Copyright (C) 2026 otoneko.
// SPDX-License-Identifier: GPL-3.0-only

package brightness

import (
	"errors"
	"log"
	"sync"
	"sync/atomic"

	"github.com/otnc/Volm/internal/config"
	"github.com/otnc/Volm/internal/control"
	"github.com/otnc/Volm/internal/win32"
)

// Method is how a monitor's brightness changes, as config.Monitor.Method names it.
type Method string

// The methods, in the order Detect tries them.
const (
	MethodAuto     Method = "auto"
	MethodWMI      Method = "wmi"
	MethodDDC      Method = "ddcci"
	MethodSoftDim  Method = "softDim"
	MethodDisabled Method = "disabled"
)

// Display is a monitor with what Detect found out about it.
type Display struct {
	Monitor
	// Detected is the method Detect would use: MethodWMI, MethodDDC or MethodSoftDim.
	Detected Method
	// Percent is the brightness Detect read, or 100 for a monitor without hardware control.
	Percent int
	ddc     ddcRange
}

// Detect lists the monitors and works out, for each, how its brightness can change: WMI for a laptop's built-in panel, DDC/CI for an external monitor that answers it, else the dimming overlay. A monitor that does not answer DDC/CI takes about 250ms; monitors are asked in parallel.
//
//	displays, err := brightness.Detect()
//	for _, d := range displays {
//		fmt.Println(d.Name, d.Detected, d.Percent)
//	}
func Detect() ([]Display, error) {
	w, err := openWMI()
	if err != nil {
		return nil, err
	}
	defer w.close()
	return detect(w)
}

func detect(w *wmiClient) ([]Display, error) {
	names, namesErr := w.names()
	levels, levelsErr := w.levels()
	monitors := Enumerate(names)
	displays := make([]Display, len(monitors))
	var wg sync.WaitGroup
	for i, m := range monitors {
		displays[i] = Display{Monitor: m, Detected: MethodSoftDim, Percent: 100}
		if level, ok := levels[WMIKey(m.ID)]; ok {
			displays[i].Detected, displays[i].Percent = MethodWMI, level
			continue
		}
		wg.Go(func() {
			if r, err := ddcRead(m.Handle); err == nil {
				displays[i].Detected, displays[i].Percent, displays[i].ddc = MethodDDC, r.percent(), r
			}
		})
	}
	wg.Wait()
	return displays, errors.Join(namesErr, levelsErr)
}

// EffectiveMethod is the method used for a monitor: the configured one, or for MethodAuto (and an unknown value) the detected one.
func EffectiveMethod(configured string, detected Method) Method {
	switch m := Method(configured); m {
	case MethodWMI, MethodDDC, MethodSoftDim, MethodDisabled:
		return m
	default:
		return detected
	}
}

// stepTarget moves a brightness by notches steps of step percent. The dimming overlay stops at softDimMin, so that the screen never goes too dark to use.
func stepTarget(current, notches, step int, method Method, softDimMin int) int {
	target := control.StepPercent(current, notches, step)
	if method == MethodSoftDim {
		target = max(target, softDimMin)
	}
	return target
}

// Change is the outcome of a step on one monitor, for the OSD.
type Change struct {
	Name    string
	Percent int
	Method  Method
}

// ErrDetecting reports that the monitors are still being looked at, after start or a display change.
var ErrDetecting = errors.New("brightness: monitors are still being detected")

// Manager changes the brightness of the monitors from the resident process.
type Manager struct {
	wmi      *wmiClient
	overlays *overlays

	mu       sync.Mutex
	displays []Display // nil while detecting
	percent  map[string]int
	writers  map[string]*writer
}

// Start detects the monitors in the background and does so again after every display change.
func Start() (*Manager, error) {
	w, err := openWMI()
	if err != nil {
		return nil, err
	}
	m := &Manager{wmi: w, percent: map[string]int{}, writers: map[string]*writer{}}
	m.overlays, err = startOverlays(func() { go m.redetect() })
	if err != nil {
		w.close()
		return nil, err
	}
	go m.redetect()
	return m, nil
}

// Close removes the overlays and stops the threads.
func (m *Manager) Close() {
	m.overlays.close()
	m.wmi.close()
}

func (m *Manager) redetect() {
	m.mu.Lock()
	m.displays = nil
	m.mu.Unlock()
	m.overlays.clearAll()

	displays, err := detect(m.wmi)
	if err != nil {
		log.Printf("brightness: detecting: %v", err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.displays = displays
	// Every overlay was removed above, so a dimmed monitor is back at 100%, which is what Detect reports for it.
	for _, d := range displays {
		m.percent[d.ID] = d.Percent
	}
	log.Printf("brightness: %d monitors", len(displays))
}

// Step changes the brightness of the monitor under at, or of every monitor when all is set, by notches steps of the configured step. The monitor under at comes first in the result. A monitor whose method is disabled is left alone and not reported.
func (m *Manager) Step(at win32.Point, all bool, notches int, cfg config.Brightness) ([]Change, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.displays == nil {
		return nil, ErrDetecting
	}
	under, ok := MonitorAt(monitorsOf(m.displays), at)
	if !ok {
		return nil, nil
	}
	// The monitor under the cursor comes first, so that the OSD shows its level when all change.
	var targets []Display
	for _, d := range m.displays {
		if d.ID == under.ID {
			targets = append([]Display{d}, targets...)
		} else if all {
			targets = append(targets, d)
		}
	}
	var changes []Change
	for _, d := range targets {
		method := EffectiveMethod(cfg.Monitors[d.ID].Method, d.Detected)
		if method == MethodDisabled {
			continue
		}
		p := stepTarget(m.percent[d.ID], notches, cfg.Step, method, cfg.SoftDimMin)
		m.percent[d.ID] = p
		m.write(d, method, p)
		changes = append(changes, Change{Name: d.Name, Percent: p, Method: method})
	}
	return changes, nil
}

// ApplyConfig removes the overlay of every monitor that no longer uses the dimming overlay, after the settings change.
func (m *Manager) ApplyConfig(cfg config.Brightness) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, d := range m.displays {
		if EffectiveMethod(cfg.Monitors[d.ID].Method, d.Detected) != MethodSoftDim {
			m.overlays.setDim(d.ID, d.Bounds, 100)
		}
	}
}

func monitorsOf(ds []Display) []Monitor {
	ms := make([]Monitor, len(ds))
	for i, d := range ds {
		ms[i] = d.Monitor
	}
	return ms
}

// write hands the target to the monitor's writer. Called with m.mu held.
func (m *Manager) write(d Display, method Method, percent int) {
	if method == MethodSoftDim {
		m.overlays.setDim(d.ID, d.Bounds, percent)
		return
	}
	// Leaving the overlay on would stack it with the hardware change.
	m.overlays.setDim(d.ID, d.Bounds, 100)
	w := m.writers[d.ID]
	if w == nil {
		w = newWriter()
		m.writers[d.ID] = w
	}
	w.set(func() error {
		switch method {
		case MethodWMI:
			return m.wmi.set(WMIKey(d.ID), percent)
		case MethodDDC:
			return ddcWrite(d.Handle, d.ddc.value(percent))
		}
		return nil
	})
}

// writer applies the latest of a monitor's writes on a goroutine of its own. DDC/CI takes tens of milliseconds per write and some monitors drop writes that come too fast, so a quick wheel only updates the pending write, and the writer catches up with the last one.
type writer struct {
	pending atomic.Pointer[func() error]
	wake    chan struct{}
}

func newWriter() *writer {
	w := &writer{wake: make(chan struct{}, 1)}
	go w.run()
	return w
}

func (w *writer) set(fn func() error) {
	w.pending.Store(&fn)
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

func (w *writer) run() {
	for range w.wake {
		fn := w.pending.Swap(nil)
		if fn == nil {
			continue
		}
		if err := (*fn)(); err != nil {
			log.Printf("brightness: writing: %v", err)
		}
	}
}
