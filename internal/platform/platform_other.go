//go:build !darwin && !windows

// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Håvard Dagenborg

// Linux and other Unix builds are not a target, but keeping them
// compiling makes development and CI on Linux easier.
package platform

import (
	"os/exec"

	"github.com/haavardj/unison-cue/internal/icon"
)

func TrayIcon(s icon.State, frame int) (template, regular []byte) {
	b := cachedIcon(s, frame, func() []byte { return icon.PNG(s, frame, 48, false) })
	return b, b
}

const UsesTemplateIcons = false

func Notify(title, body string) error          { return exec.Command("notify-send", title, body).Run() }
func Open(path string) error                   { return exec.Command("xdg-open", path).Start() }
func EditText(path string) error               { return exec.Command("xdg-open", path).Start() }
func AutostartEnabled() bool                   { return false }
func SetAutostart(on bool) error               { return nil }
func SingleInstance(dir string) (bool, func()) { return true, func() {} }
func MigrateLegacy()                           {}
func RunInTerminal(u, p string) error          { return exec.Command("x-terminal-emulator", "-e", u, p).Start() }

// AskPass is not implemented here; returning 1 makes ssh fail the login.
func AskPass(prompt, mode string) int { return 1 }
