//go:build linux

package wake

import (
	"testing"
	"time"
)

func TestWaitEitherZeroTimeoutReportsNothing(t *testing.T) {
	p, err := NewPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	if a, b := WaitEither(-1, p.ReadFD(), 0); a || b {
		t.Fatalf("idle pipe reported ready: a=%v b=%v", a, b)
	}
}

func TestSignalReleasesWait(t *testing.T) {
	p, err := NewPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	p.Signal()
	if _, ready := WaitEither(-1, p.ReadFD(), -1); !ready {
		t.Fatal("signalled pipe did not report ready")
	}

	p.Drain()
	if _, ready := WaitEither(-1, p.ReadFD(), 0); ready {
		t.Fatal("drained pipe still reports ready")
	}
}

func TestSignalFromAnotherGoroutine(t *testing.T) {
	p, err := NewPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	go func() {
		time.Sleep(10 * time.Millisecond)
		p.Signal()
	}()

	// Bounded rather than indefinite so a regression fails instead of hanging.
	if _, ready := WaitEither(-1, p.ReadFD(), 2*time.Second); !ready {
		t.Fatal("wait was not released by Signal")
	}
}

func TestWaitTimesOut(t *testing.T) {
	p, err := NewPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	start := time.Now()
	if Wait(p.ReadFD(), 20*time.Millisecond) {
		t.Fatal("idle pipe reported ready")
	}
	if elapsed := time.Since(start); elapsed < 10*time.Millisecond {
		t.Fatalf("returned after %v, expected to wait out the timeout", elapsed)
	}
}

func TestSignalsCoalesce(t *testing.T) {
	p, err := NewPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	// More signals than the pipe buffer holds must not block.
	for range 1 << 17 {
		p.Signal()
	}
	if _, ready := WaitEither(-1, p.ReadFD(), 0); !ready {
		t.Fatal("signalled pipe did not report ready")
	}
	p.Drain()
	if _, ready := WaitEither(-1, p.ReadFD(), 0); ready {
		t.Fatal("drained pipe still reports ready")
	}
}

func TestZeroPipeIsInert(t *testing.T) {
	var p Pipe
	if fd := p.ReadFD(); fd != -1 {
		t.Fatalf("zero Pipe read fd = %d, want -1", fd)
	}
	p.Signal()
	p.Drain()
	p.Close()
	p.Close()
}
