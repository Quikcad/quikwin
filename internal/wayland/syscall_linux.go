//go:build linux

package wayland

import "syscall"

func sysRead(fd int, b []byte) (int, error) {
	return syscall.Read(fd, b)
}

func sysClose(fd int) {
	_ = syscall.Close(fd)
}
