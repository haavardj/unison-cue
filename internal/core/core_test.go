// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Håvard Dagenborg

package core

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestDiscoverProfiles(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("work.prf", "# comment\nroot = /Users/me/work\nroot = ssh://host//home/me/work\nlabel = Work files\nignore = Name .DS_Store\n")
	write("docs.prf", "root=/a\n root = /b \n#root = /c\n")
	write("docs.prf~", "root = /backup\n")
	write(".hidden.prf", "")
	write("ar1234", "archive")

	ps, err := DiscoverProfiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 2 {
		t.Fatalf("got %d profiles, want 2: %+v", len(ps), ps)
	}
	if ps[0].Name != "docs" || ps[1].Name != "work" {
		t.Errorf("wrong order/names: %s, %s", ps[0].Name, ps[1].Name)
	}
	if got := strings.Join(ps[0].Roots, "|"); got != "/a|/b" {
		t.Errorf("docs roots = %q", got)
	}
	if ps[1].DisplayName() != "Work files" || len(ps[1].Roots) != 2 {
		t.Errorf("work = %+v", ps[1])
	}
}

func TestSummarize(t *testing.T) {
	cases := []struct{ out, want string }{
		{"...\nSynchronization complete at 10:12:34  (3 items transferred, 1 skipped, 0 failed)\n", "3 items transferred, 1 skipped, 0 failed"},
		{"Synchronization incomplete at 10:12:34  (0 items transferred, 0 skipped, 2 failed)\n  failed: foo\n", "0 items transferred, 0 skipped, 2 failed"},
		{"Looking for changes\nNothing to do: replicas have not changed since last sync.\n", "Nothing to do"},
		{"Contacting server...\nFatal error: Lost connection with the server\n", "Error: Lost connection with the server"},
		{"something odd\n# finished x\n", "something odd"},
		{"", "unison exited with code 3"},
	}
	for _, c := range cases {
		code := 3
		if got := Summarize(c.out, code); got != c.want {
			t.Errorf("Summarize(%q) = %q, want %q", c.out, got, c.want)
		}
	}
}

func TestStatusFromExit(t *testing.T) {
	want := map[int]Status{0: StatusOK, 1: StatusConflicts, 2: StatusFailed, 3: StatusFailed}
	for code, st := range want {
		if got := StatusFromExit(code); got != st {
			t.Errorf("exit %d: %s, want %s", code, got, st)
		}
	}
	if !StatusConflicts.Completed() || StatusFailed.Completed() || StatusCanceled.Completed() {
		t.Error("Completed() wrong")
	}
}

func TestReminders(t *testing.T) {
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.Local)
	cfg := DefaultConfig()
	cfg.ReminderHours = 24
	cfg.SetProfile("weekly", ProfileSettings{ReminderHours: 168})
	cfg.SetProfile("quiet", ProfileSettings{NoReminder: true})
	cfg.SetProfile("gone", ProfileSettings{Hidden: true})
	st := &State{}
	st.Profile("fresh").LastSuccess = now.Add(-2 * time.Hour)
	st.Profile("stale").LastSuccess = now.Add(-25 * time.Hour)
	st.Profile("weekly").LastSuccess = now.Add(-72 * time.Hour)
	profiles := []Profile{{Name: "fresh"}, {Name: "stale"}, {Name: "weekly"}, {Name: "quiet"}, {Name: "gone"}, {Name: "new"}}

	var names []string
	for _, p := range DueProfiles(now, profiles, cfg, st) {
		names = append(names, p.Name)
	}
	if got := strings.Join(names, ","); got != "stale,new" {
		t.Errorf("due = %s, want stale,new", got)
	}

	renotify := 4 * time.Hour
	if ShouldNotify(now, 0, time.Time{}, time.Time{}, renotify) {
		t.Error("notified with nothing due")
	}
	if !ShouldNotify(now, 1, time.Time{}, time.Time{}, renotify) {
		t.Error("first reminder not sent")
	}
	if ShouldNotify(now, 1, now.Add(-time.Hour), time.Time{}, renotify) {
		t.Error("renotified too soon")
	}
	if !ShouldNotify(now, 1, now.Add(-5*time.Hour), time.Time{}, renotify) {
		t.Error("did not renotify")
	}
	if ShouldNotify(now, 1, time.Time{}, now.Add(time.Hour), renotify) {
		t.Error("notified while snoozed")
	}

	if got := ReminderText(now, []Profile{{Name: "stale"}}, st); got != "“stale” was last synced 1 day ago." {
		t.Errorf("text = %q", got)
	}
	if got := NextMorning(now); got.Day() != 14 || got.Hour() != 9 {
		t.Errorf("NextMorning = %v", got)
	}
}

