// Copyright (C) 2026 otoneko.
// SPDX-License-Identifier: GPL-3.0-only

// Package config reads and writes Volm's settings file, %APPDATA%\Volm\config.json.
package config

import (
	"encoding/json/v2"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// CurrentVersion is the settings format written by this build. Load migrates older formats up to it.
const CurrentVersion = 1

// Bounds of the per-notch step, in percent.
const (
	MinStep = 1
	MaxStep = 20
)

// OSD positions, relative to the work area of the monitor that was operated on.
const (
	PositionBottomRight  = "bottomRight"
	PositionBottomLeft   = "bottomLeft"
	PositionTopRight     = "topRight"
	PositionTopLeft      = "topLeft"
	PositionBottomCenter = "bottomCenter"
)

// Config is the whole settings file. Every field has a default (see Default), and fields missing from the file keep their defaults.
type Config struct {
	Version    int        `json:"version"`
	Volume     Volume     `json:"volume"`
	Brightness Brightness `json:"brightness"`
	OSD        OSD        `json:"osd"`
	// Hotkeys maps an action to up to two key combinations. Load merges the file over the defaults, so an action without keys must be stored as an empty list: a missing action gets its default keys back.
	Hotkeys map[string][]string `json:"hotkeys"`
	Update  Update              `json:"update"`
}

// Volume holds the settings of the wheel and middle click on the taskbar.
type Volume struct {
	Step             int  `json:"step"`
	UnmuteOnIncrease bool `json:"unmuteOnIncrease"`
	MiddleClickMute  bool `json:"middleClickMute"`
	AppVolumeWithAlt bool `json:"appVolumeWithAlt"`
}

// Brightness holds the settings of Ctrl+wheel on the taskbar.
type Brightness struct {
	Step int `json:"step"`
	// SoftDimMin is the darkest the dimming overlay may go, in percent, so that a monitor never turns too dark to use.
	SoftDimMin int `json:"softDimMin"`
	// Monitors is keyed by the monitor's device interface name.
	Monitors map[string]Monitor `json:"monitors"`
}

// Monitor holds the settings of one monitor.
type Monitor struct {
	// Method is "auto", "wmi", "ddcci", "softDim" or "disabled".
	Method string `json:"method"`
}

// OSD holds the settings of the on-screen display.
type OSD struct {
	Enabled    bool   `json:"enabled"`
	Position   string `json:"position"`
	DurationMs int    `json:"durationMs"`
}

// Update holds the settings of the update check.
type Update struct {
	CheckOnStartup bool      `json:"checkOnStartup"`
	Prerelease     bool      `json:"prerelease"`
	LastCheckedAt  time.Time `json:"lastCheckedAt,omitzero"`
}

// Default returns the settings of a fresh install.
func Default() Config {
	return Config{
		Version: CurrentVersion,
		Volume: Volume{
			Step:             2,
			UnmuteOnIncrease: true,
			MiddleClickMute:  true,
			AppVolumeWithAlt: true,
		},
		Brightness: Brightness{Step: 5, SoftDimMin: 20, Monitors: map[string]Monitor{}},
		OSD:        OSD{Enabled: true, Position: PositionBottomRight, DurationMs: 1500},
		Hotkeys: map[string][]string{
			"volumeUp":   {"Win+Up"},
			"volumeDown": {"Win+Down"},
			"playPause":  {"Ctrl+Shift+Q"},
			"nextTrack":  {"Ctrl+Shift+E"},
		},
		Update: Update{CheckOnStartup: true},
	}
}

// Path returns %APPDATA%\Volm\config.json.
func Path() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "Volm", "config.json"), nil
}

// ErrRecovered reports that the settings file could not be read, was moved aside to config.json.bak, and that Load returned the defaults instead.
var ErrRecovered = errors.New("config: settings file was unreadable and has been reset")

// ErrInvalid reports a settings file that is not valid JSON.
var ErrInvalid = errors.New("config: settings file is not valid")

// Read reads the settings at path and changes nothing on disk.
//
// A missing file yields Default with a nil error, and a file that is not valid JSON an error wrapping ErrInvalid. Values out of range are clamped rather than rejected.
//
//	cfg, err := config.Read(path)
//	if err != nil {
//		return err // keep the settings in use
//	}
func Read(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Default(), nil
	}
	if err != nil {
		return Default(), err
	}
	cfg := Default()
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Default(), fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	cfg.normalize()
	return cfg, nil
}

// Load reads the settings at path as Read does, for starting the app: a file that is not valid JSON is renamed to path+".bak", and Load returns Default together with an error wrapping ErrRecovered, so that the caller can tell the user.
//
//	cfg, err := config.Load(path)
//	if errors.Is(err, config.ErrRecovered) {
//		notifyUser(err)
//	} else if err != nil {
//		return err
//	}
func Load(path string) (Config, error) {
	cfg, err := Read(path)
	if !errors.Is(err, ErrInvalid) {
		return cfg, err
	}
	if renameErr := os.Rename(path, path+".bak"); renameErr != nil {
		return Default(), errors.Join(fmt.Errorf("%w: %w", ErrRecovered, err), renameErr)
	}
	return Default(), fmt.Errorf("%w: %w", ErrRecovered, err)
}

// Save writes cfg to path, creating its directory. It writes a temporary file first and renames it over path, so that a crash midway leaves the previous settings intact.
func Save(path string, cfg Config) error {
	cfg.Version = CurrentVersion
	data, err := json.Marshal(cfg, json.Deterministic(true))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "config-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func (c *Config) normalize() {
	def := Default()
	c.Volume.Step = clampOrDefault(c.Volume.Step, MinStep, MaxStep, def.Volume.Step)
	c.Brightness.Step = clampOrDefault(c.Brightness.Step, MinStep, MaxStep, def.Brightness.Step)
	c.Brightness.SoftDimMin = clampOrDefault(c.Brightness.SoftDimMin, 5, 100, def.Brightness.SoftDimMin)
	c.OSD.DurationMs = clampOrDefault(c.OSD.DurationMs, 300, 10000, def.OSD.DurationMs)
	switch c.OSD.Position {
	case PositionBottomRight, PositionBottomLeft, PositionTopRight, PositionTopLeft, PositionBottomCenter:
	default:
		c.OSD.Position = def.OSD.Position
	}
	if c.Brightness.Monitors == nil {
		c.Brightness.Monitors = map[string]Monitor{}
	}
	if c.Hotkeys == nil {
		c.Hotkeys = map[string][]string{}
	}
}

// clampOrDefault treats zero as "not set", since a step or duration of zero would make the feature do nothing.
func clampOrDefault(v, lo, hi, def int) int {
	if v == 0 {
		return def
	}
	return min(max(v, lo), hi)
}
