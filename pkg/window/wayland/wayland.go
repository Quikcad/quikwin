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
}
