//go:build linux

package wayland

import (
	"syscall"
	"time"
	"unsafe"
)

func sysPread(fd int, b []byte, off int64) (int, error) {
	return syscall.Pread(fd, b, off)
}

func sysClose(fd int) {
	_ = syscall.Close(fd)
}

type pollfd struct {
	fd      int32
	events  int16
	revents int16
}

const pollIn = 0x0001

func fdReadable(fd int32) bool {
	return pollWait(fd, 0)
}

func sysWrite(fd int, b []byte) (int, error) { return syscall.Write(fd, b) }

func sysRead(fd int, b []byte) (int, error) { return syscall.Read(fd, b) }

// sysPipe creates a close-on-exec pipe, returning its read and write fds.
func sysPipe() (r, w int, err error) {
	var fds [2]int
	if err := syscall.Pipe2(fds[:], syscall.O_CLOEXEC); err != nil {
		return 0, 0, err
	}
	return fds[0], fds[1], nil
}

// pollWait blocks until fd is readable or timeout elapses, reporting whether it
// became readable. A negative timeout blocks indefinitely; a zero timeout polls.
//
// ppoll rather than poll: arm64 and the other newer Linux architectures were
// never given the legacy poll syscall, and ppoll exists on all of them. Its
// timespec is sized by syscall.Timespec, so 32-bit targets are handled too.
func pollWait(fd int32, timeout time.Duration) bool {
	pfd := pollfd{fd: fd, events: pollIn}
	var tsp uintptr // a nil timespec blocks indefinitely
	if timeout >= 0 {
		ts := syscall.NsecToTimespec(int64(timeout))
		tsp = uintptr(unsafe.Pointer(&ts))
	}
	r, _, _ := syscall.Syscall6(syscall.SYS_PPOLL,
		uintptr(unsafe.Pointer(&pfd)), 1, tsp, 0, 0, 0)
	return r > 0
}
