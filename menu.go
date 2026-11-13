// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Håvard Dagenborg

package main

import (
	"fmt"
	"log"
	"sync"
	"time"

	"fyne.io/systray"

	"github.com/haavardj/unison-cue/internal/core"
	"github.com/haavardj/unison-cue/internal/icon"
	"github.com/haavardj/unison-cue/internal/platform"
)

// reminderChoices are offered under "Remind Me After".
var reminderChoices = []float64{6, 12, 24, 48, 72, 168}

type profileItems struct {
	name        string
	root        *systray.MenuItem
	syncNow     *systray.MenuItem
	interactive *systray.MenuItem
	lastSynced  *systray.MenuItem
	lastResult  *systray.MenuItem
	includeAll  *systray.MenuItem
	remind      *systray.MenuItem
}

type menuUI struct {
	mu sync.Mutex // serializes rebuilds and refreshes

	header      *systray.MenuItem
	syncAll     *systray.MenuItem
	cancel      *systray.MenuItem
	profiles    []*profileItems
	remindAfter map[float64]*systray.MenuItem
	snoozeRoot  *systray.MenuItem
	snoozeOff   *systray.MenuItem
	autostart   *systray.MenuItem
	about       *systray.MenuItem

	lastIcon icon.State
	iconSet  bool
	frame    int // syncing animation frame
}

// on runs f for every click on item. The goroutine ends when the item is
// removed (systray closes ClickedCh), so menu rebuilds do not leak.
func on(item *systray.MenuItem, f func()) {
	go func() {
		for range item.ClickedCh {
			f()
		}
	}()
}

// rebuildMenu builds the whole menu from scratch. Needed when the set of
// profiles changes; everything else is updated in place by refresh.
func (a *App) rebuildMenu() {
	if !a.trayReady.Load() {
		return // tray not running (tests) or not ready yet
	}
	if a.ui == nil {
		a.ui = &menuUI{}
	}
	ui := a.ui
	ui.mu.Lock()
	systray.ResetMenu()

	a.mu.Lock()
	profiles := a.visibleProfiles()
	a.mu.Unlock()

	ui.header = systray.AddMenuItem("", "")
	ui.header.Disable()
	systray.AddSeparator()

	ui.syncAll = systray.AddMenuItem("Sync All Now", "Sync every profile included in “Sync All”")
	on(ui.syncAll, a.SyncAll)
	ui.cancel = systray.AddMenuItem("Stop Syncing", "Stop the running sync and clear the queue")
	on(ui.cancel, a.Cancel)
	systray.AddSeparator()

	ui.profiles = nil
	if len(profiles) == 0 {
		a.mu.Lock()
		dir := a.unisonDir()
		a.mu.Unlock()
		none := systray.AddMenuItem("No profiles in "+dir, "Create a .prf file there, then choose Rescan Profiles")
		none.Disable()
	}
	for _, p := range profiles {
		ui.profiles = append(ui.profiles, a.addProfileMenu(p))
	}
	systray.AddSeparator()

	// Reminders.
	remind := systray.AddMenuItem("Remind Me After", "How long a profile may go without a sync before the reminder")
	ui.remindAfter = map[float64]*systray.MenuItem{}
	for _, h := range reminderChoices {
		h := h
		it := remind.AddSubMenuItemCheckbox(core.FormatHours(h), "", false)
		ui.remindAfter[h] = it
		on(it, func() { a.SetReminderHours(h) })
	}
	ui.snoozeRoot = systray.AddMenuItem("Snooze Reminders", "Pause reminder notifications for a while")
	s1 := ui.snoozeRoot.AddSubMenuItem("For 1 Hour", "")
	on(s1, func() { a.Snooze(time.Now().Add(time.Hour)) })
	s4 := ui.snoozeRoot.AddSubMenuItem("For 4 Hours", "")
	on(s4, func() { a.Snooze(time.Now().Add(4 * time.Hour)) })
	st := ui.snoozeRoot.AddSubMenuItem("Until Tomorrow Morning", "")
	on(st, func() { a.Snooze(core.NextMorning(time.Now())) })
	ui.snoozeOff = ui.snoozeRoot.AddSubMenuItem("Resume Reminders", "")
	on(ui.snoozeOff, func() { a.Snooze(time.Time{}) })
	systray.AddSeparator()

	// Settings.
	settingsItem := systray.AddMenuItem("Settings…", "Reminders, profiles, environment variables and more")
	on(settingsItem, a.OpenSettings)
	ui.autostart = systray.AddMenuItemCheckbox("Start at Login", "Start Unison Cue when you log in", platform.AutostartEnabled())
	on(ui.autostart, func() {
		want := !platform.AutostartEnabled()
		if err := platform.SetAutostart(want); err != nil {
			notify("Unison Cue", "Could not change login item: "+err.Error())
		}
		a.refresh()
	})
	more := systray.AddMenuItem("More", "")
	editCfg := more.AddSubMenuItem("Edit Settings File…", "Changes are picked up automatically")
	on(editCfg, func() { openOrWarn(platform.EditText, a.store.ConfigPath()) })
	openDir := more.AddSubMenuItem("Open Profiles Folder", "")
	on(openDir, func() {
		a.mu.Lock()
		d := a.unisonDir()
		a.mu.Unlock()
		openOrWarn(platform.Open, d)
	})
	openLogs := more.AddSubMenuItem("Open Logs Folder", "")
	on(openLogs, func() { openOrWarn(platform.Open, a.store.LogDir()) })
	rescan := more.AddSubMenuItem("Rescan Profiles", "")
	on(rescan, func() {
		_ = a.loadConfig()
		a.scanProfiles()
		a.rebuildMenu()
	})
	ui.about = more.AddSubMenuItem("", "")
	ui.about.Disable()
	systray.AddSeparator()

	quit := systray.AddMenuItem("Quit Unison Cue", "")
	on(quit, func() {
		a.closeSettings()
		a.Cancel()
		systray.Quit()
	})

	ui.iconSet = false
	ui.mu.Unlock()
	a.refresh()
}

