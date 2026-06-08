package wayland

import "github.com/Quikcad/quikwin/pkg/window"

// WaylandWindow exposes Wayland-specific window capabilities.
// Check at runtime: if wl, ok := win.(wayland.WaylandWindow); ok { ... }
type WaylandWindow interface {
	window.Window

	// WlDisplay returns the raw wl_display* as a uintptr for interop.
	WlDisplay() uintptr

	// WlSurface returns the raw wl_surface* as a uintptr for interop.
	WlSurface() uintptr

	// SetAppID sets the xdg-shell app_id (used by compositors for window grouping).
	SetAppID(id string)

	// IsMaximized reports the toplevel's maximized state from the latest
	// compositor configure.
	IsMaximized() bool

	// ClientDecorated reports whether the client draws its own decorations
	// (server-side decorations are off).
	ClientDecorated() bool

	// Minimize requests that the compositor minimize the window.
	Minimize()

	// ToggleMaximize toggles the window's maximized state.
	ToggleMaximize()
}
