// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Håvard Dagenborg

package platform

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/haavardj/unison-cue/internal/icon"
)

const launchAgentLabel = "io.github.haavardj.unisoncue"

// TrayIcon returns the bytes for systray.SetTemplateIcon. macOS draws
// template images in the menu bar's own colour (light or dark).
func TrayIcon(s icon.State, frame int) (template, regular []byte) {
	b := cachedIcon(s, frame, func() []byte { return icon.PNG(s, frame, 44, true) }) // 22pt at 2x
	return b, b
}

// UsesTemplateIcons is true on macOS.
const UsesTemplateIcons = true

// runScript runs an AppleScript "on run argv" body with args as argv, so
// no quoting or escaping is involved. Text must go through argv, which
// osascript decodes as UTF-8: "system attribute" reads the environment as
// Mac OS Roman and garbles every non-ASCII character.
func runScript(body string, args ...string) *exec.Cmd {
	argv := append([]string{"-e", "on run argv\n" + body + "\nend run", "--"}, args...)
	return exec.Command(osascriptPath, argv...)
}

// Open opens a file or folder with its default application.
func Open(path string) error { return exec.Command(openPath, path).Start() }

// EditText opens a text file in the default text editor.
func EditText(path string) error { return exec.Command(openPath, "-t", path).Start() }

// RunInTerminal opens Terminal and runs unison interactively there, so
// the user can resolve conflicts that batch mode skipped.
func RunInTerminal(unisonPath, profile string) error {
	line := shellQuote(unisonPath) + " " + shellQuote(profile)
	return runScript(`tell application "Terminal"
activate
do script (item 1 of argv)
end tell`, line).Run()
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func launchAgentPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", launchAgentLabel+".plist")
}

// AutostartEnabled reports whether the app starts at login.
func AutostartEnabled() bool {
	_, err := os.Stat(launchAgentPath())
	return err == nil
}

// SetAutostart installs or removes a LaunchAgent that starts the app at
// login. The executable inside the .app bundle is started directly; it
// picks up the bundle's Info.plist (LSUIElement), so no Dock icon appears.
func SetAutostart(on bool) error {
	p := launchAgentPath()
	if !on {
		err := os.Remove(p)
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>ProcessType</key>
	<string>Interactive</string>
</dict>
</plist>
`, launchAgentLabel, xmlEscape(exe))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, []byte(plist), 0o644)
}

func xmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return r.Replace(s)
}

// Absolute paths, so a modified PATH cannot substitute these tools.
const (
	osascriptPath = "/usr/bin/osascript"
	openPath      = "/usr/bin/open"
)

// MigrateLegacy moves the login item of the pre-release "Unison Tray"
// name over to the current one.
func MigrateLegacy() {
	home, _ := os.UserHomeDir()
	old := filepath.Join(home, "Library", "LaunchAgents", "org.unisontray.UnisonTray.plist")
	if _, err := os.Stat(old); err == nil {
		os.Remove(old)
		_ = SetAutostart(true)
	}
}
