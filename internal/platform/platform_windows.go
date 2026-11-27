// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Håvard Dagenborg

package platform

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"github.com/haavardj/unison-cue/internal/icon"
)

// system32 returns the absolute path of a Windows system tool, so a
// modified PATH cannot substitute it.
func system32(name string) string {
	root := os.Getenv("SystemRoot")
	if root == "" {
		root = `C:\Windows`
	}
	return filepath.Join(root, "System32", name)
}

const (
	runKey         = `Software\Microsoft\Windows\CurrentVersion\Run`
	runValue       = "UnisonCue"
	createNoWindow = 0x08000000
)

// TrayIcon returns .ico bytes; Windows has no template images, so both
// values are the same colour icon.
func TrayIcon(s icon.State, frame int) (template, regular []byte) {
	b := cachedIcon(s, frame, func() []byte { return icon.ICO(s, frame, 16, 20, 24, 32, 48) })
	return b, b
}

// UsesTemplateIcons is false on Windows.
const UsesTemplateIcons = false

func hidden(cmd *exec.Cmd) *exec.Cmd {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	return cmd
}

// toastScript shows a toast through the WinRT API, attributed to
// PowerShell (an unpackaged app has no AppUserModelID of its own).
// Title and body arrive in environment variables, so nothing needs quoting.
const toastScript = `
[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] > $null
$t = [Windows.UI.Notifications.ToastNotificationManager]::GetTemplateContent([Windows.UI.Notifications.ToastTemplateType]::ToastText02)
$x = $t.GetElementsByTagName('text')
$x.Item(0).AppendChild($t.CreateTextNode($env:UT_TITLE)) > $null
$x.Item(1).AppendChild($t.CreateTextNode($env:UT_BODY)) > $null
$id = '{1AC14E77-02E7-4E5D-B744-2EB1AE5198B7}\WindowsPowerShell\v1.0\powershell.exe'
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier($id).Show([Windows.UI.Notifications.ToastNotification]::new($t))
`

// InitNotifications does nothing on Windows.
func InitNotifications() {}

// Notify shows a Windows toast notification. The click target is not
// used: a toast sent through PowerShell cannot react to clicks.
func Notify(title, body, _ string) error {
	cmd := hidden(exec.Command(system32(`WindowsPowerShell\v1.0\powershell.exe`), "-NoProfile", "-NonInteractive",
		"-ExecutionPolicy", "Bypass", "-WindowStyle", "Hidden", "-Command", toastScript))
	cmd.Env = append(os.Environ(), "UT_TITLE="+title, "UT_BODY="+body)
	return cmd.Run()
}

// Open opens a file or folder with its default application.
func Open(path string) error {
	return hidden(exec.Command(system32("rundll32.exe"), "url.dll,FileProtocolHandler", path)).Start()
}

// EditText opens a text file in Notepad (.prf has no default handler).
func EditText(path string) error { return exec.Command(system32("notepad.exe"), path).Start() }

// RunInTerminal opens a console window running unison interactively.
// The command line passes through cmd.exe twice (start, then cmd /k),
// and cmd.exe quoting cannot reliably protect & | < > ^ % ! " at both
// levels. Names or paths containing them are refused rather than risk
// running part of a file name as a command.
func RunInTerminal(unisonPath, profile string) error {
	for _, s := range []string{unisonPath, profile} {
		if strings.ContainsAny(s, "&|<>^%!\"\r\n") {
			return fmt.Errorf("%q contains characters that cannot be passed safely through cmd.exe; run unison from a terminal instead", s)
		}
	}
	comspec := system32("cmd.exe")
	inner := `"` + unisonPath + `" "` + profile + `"`
	cmd := exec.Command(comspec)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CmdLine: `/c start "Unison ` + profile + `" cmd /k "` + inner + `"`,
	}
	return cmd.Start()
}

// AutostartEnabled reports whether the Run registry value exists.
func AutostartEnabled() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	_, _, err = k.GetStringValue(runValue)
	return err == nil
}

// SetAutostart adds or removes the HKCU Run entry.
func SetAutostart(on bool) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if !on {
		err := k.DeleteValue(runValue)
		if err == registry.ErrNotExist {
			return nil
		}
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if inDir(exe, os.TempDir()) {
		return errors.New("Unison Cue is running from a temporary folder, probably straight from the zip file. " +
			"Unzip it to a permanent folder, start it from there and try again.")
	}
	return k.SetStringValue(runValue, `"`+exe+`"`)
}

// inDir reports whether path is inside dir. Windows paths compare
// without regard to case; 8.3 short names are expanded where possible.
func inDir(path, dir string) bool {
	if p, err := filepath.EvalSymlinks(path); err == nil {
		path = p
	}
	if d, err := filepath.EvalSymlinks(dir); err == nil {
		dir = d
	}
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, `..\`)
}

// SingleInstance takes a per-user named mutex. ok is false if another
// instance already holds it.
func SingleInstance(dir string) (ok bool, release func()) {
	name, _ := windows.UTF16PtrFromString(`Local\UnisonCue-single-instance`)
	h, err := windows.CreateMutex(nil, false, name)
	if err == windows.ERROR_ALREADY_EXISTS {
		if h != 0 {
			windows.CloseHandle(h)
		}
		return false, func() {}
	}
	if err != nil {
		return true, func() {} // cannot tell; run anyway
	}
	return true, func() { windows.CloseHandle(h) }
}

// MigrateLegacy moves the Run entry of the pre-release "Unison Tray" name
// over to the current one.
func MigrateLegacy() {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return
	}
	defer k.Close()
	if _, _, err := k.GetStringValue("UnisonTray"); err == nil {
		k.DeleteValue("UnisonTray")
		_ = SetAutostart(true)
	}
}
