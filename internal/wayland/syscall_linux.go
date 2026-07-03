//go:build linux

package wayland

import (
	"syscall"
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

func fdReadable(fd int32) bool {
	pfd := pollfd{fd: fd, events: 0x0001} // POLLIN
	r, _, _ := syscall.Syscall(syscall.SYS_POLL, uintptr(unsafe.Pointer(&pfd)), 1, 0)
	return r > 0
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

// pollWait blocks until fd is readable or timeoutMs elapses, reporting whether
// it became readable.
func pollWait(fd int32, timeoutMs int) bool {
	pfd := pollfd{fd: fd, events: 0x0001} // POLLIN
	r, _, _ := syscall.Syscall(syscall.SYS_POLL, uintptr(unsafe.Pointer(&pfd)), 1, uintptr(timeoutMs))
	return r > 0
}
