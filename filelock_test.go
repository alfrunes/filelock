// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !js && !plan9

package filelock

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

var isPosix bool

func init() {
	type posixLockType interface {
		op() int
		cmd() int16
	}
	var lt interface{} = readLock
	_, isPosix = lt.(posixLockType)
}

func doLock(t *testing.T, f *os.File) {
	t.Helper()
	err := Lock(f)
	t.Logf("Lock(fd %d) = %v", f.Fd(), err)
	if err != nil {
		t.Fail()
	}
}

func doTryLock(t *testing.T, f *os.File, shouldBlock bool) {
	t.Helper()
	err := TryLock(f)
	t.Logf("TryLock(fd %d) = %v", f.Fd(), err)
	if err != nil {
		if shouldBlock && err == ErrWouldBlock {
			return
		}
		t.Fail()
	}
}

func doRLock(t *testing.T, f *os.File) {
	t.Helper()
	err := RLock(f)
	t.Logf("RLock(fd %d) = %v", f.Fd(), err)
	if err != nil {
		t.Fail()
	}
}

func doTryRLock(t *testing.T, f *os.File, shouldBlock bool) {
	t.Helper()
	err := TryRLock(f)
	t.Logf("TryRLock(fd %d) = %v", f.Fd(), err)
	if err != nil {
		if shouldBlock && err == ErrWouldBlock {
			return
		}
		t.Fail()
	}
}

func doUnlock(t *testing.T, f *os.File) {
	t.Helper()
	err := Unlock(f)
	t.Logf("Unlock(fd %d) = %v", f.Fd(), err)
	if err != nil {
		t.Fail()
	}
}

func mustTempFile(t *testing.T) (f *os.File, remove func()) {
	t.Helper()

	base := filepath.Base(t.Name())
	f, err := os.CreateTemp("", base)
	if err != nil {
		t.Fatalf(`os.CreateTemp("", %q) = %v`, base, err)
	}
	t.Logf("fd %d = %s", f.Fd(), f.Name())

	return f, func() {
		f.Close()
		os.Remove(f.Name())
	}
}

func mustOpen(t *testing.T, name string) *os.File {
	t.Helper()

	f, err := os.OpenFile(name, os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("os.Open(%q) = %v", name, err)
	}

	t.Logf("fd %d = os.Open(%q)", f.Fd(), name)
	return f
}

const (
	quiescent            = 10 * time.Millisecond
	probablyStillBlocked = 10 * time.Second
)

func mustBlock(t *testing.T, op string, f *os.File) (wait func(*testing.T)) {
	t.Helper()

	desc := fmt.Sprintf("%s(fd %d)", op, f.Fd())

	done := make(chan struct{})
	go func() {
		t.Helper()
		switch op {
		case "Lock":
			doLock(t, f)
		case "RLock":
			doRLock(t, f)
		default:
			panic("invalid op: " + op)
		}
		close(done)
	}()

	select {
	case <-done:
		t.Fatalf("%s unexpectedly did not block", desc)
		return nil

	case <-time.After(quiescent):
		t.Logf("%s is blocked (as expected)", desc)
		return func(t *testing.T) {
			t.Helper()
			select {
			case <-time.After(probablyStillBlocked):
				t.Fatalf("%s is unexpectedly still blocked", desc)
			case <-done:
			}
		}
	}
}

func TestLockExcludesLock(t *testing.T) {
	t.Parallel()

	f, remove := mustTempFile(t)
	defer remove()

	other := mustOpen(t, f.Name())
	defer other.Close()

	doLock(t, f)
	doTryLock(t, other, true)
	lockOther := mustBlock(t, "Lock", other)
	doUnlock(t, f)
	lockOther(t)
	doUnlock(t, other)
}

func TestLockExcludesRLock(t *testing.T) {
	t.Parallel()

	f, remove := mustTempFile(t)
	defer remove()

	other := mustOpen(t, f.Name())
	defer other.Close()

	doLock(t, f)
	doTryRLock(t, other, true)
	rLockOther := mustBlock(t, "RLock", other)
	doUnlock(t, f)
	rLockOther(t)
	doUnlock(t, other)
}

func TestRLockExcludesOnlyLock(t *testing.T) {
	t.Parallel()

	f, remove := mustTempFile(t)
	defer remove()
	doRLock(t, f)

	f2 := mustOpen(t, f.Name())
	defer f2.Close()

	doUnlockTF := false
	if isPosix {
		// When using POSIX locks (as on Solaris), we can't safely read-lock the
		// same inode through two different descriptors at the same time: when the
		// first descriptor is closed, the second descriptor would still be open but
		// silently unlocked. So a second RLock must block instead of proceeding.
		doTryRLock(t, f2, true)
		lockF2 := mustBlock(t, "RLock", f2)
		doUnlock(t, f)
		lockF2(t)
	} else {
		doTryRLock(t, f2, false)
		doUnlockTF = true
	}

	other := mustOpen(t, f.Name())
	defer other.Close()
	doTryLock(t, other, true)
	lockOther := mustBlock(t, "Lock", other)

	doUnlock(t, f2)
	if doUnlockTF {
		doUnlock(t, f)
	}
	lockOther(t)
	doUnlock(t, other)
}

func TestLockNotDroppedByExecCommand(t *testing.T) {
	switch runtime.GOOS {
	case "js", "ios":
		t.Skip("test requires fork/exec")
	}

	f, remove := mustTempFile(t)
	defer remove()

	doLock(t, f)

	other := mustOpen(t, f.Name())
	defer other.Close()

	// Some kinds of file locks are dropped when a duplicated or forked file
	// descriptor is unlocked. Double-check that the approach used by os/exec does
	// not accidentally drop locks.
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	if err := cmd.Run(); err != nil {
		t.Fatalf("exec failed: %v", err)
	}

	lockOther := mustBlock(t, "Lock", other)
	doUnlock(t, f)
	lockOther(t)
	doUnlock(t, other)
}
