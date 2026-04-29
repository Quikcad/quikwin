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
	SetMinSize(w, h uint32)
	SetSize(w, h uint32)

	// Lifecycle cleanup
	Destroy()

	// Window dragging (for virtual title bars)
	BeginDrag()

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
	OnScroll(fn func(dx, dy float64))
	OnDragBegin(fn func(x, y float64))
	OnDragMove(fn func(x, y float64))
	OnDragEnd(fn func(x, y float64))
	OnDrop(fn func(paths []string))
}