//go:build !darwin

package cocoa

// Non-darwin stubs so pkg/window/cocoa (which type-aliases these for its
// public surface) stays importable from cross-platform code. The window impl
// and FFI calls live in the darwin-tagged file.

type TitlebarStyle uint8

const (
	TitlebarDefault TitlebarStyle = iota
	TitlebarHidden
	TitlebarTransparent
)

type MenuItem struct {
	Label     string
	Shortcut  string
	Action    func()
	Children  []MenuItem
	Separator bool
}
