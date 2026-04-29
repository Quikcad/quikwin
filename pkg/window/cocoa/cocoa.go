package cocoa

import "github.com/Quikcad/quikwin/pkg/window"

// TitlebarStyle controls the macOS titlebar appearance.
type TitlebarStyle uint8

const (
	TitlebarDefault     TitlebarStyle = iota
	TitlebarHidden                    // no titlebar, no traffic lights
	TitlebarTransparent               // transparent titlebar, content extends underneath
)

// MenuItem represents one item in a native macOS menu.
type MenuItem struct {
	Label    string
	Shortcut string // e.g. "cmd+q"
	Action   func()
	Children []MenuItem // non-empty → submenu
	Separator bool      // true → render as a separator line; other fields ignored
}

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

	// Native menu bar (the application-level dropdown menus).
	SetMenuBar(items []MenuItem)
}
