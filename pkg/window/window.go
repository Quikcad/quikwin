package window

type Window interface {
	Size() (width, height uint32)
	// Returns 1.0 on platforms without HiDPI support.
	Scale() float32

	// Lifecycle
	ShouldClose() bool
	PollEvents()

	// Appearance
	SetTitle(title string)
	SetCursor(shape CursorShape)
	HideCursor()
	ShowCursor()
	SetMinSize(w, h uint32)
	SetSize(w, h uint32)

	// Lifecycle cleanup
	Destroy()

	// Window dragging (for virtual title bars)
	BeginDrag()

	// Clipboard
	// ClipboardText returns the system clipboard's text, or "" when it is empty
	// or holds no text.
	ClipboardText() string
	// SetClipboardText publishes text to the system clipboard.
	SetClipboardText(text string)

	// Event registration
	OnResize(fn func(width, height uint32))
	// Only way to render intermediate frames on platforms that block during resize (e.g. macOS).
	OnLiveResize(fn func())
	OnClose(fn func())
	OnFocus(fn func(focused bool))
	OnKey(fn func(key Key, action Action, mods Mod))
	OnChar(fn func(ch rune))
	OnMouseButton(fn func(button Button, action Action, mods Mod))
	OnMouseMove(fn func(x, y float64))
	// OnScroll fires for both scroll wheels and trackpad scrolls. precise is
	// true when the source reports continuous deltas (a trackpad) so callers
	// can distinguish a pan gesture from a discrete wheel zoom.
	OnScroll(fn func(dx, dy float64, precise bool))
	// OnPinch fires for trackpad pinch (magnify) gestures. magnification is
	// the fractional scale change for the frame; positive = grow, negative =
	// shrink. The window emits no pinch events on platforms without gesture
	// support; the callback is then never invoked.
	OnPinch(fn func(magnification float64))
	OnDragBegin(fn func(x, y float64))
	OnDragMove(fn func(x, y float64))
	OnDragEnd(fn func(x, y float64))
	OnDrop(fn func(paths []string))
	OnHitTest(fn func(x, y float64) HitTestResult)
	OnCursorEnter(fn func(x, y float64))
}