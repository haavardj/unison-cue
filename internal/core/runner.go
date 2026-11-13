// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Håvard Dagenborg

package core

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"
)

// Status classifies the outcome of a run.
type Status string

const (
	StatusOK        Status = "ok"        // everything propagated (exit 0)
	StatusConflicts Status = "conflicts" // completed, but some items skipped (exit 1)
	StatusFailed    Status = "failed"    // some or all transfers failed (exit 2/3, or could not start)
	StatusCanceled  Status = "canceled"  // stopped by the user or timeout
)

// Completed reports whether the run counts as a sync for reminder
// purposes. Skipped conflicts still count: the replicas are otherwise in
// sync, and the conflicts are flagged separately.
func (s Status) Completed() bool { return s == StatusOK || s == StatusConflicts }

// Result describes one finished run.
type Result struct {
	Profile  string
	Start    time.Time
	End      time.Time
	ExitCode int
	Status   Status
	Summary  string // one line suitable for a menu or notification
	LogPath  string
}

// Runner runs unison in batch mode for one profile at a time.
type Runner struct {
	UnisonPath string
	ExtraArgs  []string
	LogDir     string
	Timeout    time.Duration
	// AskPass is a program ssh runs to ask the user for a passphrase,
	// password or host key confirmation (see AskPassEnv). Empty disables it.
	AskPass string
	// Env holds extra variables for unison; they override the inherited ones.
	Env map[string]string
}

// AskPassEnvVar marks a process started by ssh as an askpass helper.
const AskPassEnvVar = "UNISON_CUE_ASKPASS"

// AskPassEnv returns the environment that makes ssh call program for
// prompts. SSH_ASKPASS_REQUIRE=force (OpenSSH 8.4+) uses it even without
// an X11 DISPLAY; the marker variable tells our binary to act as the helper.
func AskPassEnv(program string) []string {
	return []string{
		"SSH_ASKPASS=" + program,
		"SSH_ASKPASS_REQUIRE=force",
		AskPassEnvVar + "=1",
	}
}

// Args returns the command-line arguments for a background run.
// -batch never asks questions; conflicts are skipped and reported
// through exit code 1. -ui text keeps GUI builds of unison from opening
// a window.
func (r *Runner) Args(profile string) []string {
	args := []string{profile, "-batch", "-ui", "text"}
	return append(args, r.ExtraArgs...)
}

// LogPath is where the output of the latest run of profile is kept.
func (r *Runner) LogPath(profile string) string {
	return filepath.Join(r.LogDir, profile+".log")
}

// Run syncs one profile and blocks until unison exits or ctx is canceled.
func (r *Runner) Run(ctx context.Context, profile string) Result {
	res := Result{Profile: profile, Start: time.Now(), LogPath: r.LogPath(profile)}
	finish := func(st Status, code int, summary string) Result {
		res.End, res.Status, res.ExitCode, res.Summary = time.Now(), st, code, summary
		return res
	}

	if err := os.MkdirAll(r.LogDir, 0o700); err != nil {
		return finish(StatusFailed, -1, err.Error())
	}
	// Keep the previous log as .1 so a "nothing to do" run does not
	// overwrite the interesting one.
	_ = os.Rename(res.LogPath, res.LogPath+".1")
	logf, err := os.OpenFile(res.LogPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return finish(StatusFailed, -1, err.Error())
	}
	defer logf.Close()

	if r.UnisonPath == "" {
		msg := "unison executable not found; set unisonPath in the settings file"
		fmt.Fprintln(logf, msg)
		return finish(StatusFailed, -1, msg)
	}

	if r.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.Timeout)
		defer cancel()
	}

	args := r.Args(profile)
	fmt.Fprintf(logf, "# %s %s\n# started %s\n\n", r.UnisonPath, strings.Join(args, " "),
		res.Start.Format(time.RFC1123))

	cmd := exec.CommandContext(ctx, r.UnisonPath, args...)
	cmd.Stdin = nil // /dev/null: unison and ssh must never wait for keyboard input
	cmd.Stdout = logf
	cmd.Stderr = logf
	cmd.Env = childEnv()
	if r.AskPass != "" {
		cmd.Env = append(cmd.Env, AskPassEnv(r.AskPass)...)
	}
	// os/exec keeps the last value of a duplicated variable, so these win.
	for _, k := range sortedKeys(r.Env) {
		cmd.Env = append(cmd.Env, k+"="+r.Env[k])
	}
	cmd.WaitDelay = 5 * time.Second
	prepareCmd(cmd)

	err = cmd.Run()
	code := 0
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			code = ee.ExitCode()
		} else {
			code = -1
		}
	}
	fmt.Fprintf(logf, "\n# finished %s, exit code %d\n", time.Now().Format(time.RFC1123), code)
	logf.Sync()

	tail := readTail(res.LogPath, 64*1024)
	switch {
	case ctx.Err() == context.DeadlineExceeded:
		return finish(StatusCanceled, code, fmt.Sprintf("Timed out after %s", r.Timeout))
	case ctx.Err() != nil:
		return finish(StatusCanceled, code, "Canceled")
	case code == -1 && err != nil:
		return finish(StatusFailed, code, "Could not start unison: "+err.Error())
	}
	return finish(StatusFromExit(code), code, Summarize(tail, code))
}

