// Copyright (C) 2026 otoneko.
// SPDX-License-Identifier: GPL-3.0-only

package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Volume.Step != 2 || cfg.Brightness.Step != 5 || !cfg.OSD.Enabled {
		t.Fatalf("got %+v, want the defaults", cfg)
	}
}

func TestSaveThenLoadKeepsChanges(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Volm", "config.json")
	cfg := Default()
	cfg.Volume.Step = 7
	cfg.Volume.MiddleClickMute = false
	cfg.OSD.Position = PositionTopLeft
	cfg.Hotkeys["playPause"] = []string{}
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Volume.Step != 7 || got.Volume.MiddleClickMute || got.OSD.Position != PositionTopLeft {
		t.Fatalf("got %+v", got)
	}
	if len(got.Hotkeys["playPause"]) != 0 {
		t.Fatalf("a cleared hotkey came back as %v", got.Hotkeys["playPause"])
	}
	if leftovers, _ := filepath.Glob(filepath.Join(filepath.Dir(path), "*.tmp")); len(leftovers) != 0 {
		t.Fatalf("temporary files left behind: %v", leftovers)
	}
}

// TestOptInFeaturesStartOff pins the features the author chose to leave off on a fresh install.
func TestOptInFeaturesStartOff(t *testing.T) {
	if Default().PauseInFullscreen {
		t.Error("pausing in full screen is on by default")
	}
	if Default().Volume.Acceleration {
		t.Error("wheel acceleration is on by default")
	}
}

func TestLoadReadsPauseInFullscreen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	writeFile(t, path, `{"pauseInFullscreen":true}`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.PauseInFullscreen {
		t.Fatal("pauseInFullscreen in the file was not read")
	}
}

func TestLoadKeepsDefaultsForMissingFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	writeFile(t, path, `{"version":1,"volume":{"step":4}}`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Volume.Step != 4 {
		t.Fatalf("volume step = %d, want 4 from the file", cfg.Volume.Step)
	}
	if !cfg.Volume.UnmuteOnIncrease || cfg.Brightness.Step != 5 || cfg.OSD.DurationMs != 1500 {
		t.Fatalf("missing fields lost their defaults: %+v", cfg)
	}
}

func TestLoadClampsOutOfRangeValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	writeFile(t, path, `{"volume":{"step":99},"brightness":{"step":-3,"softDimMin":1},"osd":{"position":"middle","durationMs":5}}`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name      string
		got, want any
	}{
		{"volume step above max", cfg.Volume.Step, MaxStep},
		{"brightness step below min", cfg.Brightness.Step, MinStep},
		{"soft dim floor", cfg.Brightness.SoftDimMin, 5},
		{"unknown position", cfg.OSD.Position, PositionBottomRight},
		{"too short duration", cfg.OSD.DurationMs, 300},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, tt.got, tt.want)
		}
	}
}

func TestLoadBrokenFileMovesItAsideAndReturnsDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	writeFile(t, path, `{"volume": {"step": 4`)
	cfg, err := Load(path)
	if !errors.Is(err, ErrRecovered) {
		t.Fatalf("err = %v, want ErrRecovered", err)
	}
	if cfg.Volume.Step != Default().Volume.Step {
		t.Fatalf("got step %d, want the default", cfg.Volume.Step)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the broken file is still at %s", path)
	}
	backup, err := os.ReadFile(path + ".bak")
	if err != nil || string(backup) != `{"volume": {"step": 4` {
		t.Fatalf("backup = %q, %v", backup, err)
	}
}

func TestReadBrokenFileLeavesItInPlace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	writeFile(t, path, `{"volume": {"step": 4`)
	if _, err := Read(path); !errors.Is(err, ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the file was moved or removed: %v", err)
	}
	if _, err := os.Stat(path + ".bak"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Read created a backup")
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
