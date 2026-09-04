//go:build linux

// Package wake supplies the blocking-wait primitives the Linux backends share:
// a ppoll over one or two descriptors, and a self-pipe that releases a blocked
// wait from another goroutine.
package wake

import (
	"fmt"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

const pollIn = 0x0001

type pollfd struct {
	fd      int32
	events  int16
	revents int16
}

// Wait blocks until fd is readable or timeout elapses, reporting whether it
// became readable. A negative timeout blocks indefinitely; a zero timeout
// returns at once.
func Wait(fd int32, timeout time.Duration) bool {
	ready, _ := WaitEither(fd, -1, timeout)
	return ready
}

// WaitEither blocks until a or b is readable or timeout elapses, reporting
// which became readable. A negative descriptor is ignored by the kernel and
// never reports ready, so a caller with one descriptor can pass -1 for the
// other.
//
// ppoll rather than poll: arm64 and the other newer Linux architectures were
// never given the legacy poll syscall. ppoll exists on all of them, and
// syscall.Timespec is sized per architecture, so 32-bit targets need no
// separate path.
func WaitEither(a, b int32, timeout time.Duration) (aReady, bReady bool) {
	fds := [2]pollfd{{fd: a, events: pollIn}, {fd: b, events: pollIn}}
	var (
		n     uintptr
		errno syscall.Errno
	)
	if timeout < 0 {
		// A nil timespec blocks indefinitely.
		n, _, errno = syscall.Syscall6(syscall.SYS_PPOLL,
			uintptr(unsafe.Pointer(&fds[0])), uintptr(len(fds)), 0, 0, 0, 0)
	} else {
		ts := syscall.NsecToTimespec(int64(timeout))
		n, _, errno = syscall.Syscall6(syscall.SYS_PPOLL,
			uintptr(unsafe.Pointer(&fds[0])), uintptr(len(fds)),
			uintptr(unsafe.Pointer(&ts)), 0, 0, 0)
	}
	if n == 0 || errno != 0 {
		return false, false
	}
	return fds[0].revents != 0, fds[1].revents != 0
}

// Pipe is a self-pipe. A WaitEither blocked on ReadFD returns as soon as
// another goroutine calls Signal. The zero value is inert: ReadFD reports -1
// and every other method is a no-op, so a backend that failed to build one
// still behaves like a non-blocking poll.
type Pipe struct {
	mu     sync.Mutex
	r, w   int
	opened bool
}

// NewPipe creates a close-on-exec, non-blocking self-pipe.
func NewPipe() (*Pipe, error) {
	var fds [2]int
	if err := syscall.Pipe2(fds[:], syscall.O_CLOEXEC|syscall.O_NONBLOCK); err != nil {
		return nil, fmt.Errorf("quikwin/wake: pipe2: %w", err)
	}
	return &Pipe{
		r:      fds[0],
		w:      fds[1],
		opened: true,
	}, nil
}

// ReadFD is the descriptor to poll, or -1 once closed.
func (p *Pipe) ReadFD() int32 {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.opened {
		return -1
	}
	return int32(p.r)
}

// Signal makes the read end readable. Safe from any goroutine. Signals
// coalesce — a pipe with bytes already in it is already a pending wake-up, so
// a write that would block is dropped rather than waited on.
func (p *Pipe) Signal() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.opened {
		return
	}
	var b [1]byte
	_, _ = syscall.Write(p.w, b[:])
}

// Drain consumes every pending signal.
func (p *Pipe) Drain() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.opened {
		return
	}
	var buf [64]byte
	for {
		n, err := syscall.Read(p.r, buf[:])
		if n < len(buf) || err != nil {
			return
		}
	}
}

// Close is idempotent.
func (p *Pipe) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.opened {
		return
	}
	p.opened = false
	_ = syscall.Close(p.r)
	_ = syscall.Close(p.w)
}