func (a *App) addProfileMenu(p core.Profile) *profileItems {
	name := p.Name
	it := &profileItems{name: name}
	it.root = systray.AddMenuItem(p.DisplayName(), "")
	it.syncNow = it.root.AddSubMenuItem("Sync Now", "Sync in the background; conflicts are skipped and reported")
	on(it.syncNow, func() { a.Enqueue(name) })
	it.interactive = it.root.AddSubMenuItem("Sync Interactively in Terminal…", "Run unison in a terminal window to review changes and resolve conflicts")
	on(it.interactive, func() {
		a.mu.Lock()
		u := a.unisonPath
		a.mu.Unlock()
		if u == "" {
			notify("Unison Cue", "unison executable not found; set unisonPath in the settings file.")
			return
		}
		if err := platform.RunInTerminal(u, name); err != nil {
			notify("Unison Cue", "Could not open a terminal: "+err.Error())
		}
	})
	it.lastSynced = it.root.AddSubMenuItem("", "")
	it.lastSynced.Disable()
	it.lastResult = it.root.AddSubMenuItem("", "")
	it.lastResult.Disable()
	for _, r := range p.Roots {
		ri := it.root.AddSubMenuItem("⇄ "+r, "")
		ri.Disable()
	}
	it.root.AddSeparator()
	viewLog := it.root.AddSubMenuItem("View Last Log", "")
	on(viewLog, func() {
		openOrWarn(platform.EditText, (&core.Runner{LogDir: a.store.LogDir()}).LogPath(name))
	})
	edit := it.root.AddSubMenuItem("Edit Profile…", p.Path)
	path := p.Path
	on(edit, func() { openOrWarn(platform.EditText, path) })
	it.includeAll = it.root.AddSubMenuItemCheckbox("Include in “Sync All”", "", true)
	on(it.includeAll, func() {
		a.UpdateProfileSettings(name, func(ps *core.ProfileSettings) { ps.ExcludeFromSyncAll = !ps.ExcludeFromSyncAll })
	})
	it.remind = it.root.AddSubMenuItemCheckbox("Remind Me to Sync", "", true)
	on(it.remind, func() {
		a.UpdateProfileSettings(name, func(ps *core.ProfileSettings) { ps.NoReminder = !ps.NoReminder })
	})
	hide := it.root.AddSubMenuItem("Hide from Menu", "Undo by editing the settings file")
	on(hide, func() {
		a.UpdateProfileSettings(name, func(ps *core.ProfileSettings) { ps.Hidden = true })
		a.rebuildMenu()
	})
	return it
}

