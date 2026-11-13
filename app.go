// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Håvard Dagenborg

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/haavardj/unison-cue/internal/core"
	"github.com/haavardj/unison-cue/internal/platform"
)

// App owns the configuration, the persisted state and the sync queue.
// mu guards every field below it; UI updates happen outside mu.
type App struct {
	store *core.Store

	mu            sync.Mutex
	cfg           *core.Config
	cfgModTime    time.Time
	state         *core.State
	profiles      []core.Profile
	unisonPath    string
	unisonVersion string

	queue   []string           // profiles waiting to sync
	running string             // profile syncing now, "" if idle
	cancel  context.CancelFunc // stops the running sync
	batch   []core.Result      // results since the queue was last empty

	wake      chan struct{}
	ui        *menuUI
	trayReady atomic.Bool
}

func NewApp(store *core.Store) (*App, error) {
	a := &App{store: store, wake: make(chan struct{}, 1)}
	if err := a.loadConfig(); err != nil {
		return nil, err
	}
	a.state = store.LoadState()
	a.scanProfiles()
	return a, nil
}

// loadConfig (re)reads the settings file. Caller must not hold mu.
func (a *App) loadConfig() error {
	cfg, err := a.store.LoadConfig()
	if err != nil {
		return fmt.Errorf("reading %s: %w", a.store.ConfigPath(), err)
	}
	var mod time.Time
	if fi, err := os.Stat(a.store.ConfigPath()); err == nil {
		mod = fi.ModTime()
	}
	path := core.FindUnison(cfg.UnisonPath)
	a.mu.Lock()
	changed := path != a.unisonPath
	a.cfg, a.cfgModTime, a.unisonPath = cfg, mod, path
	a.mu.Unlock()
	if changed {
		go a.probeUnisonVersion(path)
	}
	return nil
}

func (a *App) probeUnisonVersion(path string) {
	v := ""
	if path != "" {
		out, err := exec.Command(path, "-version").Output()
		if err == nil {
			v = strings.TrimSpace(string(out))
		}
	}
	a.mu.Lock()
	a.unisonVersion = v
	a.mu.Unlock()
	a.refresh()
}

// unisonDir is the directory holding the profiles. Caller holds mu.
func (a *App) unisonDir() string {
	if a.cfg.UnisonDir != "" {
		return a.cfg.UnisonDir
	}
	return core.DefaultUnisonDir()
}

