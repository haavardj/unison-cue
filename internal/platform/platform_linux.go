// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Håvard Dagenborg

package platform

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/godbus/dbus/v5"

	"github.com/haavardj/unison-cue/internal/icon"
)

// AppID is the name used for the .desktop file, the icon and the
// autostart entry.
const AppID = "unison-cue"

// TrayIcon returns PNG bytes. Linux panels have no template images; the
// colour icon reads well on both light and dark panels.
func TrayIcon(s icon.State, frame int) (template, regular []byte) {
	b := cachedIcon(s, frame, func() []byte { return icon.PNG(s, frame, 48, false) })
	return b, b
}

// UsesTemplateIcons is false on Linux.
const UsesTemplateIcons = false

var (
	appIconOnce sync.Once
	appIconRef  string
)

// notificationIcon returns the icon for notifications: the installed
// theme icon if the .deb put one there, else a PNG written to the cache
// directory (tarball installs).
func notificationIcon() string {
	appIconOnce.Do(func() {
		for _, dir := range []string{"/usr/share/icons/hicolor/128x128/apps", "/usr/local/share/icons/hicolor/128x128/apps"} {
			if _, err := os.Stat(filepath.Join(dir, AppID+".png")); err == nil {
				appIconRef = AppID
				return
			}
		}
		cache, err := os.UserCacheDir()
		if err != nil {
			return
		}
		p := filepath.Join(cache, AppID, "icon.png")
		if os.MkdirAll(filepath.Dir(p), 0o700) == nil &&
			os.WriteFile(p, icon.EncodePNG(icon.AppIcon(128)), 0o600) == nil {
			appIconRef = p
		}
	})
	return appIconRef
}

// InitNotifications does nothing on Linux.
func InitNotifications() {}

// Notify shows a desktop notification through the freedesktop.org
// notification service on the session bus.
func Notify(title, body, _ string) error {
	conn, err := dbus.SessionBus()
	if err != nil {
		return err
	}
	obj := conn.Object("org.freedesktop.Notifications", "/org/freedesktop/Notifications")
	hints := map[string]dbus.Variant{"desktop-entry": dbus.MakeVariant(AppID)}
	return obj.Call("org.freedesktop.Notifications.Notify", 0,
		"Unison Cue", uint32(0), notificationIcon(), title, notificationBody(body),
		[]string{}, hints, int32(-1)).Err
}

// notificationBody escapes the few characters that notification servers
// treat as markup, so file names show as written.
func notificationBody(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

// Open opens a file, folder or URL with the user's default application.
func Open(path string) error { return exec.Command("xdg-open", path).Start() }

// EditText opens a text file in the default text editor. Unison's .prf
// files are registered as another MIME type (application/pics-rules) on
// many systems, so the text/plain handler is looked up and started
// directly, with xdg-open as the fallback.
func EditText(path string) error {
	if out, err := exec.Command("xdg-mime", "query", "default", "text/plain").Output(); err == nil {
		desktop := strings.TrimSpace(string(out))
		if desktop != "" {
			if gl, err := exec.LookPath("gtk-launch"); err == nil {
				if exec.Command(gl, desktop, path).Start() == nil {
					return nil
				}
			}
		}
	}
	return Open(path)
}

// Reveal shows a file in the file manager, selected if the file manager
// supports it (org.freedesktop.FileManager1), else its folder.
func Reveal(path string) error {
	if conn, err := dbus.SessionBus(); err == nil {
		u := (&url.URL{Scheme: "file", Path: path}).String()
		obj := conn.Object("org.freedesktop.FileManager1", "/org/freedesktop/FileManager1")
		if obj.Call("org.freedesktop.FileManager1.ShowItems", 0, []string{u}, "").Err == nil {
			return nil
		}
	}
	return Open(filepath.Dir(path))
}

// terminalScript runs unison, then waits for Enter so the window does not
// vanish with the final summary. The program and profile arrive as
// positional parameters ($0, $1), never as part of the script text.
const terminalScript = `"$0" "$1"; s=$?; echo; printf 'unison finished (exit code %s). Press Enter to close. ' "$s"; read _`

// terminalCommand picks a terminal emulator and builds its command line.
// x-terminal-emulator (Debian/Ubuntu's configurable default) comes first.
func terminalCommand(lookPath func(string) (string, error), unisonPath, profile string) ([]string, error) {
	run := []string{"sh", "-c", terminalScript, unisonPath, profile}
	candidates := []struct {
		name string
		args []string // placed before the command
	}{
		{"x-terminal-emulator", []string{"-e"}},
		{"gnome-terminal", []string{"--"}},
		{"ptyxis", []string{"--"}},
		{"kgx", []string{"--"}},
		{"konsole", []string{"-e"}},
		{"xfce4-terminal", []string{"-x"}},
		{"xterm", []string{"-e"}},
	}
	for _, c := range candidates {
		if p, err := lookPath(c.name); err == nil {
			return append(append([]string{p}, c.args...), run...), nil
		}
	}
	return nil, errors.New("no terminal emulator found (tried x-terminal-emulator, gnome-terminal, ptyxis, kgx, konsole, xfce4-terminal, xterm)")
}

// RunInTerminal opens a terminal window running unison interactively.
func RunInTerminal(unisonPath, profile string) error {
	argv, err := terminalCommand(exec.LookPath, unisonPath, profile)
	if err != nil {
		return err
	}
	return exec.Command(argv[0], argv[1:]...).Start()
}

func autostartPath() string {
	dir, err := os.UserConfigDir() // $XDG_CONFIG_HOME or ~/.config
	if err != nil {
		dir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(dir, "autostart", AppID+".desktop")
}

// AutostartEnabled reports whether the XDG autostart entry exists.
func AutostartEnabled() bool {
	_, err := os.Stat(autostartPath())
	return err == nil
}

// SetAutostart writes or removes ~/.config/autostart/unison-cue.desktop.
func SetAutostart(on bool) error {
	p := autostartPath()
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
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, []byte(autostartEntry(exe)), 0o644)
}

func autostartEntry(exe string) string {
	return fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=Unison Cue
Comment=Sync Unison profiles on demand, with reminders
Exec=%s
Icon=%s
Terminal=false
X-GNOME-Autostart-enabled=true
`, desktopExecQuote(exe), AppID)
}

// desktopExecQuote quotes a path for the Exec key of a .desktop file
// (Desktop Entry Specification, "The Exec key").
func desktopExecQuote(s string) string {
	r := strings.NewReplacer(`\`, `\\\\`, `"`, `\\"`, "`", "\\\\`", `$`, `\\$`, `%`, `%%`)
	return `"` + r.Replace(s) + `"`
}

// MigrateLegacy: there was no Linux version under the old name.
func MigrateLegacy() {}

// SettingsWindowUnavailable: Linux has no embedded settings window (it
// would need WebKitGTK through cgo); the app opens the settings page in
// the default browser instead.
const SettingsWindowUnavailable = 3

// RunSettingsWindow always reports that no window is available.
func RunSettingsWindow(url, dataDir string) int { return SettingsWindowUnavailable }
