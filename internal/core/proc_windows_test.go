// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Håvard Dagenborg

package core

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// The test binary doubles as a fake unison (which starts a child, as
// unison starts ssh) and as that child, selected by this variable.
const fakeRoleEnv = "UNISON_CUE_TEST_FAKE"

func TestMain(m *testing.M) {
	switch os.Getenv(fakeRoleEnv) {
	case "unison":
		child := exec.Command(os.Args[0])
		child.Env = append(os.Environ(), fakeRoleEnv+"=ssh")
		if child.Start() == nil {
			child.Wait()
		}
		os.Exit(0)
	case "ssh":
		os.WriteFile(os.Getenv("UNISON_CUE_TEST_PIDFILE"), []byte(strconv.Itoa(os.Getpid())), 0o600)
		time.Sleep(time.Minute)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestCancelKillsChildProcesses(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "child.pid")
	r := &Runner{
		UnisonPath: os.Args[0],
		LogDir:     dir,
		Env:        map[string]string{fakeRoleEnv: "unison", "UNISON_CUE_TEST_PIDFILE": pidFile},
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan Result, 1)
	go func() { done <- r.Run(ctx, "p") }()

	var pid int
	for deadline := time.Now().Add(20 * time.Second); pid == 0; {
		if b, err := os.ReadFile(pidFile); err == nil {
			pid, _ = strconv.Atoi(strings.TrimSpace(string(b)))
		}
		if time.Now().After(deadline) {
			cancel()
			t.Fatal("fake ssh process did not start")
		}
		time.Sleep(50 * time.Millisecond)
	}
	cancel()
	if res := <-done; res.Status != StatusCanceled {
		t.Errorf("status = %s, want %s", res.Status, StatusCanceled)
	}

	h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return // already gone
	}
	defer windows.CloseHandle(h)
	if ev, _ := windows.WaitForSingleObject(h, 5000); ev != windows.WAIT_OBJECT_0 {
		windows.TerminateProcess(h, 1)
		t.Fatal("child process still running after cancel")
	}
}
