//go:build !darwin && !windows && !linux

// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Håvard Dagenborg

// Other Unix systems (the BSDs) are not a target; this keeps them
// compiling, with reduced functionality.
package platform

import (
	"os/exec"
	"path/filepath"

	"github.com/haavardj/unison-cue/internal/icon"
)

func TrayIcon(s icon.State, frame int) (template, regular []byte) {
	b := cachedIcon(s, frame, func() []byte { return icon.PNG(s, frame, 48, false) })
	return b, b
}

const UsesTemplateIcons = false

func Notify(title, body string) error { return exec.Command("notify-send", title, body).Run() }
func Open(path string) error          { return exec.Command("xdg-open", path).Start() }
func EditText(path string) error      { return exec.Command("xdg-open", path).Start() }
func AutostartEnabled() bool          { return false }
func SetAutostart(on bool) error      { return nil }
func MigrateLegacy()                  {}
func RunInTerminal(u, p string) error { return exec.Command("x-terminal-emulator", "-e", u, p).Start() }

// AskPass is not implemented here; returning 1 makes ssh fail the login.
func AskPass(prompt, mode string) int { return 1 }

// SettingsWindowUnavailable: there is no embedded window here; the app
// opens the settings in the browser.
const SettingsWindowUnavailable = 3

func RunSettingsWindow(url, dataDir string) int { return SettingsWindowUnavailable }
func Reveal(path string) error                  { return Open(filepath.Dir(path)) }