// StatusFromExit maps unison's documented exit codes:
// 0 success, 1 some files skipped, 2 non-fatal failures, 3 fatal error.
func StatusFromExit(code int) Status {
	switch code {
	case 0:
		return StatusOK
	case 1:
		return StatusConflicts
	default:
		return StatusFailed
	}
}

var (
	reComplete = regexp.MustCompile(`Synchronization (?:complete|incomplete) at [0-9:]+\s*\(([^)]*)\)`)
	reNothing  = regexp.MustCompile(`Nothing to do: replicas have not changed`)
	reFatal    = regexp.MustCompile(`(?m)^Fatal error:?\s*(.*)$`)
)

// Summarize extracts a one-line summary from unison's text output.
func Summarize(output string, code int) string {
	if m := reComplete.FindAllStringSubmatch(output, -1); m != nil {
		return capitalize(m[len(m)-1][1])
	}
	if reNothing.MatchString(output) {
		return "Nothing to do"
	}
	if m := reFatal.FindAllStringSubmatch(output, -1); m != nil {
		msg := strings.TrimSpace(m[len(m)-1][1])
		if msg == "" {
			msg = "fatal error"
		}
		return "Error: " + msg
	}
	// Fall back to the last line unison printed (skipping our trailer).
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		l := strings.TrimSpace(lines[i])
		if l != "" && !strings.HasPrefix(l, "#") {
			return truncate(l, 120)
		}
	}
	return fmt.Sprintf("unison exited with code %d", code)
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func readTail(path string, n int64) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	if fi, err := f.Stat(); err == nil && fi.Size() > n {
		f.Seek(-n, io.SeekEnd)
	}
	b, _ := io.ReadAll(f)
	return string(b)
}

// extraPathDirs are added to PATH for the child. Apps started from the
// Finder or at login get a minimal PATH without Homebrew, so unison
// could not find ssh or a Homebrew-installed unison on the server side
// of a local root.
func extraPathDirs() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{"/opt/homebrew/bin", "/usr/local/bin", "/opt/local/bin"}
	case "windows":
		return []string{filepath.Join(os.Getenv("SystemRoot"), "System32", "OpenSSH")}
	}
	return []string{"/usr/local/bin"}
}

func childEnv() []string {
	env := os.Environ()
	path := os.Getenv("PATH")
	parts := filepath.SplitList(path)
	have := map[string]bool{}
	for _, p := range parts {
		have[p] = true
	}
	for _, d := range extraPathDirs() {
		if !have[d] {
			parts = append(parts, d)
		}
	}
	newPath := strings.Join(parts, string(os.PathListSeparator))
	out := env[:0:0]
	for _, e := range env {
		if k, _, _ := strings.Cut(e, "="); strings.EqualFold(k, "PATH") {
			continue
		}
		out = append(out, e)
	}
	return append(out, "PATH="+newPath)
}

// FindUnison returns the unison executable to use: the configured path
// if set, else the first match on PATH or in the usual install places.
func FindUnison(configured string) string {
	if configured != "" {
		return configured
	}
	name := "unison"
	if runtime.GOOS == "windows" {
		name = "unison.exe"
	}
	if p, err := exec.LookPath(name); err == nil {
		if abs, err := filepath.Abs(p); err == nil {
			return abs
		}
		return p
	}
	var candidates []string
	switch runtime.GOOS {
	case "darwin":
		candidates = []string{
			"/opt/homebrew/bin/unison",
			"/usr/local/bin/unison",
			"/opt/local/bin/unison",
			"/Applications/Unison.app/Contents/MacOS/cltool",
		}
	case "windows":
		pf := os.Getenv("ProgramFiles")
		la := os.Getenv("LOCALAPPDATA")
		candidates = []string{
			filepath.Join(pf, "Unison", "bin", "unison.exe"),
			filepath.Join(pf, "Unison", "unison.exe"),
			filepath.Join(la, "Programs", "Unison", "bin", "unison.exe"),
			filepath.Join(la, "Microsoft", "WinGet", "Links", "unison.exe"),
		}
		if home, err := os.UserHomeDir(); err == nil {
			candidates = append(candidates, filepath.Join(home, "scoop", "shims", "unison.exe"))
		}
	default:
		candidates = []string{"/usr/local/bin/unison", "/usr/bin/unison"}
	}
	for _, c := range candidates {
		if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
			return c
		}
	}
	return ""
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
