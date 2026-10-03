//go:build linux

package wayland

import (
	"testing"

	"github.com/Quikcad/quikwin/internal/platform/wtypes"
	"github.com/lukem570/wayland-go/pkg/protocols/staging/cursorshapev1"
	"github.com/lukem570/wayland-go/pkg/wayland"
)

// pointing is an undecorated, resizable window with a margin, far enough along
// that it has a size and a pointer position. cursorDev is nil on a window with
// no compositor, so the tests below exercise the decision — what shape the
// pointer should have, and whether it has changed — rather than the request that
// carries it.
func pointing(inset float64, geomW, geomH uint32) *window {
	w := floating(inset, geomW, geomH)
	w.decorated = false
	return w
}

// claimCursorLocked refuses to claim anything without a cursor device, which a
// window with no compositor has none of. The decision it gates never reaches
// through the pointer, so an empty one stands in for the device here.
func claimable(w *window) {
	w.cursorDev = &cursorshapev1.DeviceV1{}
}

// **R.** The resize cursor stuck. The pointer crossed the window's edge, took
// the resize arrow, and kept it after moving back into the window — until it
// happened to cross a widget that wanted a different shape.
//
// The old applyEdgeCursor set a shape on the border and returned without doing
// anything off it, so nothing took the arrow back; and it wrote past the
// application's choice without recording it, so the application could not take
// it back either — quikgui calls SetCursor only when its own answer changes, and
// its answer had not changed.
func TestTheResizeCursorIsGivenBack(t *testing.T) {
	w := pointing(19, 800, 600)
	w.cursorShape = wtypes.CursorArrow

	// On the window's left edge.
	if got := w.cursorForLocked(20, 320); got != wtypes.CursorHResize {
		t.Fatalf("the cursor on the left edge is %v, want a horizontal resize", got)
	}
	// And back inside it.
	if got := w.cursorForLocked(400, 320); got != wtypes.CursorArrow {
		t.Errorf("the cursor inside the window is %v, want the application's arrow", got)
	}
}

// The application's cursor is the one that comes back, not an arrow: a window
// whose content asked for a crosshair keeps the crosshair after the pointer has
// been out to the border and back.
func TestWhatComesBackIsWhatTheApplicationAskedFor(t *testing.T) {
	w := pointing(19, 800, 600)
	w.cursorShape = wtypes.CursorCrosshair

	if got := w.cursorForLocked(400, 320); got != wtypes.CursorCrosshair {
		t.Errorf("the cursor inside the window is %v, want the crosshair the "+
			"application asked for", got)
	}
}

// The border wins while the pointer is on it. An application answering about the
// widget under the pointer is answering about a widget the pointer is not there
// to use.
func TestTheBorderOverridesTheApplication(t *testing.T) {
	w := pointing(19, 800, 600)
	w.cursorShape = wtypes.CursorIBeam

	if got := w.cursorForLocked(20, 20); got != wtypes.CursorNWSEResize {
		t.Errorf("the cursor on the top-left corner is %v, want a diagonal resize", got)
	}
}

// A decorated window has no invisible border of its own — the compositor draws
// the frame and owns the resize — so nothing overrides the application there.
func TestADecoratedWindowHasNoBorderCursor(t *testing.T) {
	w := pointing(0, 800, 600)
	w.decorated = true
	w.cursorShape = wtypes.CursorArrow

	if got := w.cursorForLocked(1, 1); got != wtypes.CursorArrow {
		t.Errorf("the corner of a decorated window is %v, want the application's arrow", got)
	}
}

// Sent only on a change: this runs on every motion event, and a cursor that is
// already an arrow does not need telling sixty times a second. The claim is also
// what makes the withdrawal work, so it has to say yes the first time.
func TestTheCompositorIsToldOnlyWhenTheShapeChanges(t *testing.T) {
	w := pointing(19, 800, 600)
	claimable(w)
	w.cursorShape = wtypes.CursorArrow

	if !w.claimCursorLocked(wtypes.CursorArrow) {
		t.Error("the first claim said nothing needed sending; before it, the " +
			"compositor has not been told anything")
	}
	if w.claimCursorLocked(wtypes.CursorArrow) {
		t.Error("the same shape was sent twice")
	}
	if !w.claimCursorLocked(wtypes.CursorHResize) {
		t.Error("a changed shape was not sent")
	}
	if !w.claimCursorLocked(wtypes.CursorArrow) {
		t.Error("the shape changed back and was not sent; this is the withdrawal")
	}
}

// A pointer that has left the surface takes the cursor with it: the shape is
// whoever it is over now, so what we last set is not what is showing. Remembered
// across the gap, the next enter would be a no-op and the pointer would arrive
// carrying the last client's cursor.
func TestLeavingTheSurfaceForgetsWhatWasShowing(t *testing.T) {
	w := pointing(19, 800, 600)
	claimable(w)

	w.claimCursorLocked(wtypes.CursorArrow)
	w.HandlePointerLeave(wayland.PointerLeaveEvent{})

	if !w.claimCursorLocked(wtypes.CursorArrow) {
		t.Error("the same shape was not re-sent after the pointer left and came " +
			"back; the window would show whatever the last client set")
	}
}

// A hidden pointer is not a shape, so there is nothing for the next claim to
// match — and ShowCursor has to put something back.
func TestHidingForgetsWhatWasShowing(t *testing.T) {
	w := pointing(19, 800, 600)
	claimable(w)

	w.claimCursorLocked(wtypes.CursorArrow)
	w.cursorHidden = true
	w.cursorSet = false

	w.cursorHidden = false
	if !w.claimCursorLocked(wtypes.CursorArrow) {
		t.Error("nothing was sent after the cursor was shown again")
	}
}

// And nothing is sent while it is hidden, or the application gets its pointer
// back the first time it moves.
func TestNothingIsSentWhileTheCursorIsHidden(t *testing.T) {
	w := pointing(19, 800, 600)
	claimable(w)
	w.cursorHidden = true

	if w.claimCursorLocked(wtypes.CursorHResize) {
		t.Error("a shape was sent to a hidden pointer")
	}
}
