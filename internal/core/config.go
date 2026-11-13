// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Håvard Dagenborg

// Package core holds the platform-independent logic of Unison Cue:
// configuration, persisted state, profile discovery, running unison and
// deciding when the user should be reminded to sync.
package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// AppName is used for the settings directory and user-visible strings.
const AppName = "Unison Cue"

// ProfileSettings are per-profile overrides. The zero value means
// "use the defaults", so a profile that is not mentioned in the config
// file behaves normally.
type ProfileSettings struct {
	Hidden             bool    `json:"hidden,omitempty"`             // do not show in the menu
	ExcludeFromSyncAll bool    `json:"excludeFromSyncAll,omitempty"` // skip in "Sync all"
	NoReminder         bool    `json:"noReminder,omitempty"`         // never nag about this one
	ReminderHours      float64 `json:"reminderHours,omitempty"`      // 0 = use global value
}

// Config is the user-editable settings file.
type Config struct {
	// UnisonPath is the unison executable. Empty means search PATH and
	// the usual install locations.
	UnisonPath string `json:"unisonPath,omitempty"`
	// UnisonDir is the directory holding *.prf files. Empty means the
	// same directory unison itself uses.
	UnisonDir string `json:"unisonDir,omitempty"`
	// ExtraArgs are appended to every background run.
	ExtraArgs []string `json:"extraArgs,omitempty"`
	// ReminderHours: a profile is "due" when its last successful sync
	// is older than this.
	ReminderHours float64 `json:"reminderHours"`
	// RenotifyHours: minimum time between two reminder notifications.
	RenotifyHours float64 `json:"renotifyHours"`
	// TimeoutMinutes kills a background run that takes longer than this
	// (0 = no limit). Guards against ssh waiting for a password forever.
	TimeoutMinutes float64 `json:"timeoutMinutes,omitempty"`
	// QuietSuccess: no notification when a sync finishes without problems.
	QuietSuccess bool `json:"quietSuccess,omitempty"`
	// NoSSHAskpass: do not show a dialog when ssh needs a passphrase,
	// password or host key confirmation (the run fails instead).
	NoSSHAskpass bool `json:"noSshAskpass,omitempty"`
	// Environment holds extra variables for unison, for example
	// {"UNISONLOCALHOSTNAME": "mylaptop"}. They take precedence over the
	// app's own environment. UNISON here also selects the profile folder.
	Environment map[string]string `json:"environment,omitempty"`
	// NoShellEnvironment: on macOS, do not import UNISON* variables from
	// the login shell at start-up.
	NoShellEnvironment bool `json:"noShellEnvironment,omitempty"`

	Profiles map[string]ProfileSettings `json:"profiles,omitempty"`
}

// DefaultConfig returns the settings used when no file exists.
func DefaultConfig() *Config {
	return &Config{
		ReminderHours: 24,
		RenotifyHours: 4,
		Profiles:      map[string]ProfileSettings{},
	}
}

// Profile returns the settings for one profile (zero value if unset).
func (c *Config) Profile(name string) ProfileSettings {
	return c.Profiles[name]
}

// SetProfile stores settings for one profile, dropping empty entries so
// the file stays small.
func (c *Config) SetProfile(name string, ps ProfileSettings) {
	if c.Profiles == nil {
		c.Profiles = map[string]ProfileSettings{}
	}
	if ps == (ProfileSettings{}) {
		delete(c.Profiles, name)
		return
	}
	c.Profiles[name] = ps
}

// ReminderInterval returns how long a profile may go unsynced.
func (c *Config) ReminderInterval(name string) time.Duration {
	h := c.ReminderHours
	if ps := c.Profile(name); ps.ReminderHours > 0 {
		h = ps.ReminderHours
	}
	if h <= 0 {
		h = 24
	}
	return time.Duration(h * float64(time.Hour))
}

// RenotifyInterval returns the minimum gap between notifications.
func (c *Config) RenotifyInterval() time.Duration {
	h := c.RenotifyHours
	if h <= 0 {
		h = 4
	}
	return time.Duration(h * float64(time.Hour))
}

// Timeout returns the background run limit, 0 for none.
func (c *Config) Timeout() time.Duration {
	if c.TimeoutMinutes <= 0 {
		return 0
	}
	return time.Duration(c.TimeoutMinutes * float64(time.Minute))
}

// ProfileState is what we remember about the last runs of a profile.
type ProfileState struct {
	LastSuccess time.Time `json:"lastSuccess,omitempty"` // last run that completed (exit 0 or 1)
	LastRun     time.Time `json:"lastRun,omitempty"`
	LastStatus  Status    `json:"lastStatus,omitempty"`
	LastSummary string    `json:"lastSummary,omitempty"`
}

// State is machine-written bookkeeping, kept apart from Config so that
// the user can edit the config file without racing the app.
type State struct {
	Profiles     map[string]*ProfileState `json:"profiles"`
	LastNotified time.Time                `json:"lastNotified,omitempty"`
	SnoozeUntil  time.Time                `json:"snoozeUntil,omitempty"`
}

