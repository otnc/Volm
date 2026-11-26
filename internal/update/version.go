// Copyright (C) 2026 otoneko.
// SPDX-License-Identifier: GPL-3.0-only

// Package update finds a newer Volm on GitHub Releases, downloads its installer and runs it.
package update

import (
	"cmp"
	"fmt"
	"strconv"
	"strings"
)

// Version is a semantic version such as 1.2.3 or 1.3.0-rc.1.
type Version struct {
	Major, Minor, Patch int
	// Pre is the prerelease part after "-", split at dots; empty for a release.
	Pre []string
}

// ParseVersion reads a version, with or without a leading "v".
func ParseVersion(s string) (Version, error) {
	s = strings.TrimPrefix(s, "v")
	core, pre, hasPre := strings.Cut(s, "-")
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return Version{}, fmt.Errorf("update: %q is not a version", s)
	}
	var nums [3]int
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return Version{}, fmt.Errorf("update: %q is not a version", s)
		}
		nums[i] = n
	}
	v := Version{Major: nums[0], Minor: nums[1], Patch: nums[2]}
	if hasPre {
		if pre == "" {
			return Version{}, fmt.Errorf("update: %q has an empty prerelease", s)
		}
		v.Pre = strings.Split(pre, ".")
	}
	return v, nil
}

// Compare orders versions as semantic versioning does: a prerelease comes before its release, and numeric prerelease parts compare as numbers.
func (v Version) Compare(w Version) int {
	if c := cmp.Compare(v.Major, w.Major); c != 0 {
		return c
	}
	if c := cmp.Compare(v.Minor, w.Minor); c != 0 {
		return c
	}
	if c := cmp.Compare(v.Patch, w.Patch); c != 0 {
		return c
	}
	switch {
	case len(v.Pre) == 0 && len(w.Pre) == 0:
		return 0
	case len(v.Pre) == 0:
		return 1
	case len(w.Pre) == 0:
		return -1
	}
	for i := range min(len(v.Pre), len(w.Pre)) {
		if c := comparePre(v.Pre[i], w.Pre[i]); c != 0 {
			return c
		}
	}
	return cmp.Compare(len(v.Pre), len(w.Pre))
}

func comparePre(a, b string) int {
	an, aErr := strconv.Atoi(a)
	bn, bErr := strconv.Atoi(b)
	switch {
	case aErr == nil && bErr == nil:
		return cmp.Compare(an, bn)
	case aErr == nil:
		return -1 // numeric identifiers sort before alphanumeric ones
	case bErr == nil:
		return 1
	}
	return cmp.Compare(a, b)
}

func (v Version) String() string {
	s := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if len(v.Pre) > 0 {
		s += "-" + strings.Join(v.Pre, ".")
	}
	return s
}
