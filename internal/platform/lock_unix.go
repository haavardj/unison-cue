//go:build !windows

// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Håvard Dagenborg

package platform

import (
	"os"
	"path/filepath"
	"syscall"
)

// SingleInstance takes an exclusive lock on a file in dir. On macOS the
// login agent starts the binary directly, bypassing LaunchServices, and
// on Linux nothing else prevents a second copy, so without the lock two
// instances could run side by side.
func SingleInstance(dir string) (ok bool, release func()) {
	f, err := os.OpenFile(filepath.Join(dir, ".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return true, func() {} // cannot tell; run anyway
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return false, func() {}
	}
	return true, func() { f.Close() }
}
