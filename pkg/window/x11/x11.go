package x11

import "github.com/Quikcad/quikwin/pkg/window"

// X11Window exposes X11-specific window capabilities.
// Check at runtime: if xw, ok := win.(x11.X11Window); ok { ... }
type X11Window interface {
	window.Window

	// Display returns the raw *Display pointer as a uintptr for interop.
	Display() uintptr

	// XID returns the X11 Window ID.
	XID() uint32

	// SetNetWMState sets _NET_WM_STATE atoms by name (e.g. "_NET_WM_STATE_ABOVE").
	SetNetWMState(atoms ...string)
}
