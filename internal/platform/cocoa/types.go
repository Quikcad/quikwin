package cocoa

// The types the public pkg/window/cocoa package aliases. They carry no build
// tag: pkg/window/cocoa is importable from cross-platform code, so the names
// have to exist everywhere even though only the darwin build acts on them.

// TitlebarStyle controls the macOS titlebar appearance.
type TitlebarStyle uint8

const (
	TitlebarDefault     TitlebarStyle = iota
	TitlebarHidden                    // no titlebar, no traffic lights
	TitlebarTransparent               // transparent titlebar, content extends underneath
)

// MenuItem is one entry in the native menu bar.
type MenuItem struct {
	Label string
	// Shortcut is the chord that runs the entry, as "Cmd+S", "Shift+Cmd+Z" or
	// "⌘S". A bare key ("S") takes Command, the macOS default for a menu.
	// Recognised modifiers are Cmd/Command/⌘, Shift/⇧, Alt/Opt/Option/⌥ and
	// Ctrl/Control/⌃; recognised keys are any single character plus the named
	// ones (Left, Delete, Return, F1…). An unrecognised chord leaves the entry
	// without a shortcut rather than binding the wrong one.
	Shortcut string
	Action   func()
	Children []MenuItem
	// Checked draws a checkmark, for an entry that says which way a setting is
	// currently set.
	Checked bool
	// Disabled greys the entry and stops it firing. Negative so that the zero
	// value is a live entry: menus built before this field existed keep working.
	Disabled  bool
	Separator bool
}
