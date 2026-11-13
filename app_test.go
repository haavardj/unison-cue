// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Håvard Dagenborg

package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/haavardj/unison-cue/internal/core"
	"github.com/haavardj/unison-cue/internal/settings"
)

type notes struct {
	mu   sync.Mutex
	list []string
}

func (n *notes) add(title, body string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.list = append(n.list, title+" | "+body)
	return nil
}

func (n *notes) get() []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return append([]string(nil), n.list...)
}

// newTestApp builds an App over temp directories with two profiles and a
// fake unison that succeeds for "docs" and reports a conflict for "photos".
func newTestApp(t *testing.T) (*App, *notes) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake unison is a shell script")
	}
	dir := t.TempDir()
	prf := filepath.Join(dir, "unison")
	os.MkdirAll(prf, 0o755)
	os.WriteFile(filepath.Join(prf, "docs.prf"), []byte("root = /a\nroot = /b\n"), 0o644)
	os.WriteFile(filepath.Join(prf, "photos.prf"), []byte("root = /c\nroot = /d\nlabel = Photos\n"), 0o644)

	exe := filepath.Join(dir, "unison-fake")
	os.WriteFile(exe, []byte(`#!/bin/sh
case "$1" in
  docs)   echo "Synchronization complete at 10:00:00  (2 items transferred, 0 skipped, 0 failed)"; exit 0 ;;
  photos) echo "Synchronization complete at 10:00:00  (0 items transferred, 1 skipped, 0 failed)"; exit 1 ;;
esac
exit 3
`), 0o755)

	store := &core.Store{Dir: filepath.Join(dir, "cfg")}
	cfg := core.DefaultConfig()
	cfg.UnisonPath = exe
	cfg.UnisonDir = prf
	if err := store.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	n := &notes{}
	old := notifyFunc
	notifyFunc = n.add
	t.Cleanup(func() { notifyFunc = old })

	a, err := NewApp(store)
	if err != nil {
		t.Fatal(err)
	}
	return a, n
}

func waitIdle(t *testing.T, a *App) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		a.mu.Lock()
		idle := a.running == "" && len(a.queue) == 0 && a.batch == nil
		a.mu.Unlock()
		if idle {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("sync queue did not drain")
}

func TestSyncAllAndReminders(t *testing.T) {
	a, n := newTestApp(t)
	if len(a.profiles) != 2 {
		t.Fatalf("profiles = %+v", a.profiles)
	}

	// Never synced: both due, one reminder, and no repeat a minute later.
	a.checkReminders()
	a.checkReminders()
	if got := n.get(); len(got) != 1 || !strings.Contains(got[0], "2 profiles may need syncing") {
		t.Fatalf("reminders = %q", got)
	}

	go a.worker()
	a.SyncAll()
	time.Sleep(50 * time.Millisecond)
	waitIdle(t, a)
	for i := 0; i < 100 && len(n.get()) < 2; i++ {
		time.Sleep(20 * time.Millisecond) // the notification is sent after the queue drains
	}

	got := n.get()
	if len(got) != 2 || !strings.Contains(got[1], "Photos has unresolved conflicts") {
		t.Fatalf("notifications = %q", got)
	}
	a.mu.Lock()
	docs, photos := a.state.Profiles["docs"], a.state.Profiles["photos"]
	a.mu.Unlock()
	if docs.LastStatus != core.StatusOK || docs.LastSuccess.IsZero() || docs.LastSummary != "2 items transferred, 0 skipped, 0 failed" {
		t.Errorf("docs state = %+v", docs)
	}
	if photos.LastStatus != core.StatusConflicts || photos.LastSuccess.IsZero() {
		t.Errorf("photos state = %+v", photos)
	}

	// State survives a restart.
	st := a.store.LoadState()
	if st.Profile("docs").LastStatus != core.StatusOK {
		t.Error("state not persisted")
	}

	// Nothing is due any more, even after the renotify interval.
	a.mu.Lock()
	a.state.LastNotified = time.Now().Add(-24 * time.Hour)
	a.mu.Unlock()
	a.checkReminders()
	if len(n.get()) != 2 {
		t.Errorf("unexpected reminder: %q", n.get())
	}
}

func TestSnoozeAndSettings(t *testing.T) {
	a, n := newTestApp(t)
	a.Snooze(time.Now().Add(time.Hour))
	a.checkReminders()
	if len(n.get()) != 0 {
		t.Errorf("reminder while snoozed: %q", n.get())
	}

	a.UpdateProfileSettings("docs", func(ps *core.ProfileSettings) { ps.NoReminder = true })
	a.SetReminderHours(72)
	cfg, err := a.store.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Profile("docs").NoReminder || cfg.ReminderHours != 72 {
		t.Errorf("settings not saved: %+v", cfg)
	}

	// An external edit to the settings file is picked up on the next tick.
	cfg.Profiles["photos"] = core.ProfileSettings{Hidden: true}
	time.Sleep(10 * time.Millisecond) // make sure the mtime moves
	if err := a.store.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(2 * time.Second)
	os.Chtimes(a.store.ConfigPath(), future, future)
	a.Snooze(time.Time{})
	a.checkReminders()
	a.mu.Lock()
	vis := a.visibleProfiles()
	a.mu.Unlock()
	if len(vis) != 1 || vis[0].Name != "docs" {
		t.Errorf("visible = %+v", vis)
	}
	// docs has reminders off and photos is hidden: nothing to nag about.
	if len(n.get()) != 0 {
		t.Errorf("unexpected reminder: %q", n.get())
	}
}

func TestSettingsBackend(t *testing.T) {
	a, _ := newTestApp(t)
	st := a.Snapshot()
	if len(st.Profiles) != 2 || st.Info.ProfileDir == "" || st.Config.ReminderHours != 24 {
		t.Fatalf("snapshot = %+v", st)
	}

	st.Config.ReminderHours = 0
	if _, errs, _ := a.Apply(settings.Update{Config: st.Config, Autostart: st.Autostart}); errs["reminderHours"] == "" {
		t.Error("invalid reminder accepted")
	}

	st.Config.ReminderHours = 72
	st.Config.Environment = map[string]string{"UNISONLOCALHOSTNAME": "from-settings"}
	st.Config.Profiles["photos"] = core.ProfileSettings{Hidden: true}
	st2, errs, err := a.Apply(settings.Update{Config: st.Config, Autostart: st.Autostart})
	if err != nil || len(errs) > 0 {
		t.Fatalf("apply: %v %v", err, errs)
	}
	if st2.Config.ReminderHours != 72 {
		t.Errorf("not applied: %+v", st2.Config)
	}
	saved, _ := a.store.LoadConfig()
	if saved.ReminderHours != 72 || !saved.Profile("photos").Hidden || saved.Environment["UNISONLOCALHOSTNAME"] != "from-settings" {
		t.Errorf("not saved: %+v", saved)
	}
	var found bool
	for _, v := range st2.Env {
		if v.Name == "UNISONLOCALHOSTNAME" {
			found = v.Value == "from-settings" && v.Source == "settings"
		}
	}
	if !found {
		t.Errorf("effective env = %+v", st2.Env)
	}
}
