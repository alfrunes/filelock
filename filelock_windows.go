// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build windows

package filelock

import (
	"io/fs"
	"syscall"
	"unsafe"
)

var (
	modkernel32      = syscall.NewLazyDLL("kernel32.dll")
	procLockFileEx   = modkernel32.NewProc("LockFileEx")
	procUnlockFileEx = modkernel32.NewProc("UnlockFileEx")
)

const (
	// https://docs.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-lockfileex#parameters
	sysLOCKFILE_EXCLUSIVE_LOCK   = 0x00000002
	sysLOCKFILE_FAIL_IMMEDIATELY = 0x00000001

	// https://docs.microsoft.com/en-us/windows/win32/debug/system-error-codes--0-499-
	errnoERROR_LOCK_VIOLATION       syscall.Errno = 33
	errnoERROR_NOT_SUPPORTED        syscall.Errno = 50
	errnoERROR_CALL_NOT_IMPLEMENTED syscall.Errno = 120

	reserved = 0
	allBytes = ^uint32(0)
)

type lockType uint32

const (
	readLock    lockType = 0x00000000
	readLockNB  lockType = sysLOCKFILE_FAIL_IMMEDIATELY
	writeLock   lockType = sysLOCKFILE_EXCLUSIVE_LOCK
	writeLockNB lockType = sysLOCKFILE_EXCLUSIVE_LOCK | sysLOCKFILE_FAIL_IMMEDIATELY
)

func lock(f File, lt lockType) error {
	// Per https://golang.org/issue/19098, “Programs currently expect the Fd
	// method to return a handle that uses ordinary synchronous I/O.”
	// However, LockFileEx still requires an OVERLAPPED structure,
	// which contains the file offset of the beginning of the lock range.
	// We want to lock the entire file, so we leave the offset as zero.
	ol := new(syscall.Overlapped)

	r1, _, err := procLockFileEx.Call(
		uintptr(syscall.Handle(f.Fd())), uintptr(uint32(lt)),
		uintptr(reserved), uintptr(allBytes),
		uintptr(allBytes), uintptr(unsafe.Pointer(ol)),
	)
	if r1 == 0 {
		if err == errnoERROR_LOCK_VIOLATION || err == syscall.ERROR_IO_PENDING {
			return ErrWouldBlock
		}
		return &fs.PathError{
			Op:   lt.String(),
			Path: f.Name(),
			Err:  err,
		}
	}
	return nil
}

func unlock(f File) error {
	ol := new(syscall.Overlapped)
	r1, _, err := procUnlockFileEx.Call(
		uintptr(syscall.Handle(f.Fd())), uintptr(reserved),
		uintptr(allBytes), uintptr(allBytes), uintptr(unsafe.Pointer(ol)),
	)
	if r1 == 0 {
		return &fs.PathError{
			Op:   "Unlock",
			Path: f.Name(),
			Err:  err,
		}
	}
	return nil
}

func isNotSupported(err error) bool {
	switch err {
	case errnoERROR_NOT_SUPPORTED, errnoERROR_CALL_NOT_IMPLEMENTED, ErrNotSupported:
		return true
	default:
		return false
	}
}
