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

	// IsTiled reports whether the latest compositor configure carried any
	// tiled_* edge (e.g. a KWin half-screen quick-tile). Distinct from
	// maximized and fullscreen.
	IsTiled() bool

	// IsFullscreen reports the toplevel's fullscreen state from the latest
	// compositor configure.
	IsFullscreen() bool

	// ClientDecorated reports whether the client draws its own decorations
	// (server-side decorations are off).
	ClientDecorated() bool

	// SetFrameInset declares how far the window a user sees is inset from the
	// surface the client paints — the margin a client-drawn drop shadow
	// occupies.
	//
	// The compositor is then told the window's own rectangle, so it snaps,
	// tiles and maximizes to the window's edge rather than to the shadow's, and
	// the invisible resize border moves there with it. Nothing else changes
	// coordinate system: [window.Window.Size] is still the buffer to paint and
	// a pointer position is still a point in it. xdg-shell measures the
	// toplevel's size and its minimum in window geometry instead, and that
	// translation lives behind this call.
	//
	// It is not applied while the window is maximized, tiled or fullscreen.
	// Those sit flush against a screen edge, where there is no room for a
	// shadow and a client has nothing to inset. The decision is here rather
	// than the caller's because the state and the size arrive in the same
	// configure: a caller told the size first would inflate it by a margin it
	// is about to drop, and the window would be a frame wrong on every maximize.
	//
	// A side is clamped at zero; see [window.FrameInset].
	SetFrameInset(inset window.FrameInset)

	// Minimize requests that the compositor minimize the window.
	Minimize()

	// ToggleMaximize toggles the window's maximized state.
	ToggleMaximize()
}
