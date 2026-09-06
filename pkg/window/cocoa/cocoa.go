package cocoa

import (
	icocoa "github.com/Quikcad/quikwin/internal/platform/cocoa"
	"github.com/Quikcad/quikwin/pkg/window"
)

// TitlebarStyle controls the macOS titlebar appearance. Aliased to the
// canonical type in internal/platform/cocoa so the impl and the public interface
// reference the same type without forming an import cycle.
type TitlebarStyle = icocoa.TitlebarStyle

const (
	TitlebarDefault     = icocoa.TitlebarDefault
	TitlebarHidden      = icocoa.TitlebarHidden
	TitlebarTransparent = icocoa.TitlebarTransparent
)

// MenuItem represents one item in a native macOS menu. Aliased to the
// canonical type in internal/platform/cocoa.
type MenuItem = icocoa.MenuItem

// CocoaWindow exposes macOS-specific window capabilities.
// Check at runtime: if cw, ok := win.(cocoa.CocoaWindow); ok { ... }
type CocoaWindow interface {
	window.Window

	// Titlebar
	SetTitlebarStyle(style TitlebarStyle)
	SetTitleVisible(visible bool)
	// SetTrafficLightsOffset repositions the close/minimise/zoom buttons.
	// Only meaningful when TitlebarTransparent is active.
	SetTrafficLightsOffset(x, y float32)

	// SetMenuBar installs the application's menu bar. macOS has one menu bar
	// per application rather than per window, so this replaces whatever a
	// previous call installed, from whichever window it was called on.
	//
	// The first menu — the one named after the application, holding Services,
	// Hide and Quit — is supplied by quikwin. items become the menus after it.
	SetMenuBar(items []MenuItem)

	// Window-state controls (custom-chrome apps drive these from their own UI).
	Minimize()
	// ToggleMaximize enters or leaves native full screen, which is what the
	// green button does on a plain click. Full screen binds Escape to leaving
	// it, so an application with its own meaning for that key wants Zoom.
	ToggleMaximize()
	IsMaximized() bool
	// Zoom fills the screen and stays a window — the green button's
	// Option-click behaviour. The system menu bar and the titlebar stay, and
	// the Escape key remains the application's.
	Zoom()
	IsZoomed() bool

	// SetCornerRadius rounds the window's content. Implementations apply this
	// to the CAMetalLayer backing the content view; pair with a transparent
	// background so the corners aren't filled by system chrome.
	SetCornerRadius(r float64)
}
