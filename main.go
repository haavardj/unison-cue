// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Håvard Dagenborg

// Unison Cue is a menu bar (macOS) / notification area (Windows) front
// end for the Unison file synchronizer. Syncs run only when the user asks;
// the tray icon and occasional notifications remind them when a profile
// has gone unsynced for too long.
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"fyne.io/systray"

	"github.com/haavardj/unison-cue/internal/core"
	"github.com/haavardj/unison-cue/internal/platform"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	// Started by ssh as its SSH_ASKPASS helper: show one dialog and exit.
	if os.Getenv(core.AskPassEnvVar) == "1" {
		prompt := strings.Join(os.Args[1:], " ")
		os.Exit(platform.AskPass(prompt, os.Getenv("SSH_ASKPASS_PROMPT")))
	}

	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println("Unison Cue", version)
		return
	}

	store, err := core.DefaultStore()
	if err != nil {
		log.Fatal(err)
	}
	// Logs name synced files and state names profiles: keep them private.
	if err := os.MkdirAll(store.LogDir(), 0o700); err != nil {
		log.Fatal(err)
	}
	os.Chmod(store.Dir, 0o700)
	os.Chmod(store.LogDir(), 0o700)
	// GUI apps have no visible stderr; keep our own diagnostics in a file.
	if f, err := os.OpenFile(filepath.Join(store.Dir, "unison-cue.log"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); err == nil {
		log.SetOutput(io.MultiWriter(os.Stderr, f))
	}

	ok, release := platform.SingleInstance(store.Dir)
	if !ok {
		log.Println("another instance is already running")
		return
	}
	defer release()
	platform.MigrateLegacy()

	app, err := NewApp(store)
	if err != nil {
		log.Fatal(err)
	}
	systray.Run(func() {
		systray.SetTitle("")
		app.trayReady.Store(true)
		app.rebuildMenu()
		go app.animate()
		go app.worker()
		go app.reminderLoop()
	}, func() {
		log.Println("exiting")
	})
}