func TestAgoAndHours(t *testing.T) {
	now := time.Now()
	cases := map[time.Duration]string{
		10 * time.Second:    "just now",
		5 * time.Minute:     "5 minutes ago",
		time.Hour:           "1 hour ago",
		50 * time.Hour:      "2 days ago",
		20 * 24 * time.Hour: "2 weeks ago",
	}
	for d, want := range cases {
		if got := Ago(now, now.Add(-d)); got != want {
			t.Errorf("Ago(%v) = %q, want %q", d, got, want)
		}
	}
	if Ago(now, time.Time{}) != "never" {
		t.Error("zero time")
	}
	for h, want := range map[float64]string{6: "6 hours", 24: "1 day", 72: "3 days", 168: "1 week"} {
		if got := FormatHours(h); got != want {
			t.Errorf("FormatHours(%v) = %q, want %q", h, got, want)
		}
	}
}

func TestStoreRoundTrip(t *testing.T) {
	s := &Store{Dir: filepath.Join(t.TempDir(), "cfg")}
	cfg, err := s.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ReminderHours != 24 {
		t.Errorf("default reminder = %v", cfg.ReminderHours)
	}
	if _, err := os.Stat(s.ConfigPath()); err != nil {
		t.Error("default config not written on first load")
	}
	cfg.SetProfile("a", ProfileSettings{NoReminder: true})
	cfg.SetProfile("b", ProfileSettings{})
	if err := s.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	cfg2, _ := s.LoadConfig()
	if !cfg2.Profile("a").NoReminder || len(cfg2.Profiles) != 1 {
		t.Errorf("profiles = %+v", cfg2.Profiles)
	}

	st := s.LoadState()
	when := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	st.Profile("a").LastSuccess = when
	st.Profile("a").LastStatus = StatusConflicts
	if err := s.SaveState(st); err != nil {
		t.Fatal(err)
	}
	st2 := s.LoadState()
	if !st2.Profile("a").LastSuccess.Equal(when) || st2.Profile("a").LastStatus != StatusConflicts {
		t.Errorf("state = %+v", st2.Profile("a"))
	}
}

// fakeUnison writes a shell script that behaves like unison for the
// given output and exit code, and records its arguments.
func fakeUnison(t *testing.T, script string) (exe, argsFile string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake unison is a shell script")
	}
	dir := t.TempDir()
	argsFile = filepath.Join(dir, "args")
	exe = filepath.Join(dir, "unison")
	body := "#!/bin/sh\necho \"$@\" > '" + argsFile + "'\n" + script + "\n"
	if err := os.WriteFile(exe, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return exe, argsFile
}

func TestRunnerResults(t *testing.T) {
	cases := []struct {
		script  string
		status  Status
		summary string
	}{
		{"echo 'Synchronization complete at 10:00:00  (2 items transferred, 0 skipped, 0 failed)'; exit 0", StatusOK, "2 items transferred, 0 skipped, 0 failed"},
		{"echo 'Synchronization complete at 10:00:00  (1 item transferred, 1 skipped, 0 failed)'; echo '  skipped: a.txt' ; exit 1", StatusConflicts, "1 item transferred, 1 skipped, 0 failed"},
		{"echo 'Fatal error: Profile x does not exist' >&2; exit 3", StatusFailed, "Error: Profile x does not exist"},
	}
	for _, c := range cases {
		exe, argsFile := fakeUnison(t, c.script)
		r := &Runner{UnisonPath: exe, LogDir: t.TempDir(), ExtraArgs: []string{"-fastcheck", "true"}}
		res := r.Run(context.Background(), "myprof")
		if res.Status != c.status || res.Summary != c.summary {
			t.Errorf("script %q: got %s %q, want %s %q", c.script, res.Status, res.Summary, c.status, c.summary)
		}
		args, _ := os.ReadFile(argsFile)
		if got := strings.TrimSpace(string(args)); got != "myprof -batch -ui text -fastcheck true" {
			t.Errorf("args = %q", got)
		}
		log, _ := os.ReadFile(res.LogPath)
		if !strings.Contains(string(log), "# finished") {
			t.Errorf("log missing trailer:\n%s", log)
		}
	}
}

