# filelock

This package is a fork of the internal package
"cmd/go/internal/lockedfile/internal/filelock" providing an advisory file
locking interface. The package attempts to expose filelocks as an interface
similar to `sync.RWMutex`. Internally, this package uses
[flock(2)](https://man.archlinux.org/man/flock.2.en) with fallback to [fcntl(2)
advisory record
locking](https://man.archlinux.org/man/fcntl.2.en#Advisory_record_locking) for
UNIX systems and uses the [win32 file
API](https://docs.microsoft.com/en-us/windows/win32/api/fileapi/) for Windows.

## Caveats

Although this package attempts to provide a cross-platform interface, there are
some minor platform-dependent discrepancies the programmer should be aware of:

 * For POSIX systems that relies on fcntl(2) advisory file locking (e.g.
   Solaris) each process can hold only one read-lock per inode. Additional
   read-locks within the same process will block until the initial lock is
   released.
 * While UNIX systems places advisory file locks, Windows enforces the filelocks
   on any I/O operation performed on the file. That is, an I/O operation on a
   locked file will block until released.
