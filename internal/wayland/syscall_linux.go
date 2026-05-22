//go:build linux

package wayland

import (
	"syscall"
	"unsafe"
)

func sysRead(fd int, b []byte) (int, error) {
	return syscall.Read(fd, b)
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
