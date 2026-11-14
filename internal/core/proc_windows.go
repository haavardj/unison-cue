// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Håvard Dagenborg

package core

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
)

const createNoWindow = 0x08000000

func hiddenAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
}

// prepareCmd keeps a console window from flashing up for every run.
// Cancelling kills the whole process tree: Windows does not end child
// processes with their parent, so the ssh that unison started for a
// remote root would otherwise stay connected.
func prepareCmd(cmd *exec.Cmd) {
	cmd.SysProcAttr = hiddenAttr()
	cmd.Cancel = func() error { return killTree(cmd.Process) }
}

// killTree ends p and its descendants. taskkill finds the children
// through their parent process ID, so it must run while p is alive,
// which holds for cmd.Cancel.
func killTree(p *os.Process) error {
	root := os.Getenv("SystemRoot")
	if root == "" {
		root = `C:\Windows`
	}
	tk := exec.Command(filepath.Join(root, "System32", "taskkill.exe"), "/T", "/F", "/PID", strconv.Itoa(p.Pid))
	tk.SysProcAttr = hiddenAttr()
	if tk.Run() != nil {
		return p.Kill()
	}
	return nil
}
