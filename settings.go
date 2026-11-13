// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Håvard Dagenborg

package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/haavardj/unison-cue/internal/core"
	"github.com/haavardj/unison-cue/internal/platform"
	"github.com/haavardj/unison-cue/internal/settings"
)

// Environment variables that start the binary as the settings window
// instead of the tray app.
const (
	settingsURLEnv  = "UNISON_CUE_SETTINGS_URL"
	settingsDataEnv = "UNISON_CUE_SETTINGS_DATA"
)

// OpenSettings shows the settings window. The window is a separate
// process (this binary in settings mode) showing a page served by this
// process; if the window crashes, the tray app is unaffected. A second
// click replaces an open window, bringing it to the front.
func (a *App) OpenSettings() {
	a.settingsMu.Lock()
	defer a.settingsMu.Unlock()
	if a.settingsProc != nil {
		_ = a.settingsProc.Kill()
		a.settingsProc = nil
	}
	srv, err := settings.Start(a)
	if err != nil {
		notify("Unison Cue", "Could not open the settings: "+err.Error())
		return
	}
	exe, err := os.Executable()
	if err != nil {
		srv.Close()
		notify("Unison Cue", "Could not open the settings: "+err.Error())
		return
	}
	cmd := exec.Command(exe)
	cmd.Env = append(os.Environ(), settingsURLEnv+"="+srv.URL(), settingsDataEnv+"="+a.store.Dir)
	if err := cmd.Start(); err != nil {
		srv.Close()
		notify("Unison Cue", "Could not open the settings: "+err.Error())
		return
	}
	proc := cmd.Process
	a.settingsProc = proc
	go func() {
		err := cmd.Wait()
		var ee *exec.ExitError
		if errors.As(err, &ee) && ee.ExitCode() == platform.SettingsWindowUnavailable {
			// No embedded browser (Windows without WebView2): use the
			// default browser, and keep the page available for a while.
			log.Println("settings window unavailable; opening the settings in the browser")
			if err := platform.Open(srv.URL()); err != nil {
				notify("Unison Cue", "Could not open the settings: "+err.Error())
			}
			time.AfterFunc(30*time.Minute, func() { srv.Close() })
		} else {
			srv.Close()
		}
		a.settingsMu.Lock()
		if a.settingsProc == proc {
			a.settingsProc = nil
		}
		a.settingsMu.Unlock()
	}()
}

// closeSettings closes an open settings window (on quit).
func (a *App) closeSettings() {
	a.settingsMu.Lock()
	defer a.settingsMu.Unlock()
	if a.settingsProc != nil {
		_ = a.settingsProc.Kill()
		a.settingsProc = nil
	}
}

// runSettingsWindowIfRequested runs the settings window when this
// process was started for that, and reports whether it did.
func runSettingsWindowIfRequested() (exitCode int, ran bool) {
	url := os.Getenv(settingsURLEnv)
	if url == "" {
		return 0, false
	}
	// Do not pass the token on to anything the window might start.
	os.Unsetenv(settingsURLEnv)
	return platform.RunSettingsWindow(url, os.Getenv(settingsDataEnv)), true
}

// ---- settings.Backend ------------------------------------------------

// Snapshot is the current settings and related information.
func (a *App) Snapshot() settings.State {
	a.mu.Lock()
	cfg := *a.cfg
	cfg.Profiles = cloneMap(a.cfg.Profiles)
	cfg.Environment = cloneMap(a.cfg.Environment)
	cfg.ExtraArgs = slices.Clone(a.cfg.ExtraArgs)
	var profiles []settings.Profile
	for _, p := range a.profiles {
		profiles = append(profiles, settings.Profile{
			Name: p.Name, Label: p.Label, Roots: p.Roots, Path: p.Path, Settings: cfg.Profile(p.Name),
		})
	}
	info := settings.Info{
		Version:          version,
		Platform:         runtime.GOOS,
		DetectedUnison:   core.FindUnison(""),
		UnisonVersion:    a.unisonVersion,
		DefaultDir:       core.DefaultUnisonDir(),
		ProfileDir:       a.unisonDir(),
		ConfigPath:       a.store.ConfigPath(),
		LogDir:           a.store.LogDir(),
		ShellImportNames: a.shellEnvNames,
	}
	env := effectiveEnv(cfg.Environment, a.shellEnvNames)
	a.mu.Unlock()

	if cfg.Profiles == nil {
		cfg.Profiles = map[string]core.ProfileSettings{}
	}
	if cfg.Environment == nil {
		cfg.Environment = map[string]string{}
	}
	return settings.State{
		Config:    cfg,
		Autostart: platform.AutostartEnabled(),
		Profiles:  profiles,
		Env:       env,
		Info:      info,
	}
}

// Apply validates and saves settings from the settings window.
func (a *App) Apply(u settings.Update) (settings.State, map[string]string, error) {
	cfg := u.Config
	cfg.Normalize()
	if errs := cfg.Validate(); len(errs) > 0 {
		return settings.State{}, errs, nil
	}
	if u.Autostart != platform.AutostartEnabled() {
		if err := platform.SetAutostart(u.Autostart); err != nil {
			return settings.State{}, nil, fmt.Errorf("could not change the login item: %w", err)
		}
	}
	if err := a.store.SaveConfig(&cfg); err != nil {
		return settings.State{}, nil, err
	}
	if err := a.loadConfig(); err != nil {
		return settings.State{}, nil, err
	}
	a.scanProfiles()
	a.rebuildMenu()
	return a.Snapshot(), nil, nil
}

// Open shows one of the app's folders in the file manager.
func (a *App) Open(what string) error {
	a.mu.Lock()
	dir := a.unisonDir()
	a.mu.Unlock()
	switch what {
	case "config":
		return platform.Reveal(a.store.ConfigPath())
	case "logs":
		return platform.Open(a.store.LogDir())
	case "profiles":
		return platform.Open(dir)
	}
	return fmt.Errorf("unknown folder %q", what)
}

// effectiveEnv lists the UNISON* variables unison will see, with where
// each value comes from. Settings override the process environment.
func effectiveEnv(fromSettings map[string]string, shellNames []string) []settings.EnvVar {
	vals := map[string]settings.EnvVar{}
	for _, kv := range os.Environ() {
		k, v, _ := strings.Cut(kv, "=")
		if core.IsUnisonVar(k) {
			src := "system"
			if slices.Contains(shellNames, k) {
				src = "shell"
			}
			vals[k] = settings.EnvVar{Name: k, Value: v, Source: src}
		}
	}
	for k, v := range fromSettings {
		vals[k] = settings.EnvVar{Name: k, Value: v, Source: "settings"}
	}
	out := make([]settings.EnvVar, 0, len(vals))
	for _, v := range vals {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func cloneMap[K comparable, V any](m map[K]V) map[K]V {
	if m == nil {
		return nil
	}
	out := make(map[K]V, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
