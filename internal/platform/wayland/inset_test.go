//go:build linux

package wayland

import (
	"testing"

	"github.com/Quikcad/quikwin/internal/platform/wtypes"
)

// floating is a window sized by the compositor, with a margin declared around
// it. The methods under test read and write struct fields and take the window's
// own mutex; none of them touches the connection, which is what lets the whole
// inset rule be checked without a compositor.
func floating(inset float64, geomW, geomH uint32) *window {
	w := &window{
		resizable:      true,
		inset:          wtypes.MakeFrameInset(inset),
		geomW:          geomW,
		geomH:          geomH,
		geomConfigured: true,
	}
	w.syncSurfaceSizeLocked()
	w.resizePending = false
	return w
}

// The surface is the window plus a margin on all four sides, which is the
// arithmetic everything else here rests on: the compositor sizes the window and
// the client paints the surface.
func TestTheSurfaceIsTheWindowPlusItsMargin(t *testing.T) {
	w := floating(19, 800, 600)
	if gw, gh := w.Size(); gw != 838 || gh != 638 {
		t.Errorf("surface is %dx%d, want 838x638", gw, gh)
	}
}

// A window flush against a screen edge has no margin: the shadow would be
// painted into space the compositor has given to something else, so the client
// does not paint it and the geometry must not claim it.
//
// The backend decides this rather than the client because the state and the
// size arrive in the same configure. A client told the size first would inflate
// it by a margin it is about to drop.
func TestAMaximizedWindowHasNoMargin(t *testing.T) {
	for _, tc := range []struct {
		name string
		set  func(*window)
	}{
		{"maximized", func(w *window) { w.maximized = true }},
		{"tiled", func(w *window) { w.tiled = true }},
		{"fullscreen", func(w *window) { w.fullscreen = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := floating(19, 2560, 1400)
			tc.set(w)
			w.syncSurfaceSizeLocked()

			if in := w.effectiveInset(); in != (wtypes.FrameInset{}) {
				t.Errorf("the inset is %+v, want nothing", in)
			}
			if gw, gh := w.Size(); gw != 2560 || gh != 1400 {
				t.Errorf("surface is %dx%d, want the 2560x1400 it was given", gw, gh)
			}
		})
	}
}

// The size the compositor configured is kept as the geometry it is, so a margin
// that comes and goes is applied to that rather than to a width that already
// carries the old one. Added to the surface instead, a window gains a margin
// every time the shadow is turned off and on.
func TestAMarginIsAppliedToTheConfiguredSizeAndNotToTheLastOne(t *testing.T) {
	w := floating(19, 800, 600)

	w.maximized = true
	w.syncSurfaceSizeLocked()
	w.maximized = false
	w.syncSurfaceSizeLocked()

	if gw, gh := w.Size(); gw != 838 || gh != 638 {
		t.Errorf("after a round trip through maximized the surface is %dx%d, want 838x638", gw, gh)
	}
}

// Nothing to derive a surface from before the first configure that carried a
// size: width and height are what the caller asked for, and a declared inset
// must not move them.
func TestAnUnconfiguredWindowKeepsTheSizeItAskedFor(t *testing.T) {
	w := &window{
		width:  800,
		height: 600,
		inset:  wtypes.MakeFrameInset(19),
	}
	if w.syncSurfaceSizeLocked() {
		t.Error("the surface was resized from a geometry the compositor never sent")
	}
	if gw, gh := w.Size(); gw != 800 || gh != 600 {
		t.Errorf("surface is %dx%d, want the 800x600 asked for", gw, gh)
	}
}

// The resize border belongs on the window's edge. Measured against the surface
// it sits out where the shadow fades, with a dead strip the pointer crosses on
// the way in — which is what a margin looks like from outside.
func TestTheResizeBorderIsOnTheWindowAndNotTheSurface(t *testing.T) {
	const m = 19
	w := floating(m, 800, 600)

	for _, tc := range []struct {
		name string
		x, y float64
		want wtypes.ResizeEdge
	}{
		{"the window's top-left corner", m + 1, m + 1, wtypes.EdgeTopLeft},
		{"the window's left edge", m + 1, m + 300, wtypes.EdgeLeft},
		{"the window's bottom-right corner", m + 798, m + 598, wtypes.EdgeBottomRight},
		{"out in the margin", 2, 2, wtypes.EdgeTopLeft},
		{"well inside the window", m + 400, m + 300, wtypes.EdgeNone},
		// Where the border used to be, and the whole of the complaint: a strip
		// a margin deep that looked like the window and resized nothing.
		{"a margin inside the window's edge", m + 25, m + 300, wtypes.EdgeNone},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := w.detectEdge(tc.x, tc.y); got != tc.want {
				t.Errorf("(%v, %v) is edge %d, want %d", tc.x, tc.y, got, tc.want)
			}
		})
	}
}

// A maximized window's border is its surface's, because there is no margin
// between the two.
func TestTheResizeBorderFollowsTheMarginAway(t *testing.T) {
	w := floating(19, 2560, 1400)
	w.maximized = true
	w.syncSurfaceSizeLocked()

	if got := w.detectEdge(1, 1); got != wtypes.EdgeTopLeft {
		t.Errorf("the corner of a maximized window is edge %d, want %d", got, wtypes.EdgeTopLeft)
	}
}

// An inset is rounded to whole pixels, because xdg-shell's geometry is whole
// pixels. Rounding each side on its own is what keeps the two halves adding up
// to what the surface arithmetic took off.
func TestTheInsetIsWholePixels(t *testing.T) {
	l, top, r, b := insetPixels(wtypes.FrameInset{Left: 19.4, Top: 19.5, Right: 0.4, Bottom: 2})
	if l != 19 || top != 20 || r != 0 || b != 2 {
		t.Errorf("got %d %d %d %d, want 19 20 0 2", l, top, r, b)
	}
}
