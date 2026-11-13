//go:build !windows

// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Håvard Dagenborg

package core

import (
	"os/exec"
	"syscall"
)

// prepareCmd puts unison in its own session, without a controlling
// terminal, so ssh cannot block on a password prompt and fails instead.
// Cancelling signals the whole process group, so the ssh child unison
// started for a remote root goes away too.
func prepareCmd(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
	}
}