func TestRunnerCancelAndTimeout(t *testing.T) {
	exe, _ := fakeUnison(t, "sleep 30")
	r := &Runner{UnisonPath: exe, LogDir: t.TempDir(), Timeout: 300 * time.Millisecond}
	start := time.Now()
	res := r.Run(context.Background(), "p")
	if res.Status != StatusCanceled || !strings.HasPrefix(res.Summary, "Timed out") {
		t.Errorf("timeout: %s %q", res.Status, res.Summary)
	}
	if time.Since(start) > 10*time.Second {
		t.Error("timeout did not kill the process group promptly")
	}

	r.Timeout = 0
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(200*time.Millisecond, cancel)
	res = r.Run(ctx, "p")
	if res.Status != StatusCanceled || res.Summary != "Canceled" {
		t.Errorf("cancel: %s %q", res.Status, res.Summary)
	}
}

func TestRunnerAskPassEnv(t *testing.T) {
	exe, argsFile := fakeUnison(t, `echo "$SSH_ASKPASS|$SSH_ASKPASS_REQUIRE|$UNISON_CUE_ASKPASS" > "$(dirname "$0")/env"`)
	r := &Runner{UnisonPath: exe, LogDir: t.TempDir(), AskPass: "/path/to/helper"}
	r.Run(context.Background(), "p")
	env, _ := os.ReadFile(filepath.Join(filepath.Dir(argsFile), "env"))
	if got := strings.TrimSpace(string(env)); got != "/path/to/helper|force|1" {
		t.Errorf("askpass env = %q", got)
	}
}

func TestRunnerMissingUnison(t *testing.T) {
	r := &Runner{LogDir: t.TempDir()}
	if res := r.Run(context.Background(), "p"); res.Status != StatusFailed {
		t.Errorf("status = %s", res.Status)
	}
	r.UnisonPath = filepath.Join(t.TempDir(), "does-not-exist")
	if res := r.Run(context.Background(), "p"); res.Status != StatusFailed || !strings.HasPrefix(res.Summary, "Could not start") {
		t.Errorf("got %s %q", res.Status, res.Summary)
	}
}

func TestChildEnvAddsPath(t *testing.T) {
	env := childEnv()
	n := 0
	for _, e := range env {
		if strings.HasPrefix(strings.ToUpper(e), "PATH=") {
			n++
			for _, d := range extraPathDirs() {
				if !strings.Contains(e, d) {
					t.Errorf("PATH lacks %s", d)
				}
			}
		}
	}
	if n != 1 {
		t.Errorf("%d PATH entries", n)
	}
}

func TestValidate(t *testing.T) {
	exe, _ := fakeUnison(t, "exit 0")
	good := DefaultConfig()
	good.UnisonPath = exe
	good.UnisonDir = t.TempDir()
	good.Environment = map[string]string{"UNISONLOCALHOSTNAME": "x"}
	good.SetProfile("p", ProfileSettings{ReminderHours: 12})
	if errs := good.Validate(); len(errs) != 0 {
		t.Errorf("valid config rejected: %v", errs)
	}

	bad := DefaultConfig()
	bad.ReminderHours = 0
	bad.RenotifyHours = -1
	bad.TimeoutMinutes = -5
	bad.UnisonPath = "unison" // relative
	bad.UnisonDir = filepath.Join(t.TempDir(), "missing")
	bad.Environment = map[string]string{"UNISON_CUE_SETTINGS_URL": "x"}
	bad.Profiles = map[string]ProfileSettings{"p": {ReminderHours: -1}}
	errs := bad.Validate()
	for _, k := range []string{"reminderHours", "renotifyHours", "timeoutMinutes", "unisonPath", "unisonDir", "environment", "profiles.p.reminderHours"} {
		if errs[k] == "" {
			t.Errorf("no error for %s (got %v)", k, errs)
		}
	}
}

func TestNormalize(t *testing.T) {
	c := DefaultConfig()
	c.Profiles = map[string]ProfileSettings{"a": {}, "b": {NoReminder: true}}
	c.Environment = map[string]string{}
	c.Normalize()
	if len(c.Profiles) != 1 || c.Environment != nil {
		t.Errorf("normalized = %+v", c)
	}
}
