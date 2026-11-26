// Copyright (C) 2026 otoneko.
// SPDX-License-Identifier: GPL-3.0-only

// Package control turns raw input into volume and brightness changes.
package control

// WheelDelta is the delta Windows reports for one notch of a standard wheel.
const WheelDelta = 120

// NotchCounter adds up wheel deltas and reports whole notches, so that high-resolution wheels and touchpads, which send many deltas smaller than WheelDelta, change the volume by the same amount per notch as a standard wheel. The zero value is ready to use.
type NotchCounter struct {
	partial int
}

// Add returns how many whole notches delta completes: positive when rolled away from the user, negative toward. Turning back the other way discards the partial notch in the old direction, so a reversal takes effect after one notch in the new direction rather than after undoing the leftover.
//
//	var c NotchCounter
//	c.Add(40)  // 0
//	c.Add(40)  // 0
//	c.Add(40)  // 1
//	c.Add(-120) // -1
func (c *NotchCounter) Add(delta int) int {
	if (delta > 0 && c.partial < 0) || (delta < 0 && c.partial > 0) {
		c.partial = 0
	}
	c.partial += delta
	notches := c.partial / WheelDelta
	c.partial -= notches * WheelDelta
	return notches
}

// Reset drops any partial notch, as when the wheel moves to another target.
func (c *NotchCounter) Reset() { c.partial = 0 }

// StepPercent returns current moved by notches steps of step percent, within 0 to 100.
func StepPercent(current, notches, step int) int {
	return min(max(current+notches*step, 0), 100)
}
