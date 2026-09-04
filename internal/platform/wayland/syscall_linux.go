//go:build linux

package wayland

import "syscall"

func sysPread(fd int, b []byte, off int64) (int, error) {
	return syscall.Pread(fd, b, off)
}

func sysClose(fd int) {
	_ = syscall.Close(fd)
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