func openOrWarn(open func(string) error, path string) {
	if err := open(path); err != nil {
		log.Printf("open %s: %v", path, err)
		notify("Unison Cue", "Could not open "+path+": "+err.Error())
	}
}

// profileView is a snapshot of one profile for display.
type profileView struct {
	title, lastSynced, lastResult string
	interactiveTitle              string
	includeAll, remind, busy      bool
}

// refresh updates titles, check marks, tooltip and icon from the current
// state. The snapshot is taken under a.mu; systray is called without it.
func (a *App) refresh() {
	ui := a.ui
	if ui == nil {
		return
	}
	now := time.Now()

	a.mu.Lock()
	due := map[string]bool{}
	for _, p := range core.DueProfiles(now, a.profiles, a.cfg, a.state) {
		due[p.Name] = true
	}
	views := map[string]profileView{}
	problems := 0
	var problemName string
	for _, p := range a.visibleProfiles() {
		st := a.state.Profiles[p.Name]
		if st == nil {
			st = &core.ProfileState{}
		}
		ps := a.cfg.Profile(p.Name)
		v := profileView{
			includeAll:       !ps.ExcludeFromSyncAll,
			remind:           !ps.NoReminder,
			interactiveTitle: "Sync Interactively in Terminal…",
		}
		ago := core.Ago(now, st.LastSuccess)
		queued := false
		for _, q := range a.queue {
			queued = queued || q == p.Name
		}
		switch {
		case a.running == p.Name:
			v.title = "↻  " + p.DisplayName() + " — syncing…"
			v.busy = true
		case queued:
			v.title = "…  " + p.DisplayName() + " — waiting"
			v.busy = true
		case st.LastStatus == core.StatusFailed:
			v.title = "✕  " + p.DisplayName() + " — last sync failed"
		case st.LastStatus == core.StatusConflicts:
			v.title = "⚠  " + p.DisplayName() + " — conflicts, synced " + ago
		case due[p.Name]:
			v.title = "●  " + p.DisplayName() + " — synced " + ago
		default:
			v.title = "✓  " + p.DisplayName() + " — synced " + ago
		}
		if st.LastStatus == core.StatusFailed || st.LastStatus == core.StatusConflicts {
			problems++
			problemName = p.DisplayName()
		}
		if st.LastStatus == core.StatusConflicts {
			v.interactiveTitle = "Resolve Conflicts in Terminal…"
		}
		if st.LastSuccess.IsZero() {
			v.lastSynced = "Never synced by Unison Cue"
		} else {
			v.lastSynced = "Last synced " + st.LastSuccess.Format("Mon 2 Jan 15:04") + " (" + ago + ")"
		}
		if st.LastRun.IsZero() {
			v.lastResult = "No runs yet"
		} else {
			v.lastResult = "Last run: " + st.LastSummary
		}
		views[p.Name] = v
	}

	nDue := len(due)
	var header string
	state := icon.Idle
	switch {
	case a.unisonPath == "":
		header = "unison not found — see More ▸ Edit Settings File"
		state = icon.Error
	case a.running != "":
		name := a.running
		if p, ok := a.profileByName(name); ok {
			name = p.DisplayName()
		}
		header = "Syncing " + name + "…"
		if n := len(a.queue); n > 0 {
			header += fmt.Sprintf(" (%d more waiting)", n)
		}
		state = icon.Syncing
	case problems == 1:
		header = problemName + " needs attention"
		state = icon.Error
	case problems > 1:
		header = fmt.Sprintf("%d profiles need attention", problems)
		state = icon.Error
	case nDue == 1:
		header = "1 profile is due for a sync"
		state = icon.Due
	case nDue > 1:
		header = fmt.Sprintf("%d profiles are due for a sync", nDue)
		state = icon.Due
	default:
		header = "All profiles up to date"
	}
	busy := a.running != "" || len(a.queue) > 0
	reminderHours := a.cfg.ReminderHours
	snoozed := now.Before(a.state.SnoozeUntil)
	snoozeUntil := a.state.SnoozeUntil
	about := "Unison Cue " + version
	if a.unisonVersion != "" {
		about += " · " + a.unisonVersion
	}
	a.mu.Unlock()

	ui.mu.Lock()
	defer ui.mu.Unlock()
	if ui.header == nil {
		return
	}
	ui.header.SetTitle(header)
	systray.SetTooltip("Unison Cue — " + header)
	if busy {
		ui.cancel.Show()
		ui.syncAll.Disable()
	} else {
		ui.cancel.Hide()
		ui.syncAll.Enable()
	}
	for _, it := range ui.profiles {
		v, ok := views[it.name]
		if !ok {
			continue
		}
		it.root.SetTitle(v.title)
		it.lastSynced.SetTitle(v.lastSynced)
		it.lastResult.SetTitle(v.lastResult)
		it.interactive.SetTitle(v.interactiveTitle)
		setChecked(it.includeAll, v.includeAll)
		setChecked(it.remind, v.remind)
		if v.busy {
			it.syncNow.Disable()
			it.interactive.Disable()
		} else {
			it.syncNow.Enable()
			it.interactive.Enable()
		}
	}
	for h, it := range ui.remindAfter {
		setChecked(it, h == reminderHours)
	}
	if snoozed {
		ui.snoozeRoot.SetTitle("Reminders Snoozed Until " + formatUntil(now, snoozeUntil))
		ui.snoozeOff.Show()
	} else {
		ui.snoozeRoot.SetTitle("Snooze Reminders")
		ui.snoozeOff.Hide()
	}
	setChecked(ui.autostart, platform.AutostartEnabled())
	ui.about.SetTitle(about)

	if !ui.iconSet || ui.lastIcon != state {
		ui.frame = icon.SyncFrames / 2 // start the animation half full
		setTrayIcon(state, ui.frame)
		ui.lastIcon, ui.iconSet = state, true
	}
}

func setTrayIcon(state icon.State, frame int) {
	tmpl, reg := platform.TrayIcon(state, frame)
	if platform.UsesTemplateIcons {
		systray.SetTemplateIcon(tmpl, reg)
	} else {
		systray.SetIcon(reg)
	}
}

// animate steps the syncing animation: the U fills up from the bottom,
// one cycle every 1.2 seconds. It runs for the life of the app and only
// touches the icon while the state is Syncing.
func (a *App) animate() {
	t := time.NewTicker(150 * time.Millisecond)
	defer t.Stop()
	for range t.C {
		ui := a.ui
		ui.mu.Lock()
		if ui.iconSet && ui.lastIcon == icon.Syncing {
			ui.frame = (ui.frame + 1) % icon.SyncFrames
			setTrayIcon(icon.Syncing, ui.frame)
		}
		ui.mu.Unlock()
	}
}

func setChecked(it *systray.MenuItem, on bool) {
	if on != it.Checked() {
		if on {
			it.Check()
		} else {
			it.Uncheck()
		}
	}
}

func formatUntil(now, t time.Time) string {
	if t.YearDay() == now.YearDay() && t.Year() == now.Year() {
		return t.Format("15:04")
	}
	return t.Format("Mon 15:04")
}