// scanProfiles re-reads the profile directory and reports whether the
// set of profiles (or their labels/roots) changed.
func (a *App) scanProfiles() bool {
	a.mu.Lock()
	dir := a.unisonDir()
	a.mu.Unlock()
	ps, err := core.DiscoverProfiles(dir)
	if err != nil {
		log.Printf("scanning %s: %v", dir, err)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	same := slices.EqualFunc(ps, a.profiles, func(x, y core.Profile) bool {
		return x.Name == y.Name && x.Label == y.Label && slices.Equal(x.Roots, y.Roots)
	})
	a.profiles = ps
	return !same
}

// visibleProfiles returns the profiles shown in the menu. Caller holds mu.
func (a *App) visibleProfiles() []core.Profile {
	var out []core.Profile
	for _, p := range a.profiles {
		if !a.cfg.Profile(p.Name).Hidden {
			out = append(out, p)
		}
	}
	return out
}

func (a *App) profileByName(name string) (core.Profile, bool) {
	for _, p := range a.profiles {
		if p.Name == name {
			return p, true
		}
	}
	return core.Profile{}, false
}

// ---- syncing ---------------------------------------------------------

// Enqueue schedules profiles for a background sync.
func (a *App) Enqueue(names ...string) {
	a.mu.Lock()
	for _, n := range names {
		if n != a.running && !slices.Contains(a.queue, n) {
			a.queue = append(a.queue, n)
		}
	}
	a.mu.Unlock()
	select {
	case a.wake <- struct{}{}:
	default:
	}
	a.refresh()
}

// SyncAll queues every visible profile not excluded from "Sync all".
func (a *App) SyncAll() {
	a.mu.Lock()
	var names []string
	for _, p := range a.visibleProfiles() {
		if !a.cfg.Profile(p.Name).ExcludeFromSyncAll {
			names = append(names, p.Name)
		}
	}
	a.mu.Unlock()
	a.Enqueue(names...)
}

// Cancel stops the running sync and drops the queue.
func (a *App) Cancel() {
	a.mu.Lock()
	a.queue = nil
	if a.cancel != nil {
		a.cancel()
	}
	a.mu.Unlock()
	a.refresh()
}

// worker runs queued syncs one at a time. Unison locks its archives, so
// running two profiles that share a root concurrently would fail anyway.
func (a *App) worker() {
	for range a.wake {
		for a.runNext() {
		}
	}
}

func (a *App) runNext() bool {
	a.mu.Lock()
	if len(a.queue) == 0 {
		a.mu.Unlock()
		return false
	}
	name := a.queue[0]
	a.queue = a.queue[1:]
	ctx, cancel := context.WithCancel(context.Background())
	a.running, a.cancel = name, cancel
	runner := &core.Runner{
		UnisonPath: a.unisonPath,
		ExtraArgs:  a.cfg.ExtraArgs,
		LogDir:     a.store.LogDir(),
		Timeout:    a.cfg.Timeout(),
	}
	if !a.cfg.NoSSHAskpass {
		if exe, err := os.Executable(); err == nil {
			runner.AskPass = exe
		}
	}
	a.mu.Unlock()
	a.refresh()

	res := runner.Run(ctx, name)
	cancel()

	a.mu.Lock()
	a.running, a.cancel = "", nil
	if res.Status != core.StatusCanceled {
		ps := a.state.Profile(name)
		ps.LastRun = res.End
		ps.LastStatus = res.Status
		ps.LastSummary = res.Summary
		if res.Status.Completed() {
			// The replicas reflect the moment the scan started.
			ps.LastSuccess = res.Start
		}
		if err := a.store.SaveState(a.state); err != nil {
			log.Printf("saving state: %v", err)
		}
	}
	a.batch = append(a.batch, res)
	var finished []core.Result
	if len(a.queue) == 0 {
		finished, a.batch = a.batch, nil
	}
	a.mu.Unlock()

	if finished != nil {
		a.reportBatch(finished)
	}
	a.refresh()
	return true
}

// reportBatch shows one notification for a completed run of the queue.
func (a *App) reportBatch(results []core.Result) {
	var done, problems []core.Result
	for _, r := range results {
		switch r.Status {
		case core.StatusOK:
			done = append(done, r)
		case core.StatusConflicts, core.StatusFailed:
			problems = append(problems, r)
		}
	}
	a.mu.Lock()
	quiet := a.cfg.QuietSuccess
	name := func(r core.Result) string {
		if p, ok := a.profileByName(r.Profile); ok {
			return p.DisplayName()
		}
		return r.Profile
	}
	var title, body string
	switch {
	case len(problems) == 1:
		r := problems[0]
		if r.Status == core.StatusConflicts {
			title = "Unison: " + name(r) + " has unresolved conflicts"
			body = r.Summary + ". Use “Resolve Conflicts in Terminal…” in the menu."
		} else {
			title = "Unison: syncing " + name(r) + " failed"
			body = r.Summary
		}
		switch len(done) {
		case 0:
		case 1:
			body += "\n" + name(done[0]) + " synced fine."
		default:
			body += fmt.Sprintf("\n%d other profiles synced fine.", len(done))
		}
	case len(problems) > 0:
		title = fmt.Sprintf("Unison: %d of %d profiles need attention", len(problems), len(problems)+len(done))
		var parts []string
		for _, r := range problems {
			parts = append(parts, name(r)+": "+r.Summary)
		}
		body = strings.Join(parts, "\n")
	case len(done) == 1 && !quiet:
		title = "Unison: " + name(done[0]) + " synced"
		body = done[0].Summary
	case len(done) > 1 && !quiet:
		title = fmt.Sprintf("Unison: %d profiles synced", len(done))
		var parts []string
		for _, r := range done {
			parts = append(parts, name(r)+": "+r.Summary)
		}
		body = strings.Join(parts, "\n")
	}
	a.mu.Unlock()
	if title != "" {
		notify(title, body)
	}
}

// ---- reminders -------------------------------------------------------

// checkReminders runs every minute: it picks up edits to the settings
// file and new profiles, updates the menu, and nags if something is due.
func (a *App) checkReminders() {
	if fi, err := os.Stat(a.store.ConfigPath()); err == nil {
		a.mu.Lock()
		changed := !fi.ModTime().Equal(a.cfgModTime)
		a.mu.Unlock()
		if changed {
			if err := a.loadConfig(); err != nil {
				notify("Unison Cue: settings not loaded", err.Error())
				a.mu.Lock()
				a.cfgModTime = fi.ModTime() // do not repeat the error every minute
				a.mu.Unlock()
			} else {
				a.scanProfiles()
				a.rebuildMenu()
			}
		}
	}
	if a.scanProfiles() {
		a.rebuildMenu()
	}

	now := time.Now()
	a.mu.Lock()
	due := core.DueProfiles(now, a.profiles, a.cfg, a.state)
	busy := a.running != "" || len(a.queue) > 0
	send := !busy && core.ShouldNotify(now, len(due), a.state.LastNotified, a.state.SnoozeUntil, a.cfg.RenotifyInterval())
	var body string
	if send {
		body = core.ReminderText(now, due, a.state)
		a.state.LastNotified = now
		_ = a.store.SaveState(a.state)
	}
	a.mu.Unlock()

	if send {
		notify("Time to sync?", body+" Use the Unison Cue menu to sync.")
	}
	a.refresh()
}

func (a *App) reminderLoop() {
	time.Sleep(20 * time.Second) // let the desktop settle after login
	a.checkReminders()
	t := time.NewTicker(time.Minute)
	for range t.C {
		a.checkReminders()
	}
}

// Snooze suppresses reminder notifications until t (zero to resume).
func (a *App) Snooze(until time.Time) {
	a.mu.Lock()
	a.state.SnoozeUntil = until
	_ = a.store.SaveState(a.state)
	a.mu.Unlock()
	a.refresh()
}

// SetReminderHours changes the global reminder interval.
func (a *App) SetReminderHours(h float64) {
	a.updateConfig(func(c *core.Config) { c.ReminderHours = h })
}

// UpdateProfileSettings changes one profile's settings.
func (a *App) UpdateProfileSettings(name string, f func(*core.ProfileSettings)) {
	a.updateConfig(func(c *core.Config) {
		ps := c.Profile(name)
		f(&ps)
		c.SetProfile(name, ps)
	})
}

func (a *App) updateConfig(f func(*core.Config)) {
	a.mu.Lock()
	f(a.cfg)
	err := a.store.SaveConfig(a.cfg)
	if fi, statErr := os.Stat(a.store.ConfigPath()); statErr == nil {
		a.cfgModTime = fi.ModTime() // our own write; no reload needed
	}
	a.mu.Unlock()
	if err != nil {
		notify("Unison Cue: could not save settings", err.Error())
	}
	a.refresh()
}

// notifyFunc is replaced in tests.
var notifyFunc = platform.Notify

func notify(title, body string) {
	if err := notifyFunc(title, body); err != nil {
		log.Printf("notification failed: %v", err)
	}
}
