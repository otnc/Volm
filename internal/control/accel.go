// Copyright (C) 2026 otoneko.
// SPDX-License-Identifier: GPL-3.0-only

package control

import "time"

// The gaps between notches below which the wheel counts as turned quickly. A deliberate notch-by-notch turn leaves about 100 ms or more between notches; a flick leaves 10 to 30 ms.
const (
	fastGap  = 35 * time.Millisecond
	quickGap = 70 * time.Millisecond
)

// Accelerator speeds up the volume when the wheel turns quickly, so that a flick crosses the range without many turns while slow turning keeps its precision. The zero value is ready to use.
type Accelerator struct {
	last      time.Time
	direction int
}

// Factor returns how many times the usual step the notches reported at now should move: 4 within fastGap of the previous notch in the same direction, 2 within quickGap, else 1. A change of direction starts over at 1.
//
//	var a control.Accelerator
//	a.Factor(t0, 1)                           // 1
//	a.Factor(t0.Add(20*time.Millisecond), 1) // 4
func (a *Accelerator) Factor(now time.Time, notches int) int {
	direction := 1
	if notches < 0 {
		direction = -1
	}
	gap := now.Sub(a.last)
	sameDirection := !a.last.IsZero() && direction == a.direction
	a.last, a.direction = now, direction
	switch {
	case !sameDirection:
		return 1
	case gap < fastGap:
		return 4
	case gap < quickGap:
		return 2
	default:
		return 1
	}
}
