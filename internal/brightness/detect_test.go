// Copyright (C) 2026 otoneko.
// SPDX-License-Identifier: GPL-3.0-only

package brightness

import (
	"os"
	"testing"
)

// TestDetectFindsEveryMonitor runs only with VOLM_INTEGRATION=1, as it asks the real monitors.
func TestDetectFindsEveryMonitor(t *testing.T) {
	if os.Getenv("VOLM_INTEGRATION") != "1" {
		t.Skip("set VOLM_INTEGRATION=1 to ask the real monitors")
	}
	displays, err := Detect()
	if err != nil {
		t.Fatal(err)
	}
	if len(displays) == 0 {
		t.Fatal("no monitors found")
	}
	primaries := 0
	for _, d := range displays {
		t.Logf("%q %s %d%% bounds=%+v", d.Name, d.Detected, d.Percent, d.Bounds)
		if d.ID == "" || d.Name == "" {
			t.Errorf("monitor without an ID or a name: %+v", d.Monitor)
		}
		if d.Percent < 0 || d.Percent > 100 {
			t.Errorf("%s: percent %d out of range", d.Name, d.Percent)
		}
		if d.Primary {
			primaries++
		}
	}
	if primaries != 1 {
		t.Errorf("%d primary monitors, want 1", primaries)
	}
}