// Profile returns the state for name, creating it if needed.
func (s *State) Profile(name string) *ProfileState {
	if s.Profiles == nil {
		s.Profiles = map[string]*ProfileState{}
	}
	ps := s.Profiles[name]
	if ps == nil {
		ps = &ProfileState{}
		s.Profiles[name] = ps
	}
	return ps
}

// Store reads and writes the config and state files in one directory.
type Store struct {
	Dir string
}

// DefaultStore uses the OS settings location:
// ~/Library/Application Support/Unison Cue on macOS,
// %AppData%\Unison Cue on Windows.
func DefaultStore() (*Store, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(base, AppName)
	// The project was called "Unison Tray" before release; keep its
	// settings and sync history.
	if _, err := os.Stat(dir); errors.Is(err, fs.ErrNotExist) {
		_ = os.Rename(filepath.Join(base, legacyAppName), dir)
	}
	return &Store{Dir: dir}, nil
}

const legacyAppName = "Unison Tray"

func (s *Store) ConfigPath() string { return filepath.Join(s.Dir, "config.json") }
func (s *Store) StatePath() string  { return filepath.Join(s.Dir, "state.json") }
func (s *Store) LogDir() string     { return filepath.Join(s.Dir, "logs") }

// LoadConfig reads the config, writing the defaults on first start so the
// user has a file to edit.
func (s *Store) LoadConfig() (*Config, error) {
	cfg := DefaultConfig()
	err := readJSON(s.ConfigPath(), cfg)
	if errors.Is(err, fs.ErrNotExist) {
		return cfg, s.SaveConfig(cfg)
	}
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]ProfileSettings{}
	}
	return cfg, err
}

func (s *Store) SaveConfig(c *Config) error { return writeJSON(s.ConfigPath(), c) }

// LoadState reads the state; a missing or corrupt file yields empty state.
func (s *Store) LoadState() *State {
	st := &State{}
	_ = readJSON(s.StatePath(), st)
	if st.Profiles == nil {
		st.Profiles = map[string]*ProfileState{}
	}
	return st
}

func (s *Store) SaveState(st *State) error { return writeJSON(s.StatePath(), st) }

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// writeJSON writes atomically so a crash never leaves a truncated file.
func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

var envNameRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Validate checks settings entered in the settings window. It returns
// messages keyed by setting name ("profiles.<name>.reminderHours" for
// per-profile values); an empty map means the settings are valid.
func (c *Config) Validate() map[string]string {
	errs := map[string]string{}
	const maxHours = 24 * 366
	if !(c.ReminderHours > 0 && c.ReminderHours <= maxHours) {
		errs["reminderHours"] = "Enter a period between 1 hour and 1 year."
	}
	if !(c.RenotifyHours > 0 && c.RenotifyHours <= maxHours) {
		errs["renotifyHours"] = "Enter a number of hours greater than 0."
	}
	if !(c.TimeoutMinutes >= 0) {
		errs["timeoutMinutes"] = "Enter 0 or a positive number of minutes."
	}
	if c.UnisonPath != "" {
		if !filepath.IsAbs(c.UnisonPath) {
			errs["unisonPath"] = "Enter the full path, or leave this empty."
		} else if fi, err := os.Stat(c.UnisonPath); err != nil {
			errs["unisonPath"] = "No program at this path."
		} else if fi.IsDir() {
			errs["unisonPath"] = "This is a folder; enter the path of the unison program itself."
		}
	}
	if c.UnisonDir != "" {
		if fi, err := os.Stat(c.UnisonDir); err != nil || !fi.IsDir() {
			errs["unisonDir"] = "No folder at this path."
		}
	}
	for k, v := range c.Environment {
		switch {
		case !envNameRE.MatchString(k):
			errs["environment"] = fmt.Sprintf("“%s” is not a valid variable name (letters, digits and _ only).", k)
		case strings.HasPrefix(k, "UNISON_CUE_"):
			errs["environment"] = fmt.Sprintf("%s is reserved for Unison Cue itself.", k)
		case strings.ContainsRune(v, 0):
			errs["environment"] = fmt.Sprintf("The value of %s contains an invalid character.", k)
		}
	}
	for name, ps := range c.Profiles {
		if !(ps.ReminderHours >= 0 && ps.ReminderHours <= maxHours) {
			errs["profiles."+name+".reminderHours"] = "Enter a number of hours, or leave empty."
		}
	}
	return errs
}

// Normalize drops empty per-profile entries and an empty environment, so
// the saved file only contains settings that differ from the defaults.
func (c *Config) Normalize() {
	for name, ps := range c.Profiles {
		c.SetProfile(name, ps)
	}
	if len(c.Environment) == 0 {
		c.Environment = nil
	}
	if len(c.ExtraArgs) == 0 {
		c.ExtraArgs = nil
	}
}
