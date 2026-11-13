// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Håvard Dagenborg

package core

import (
	"fmt"
	"strings"
	"time"
)

// IsDue reports whether a profile should be synced: it never completed a
// sync, or the last completed sync is older than interval.
func IsDue(now, lastSuccess time.Time, interval time.Duration) bool {
	return lastSuccess.IsZero() || now.Sub(lastSuccess) >= interval
}

// DueProfiles returns the visible, reminder-enabled profiles that are due.
func DueProfiles(now time.Time, profiles []Profile, cfg *Config, st *State) []Profile {
	var due []Profile
	for _, p := range profiles {
		ps := cfg.Profile(p.Name)
		if ps.Hidden || ps.NoReminder {
			continue
		}
		var last time.Time
		if s := st.Profiles[p.Name]; s != nil {
			last = s.LastSuccess
		}
		if IsDue(now, last, cfg.ReminderInterval(p.Name)) {
			due = append(due, p)
		}
	}
	return due
}

// ShouldNotify decides whether to show a reminder notification now.
// The menu icon always shows the due state; notifications are rate
// limited by renotify and can be snoozed.
func ShouldNotify(now time.Time, due int, lastNotified, snoozeUntil time.Time, renotify time.Duration) bool {
	if due == 0 || now.Before(snoozeUntil) {
		return false
	}
	return lastNotified.IsZero() || now.Sub(lastNotified) >= renotify
}

// ReminderText builds the notification body for the due profiles.
func ReminderText(now time.Time, due []Profile, st *State) string {
	if len(due) == 1 {
		p := due[0]
		var last time.Time
		if s := st.Profiles[p.Name]; s != nil {
			last = s.LastSuccess
		}
		if last.IsZero() {
			return fmt.Sprintf("“%s” has not been synced yet.", p.DisplayName())
		}
		return fmt.Sprintf("“%s” was last synced %s.", p.DisplayName(), Ago(now, last))
	}
	names := make([]string, 0, len(due))
	for _, p := range due {
		names = append(names, p.DisplayName())
	}
	return fmt.Sprintf("%d profiles may need syncing: %s.", len(due), strings.Join(names, ", "))
}

// NextMorning returns 09:00 on the day after now, local time. Used for
// "snooze until tomorrow".
func NextMorning(now time.Time) time.Time {
	y, m, d := now.Date()
	return time.Date(y, m, d+1, 9, 0, 0, 0, now.Location())
}

// Ago formats the time since t in a short human form.
func Ago(now, t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return plural(int(d/time.Minute), "minute") + " ago"
	case d < 24*time.Hour:
		return plural(int(d/time.Hour), "hour") + " ago"
	case d < 14*24*time.Hour:
		return plural(int(d/(24*time.Hour)), "day") + " ago"
	case d < 60*24*time.Hour:
		return plural(int(d/(7*24*time.Hour)), "week") + " ago"
	}
	return "on " + t.Format("2 Jan 2006")
}

func plural(n int, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	return fmt.Sprintf("%d %ss", n, unit)
}

// FormatHours renders a reminder interval for menus ("12 hours", "3 days").
func FormatHours(h float64) string {
	if h >= 24 && int(h)%24 == 0 {
		d := int(h) / 24
		if d%7 == 0 {
			return plural(d/7, "week")
		}
		return plural(d, "day")
	}
	return plural(int(h), "hour")
}
