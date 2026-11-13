// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Håvard Dagenborg

package platform

import (
	"os"
	"os/exec"
	"path/filepath"

	webview2 "github.com/jchv/go-webview2"
)

// SettingsWindowUnavailable is the exit code of the settings window
// process when WebView2 is missing; the app then uses the browser.
const SettingsWindowUnavailable = 3

// RunSettingsWindow shows url in a WebView2 window and returns when the
// window is closed. It must run on a thread with no other windows (the
// settings process's main thread). WebView2 keeps its cache under dataDir.
func RunSettingsWindow(url, dataDir string) int {
	if dataDir == "" {
		dataDir = os.TempDir()
	}
	w := webview2.NewWithOptions(webview2.WebViewOptions{
		DataPath:  filepath.Join(dataDir, "WebView2"),
		AutoFocus: true,
		WindowOptions: webview2.WindowOptions{
			Title:  "Unison Cue Settings",
			Width:  760,
			Height: 600,
			Center: true,
		},
	})
	if w == nil {
		return SettingsWindowUnavailable
	}
	defer w.Destroy()
	w.SetSize(560, 420, webview2.HintMin)
	w.Navigate(url)
	w.Run()
	return 0
}

// Reveal shows a file selected in Explorer.
func Reveal(path string) error {
	root := os.Getenv("SystemRoot")
	if root == "" {
		root = `C:\Windows`
	}
	return exec.Command(filepath.Join(root, "explorer.exe"), "/select,", path).Start()
}
