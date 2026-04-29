package win32

import "github.com/Quikcad/quikwin/pkg/window"

// Win32Window exposes Windows-specific window capabilities.
// Check at runtime: if ww, ok := win.(win32.Win32Window); ok { ... }
type Win32Window interface {
	window.Window

	// HWND returns the raw Win32 window handle for interop.
	HWND() uintptr

	// SetDarkMode enables or disables the dark-mode title bar (DWMWA_USE_IMMERSIVE_DARK_MODE).
	SetDarkMode(enabled bool)

	// SetSnapLayoutEnabled controls whether the snap-layout flyout appears
	// on hover over the maximize button (Windows 11+).
	SetSnapLayoutEnabled(enabled bool)

	// SetMicaBackground applies the Mica backdrop material (Windows 11+).
	SetMicaBackground(enabled bool)
}
