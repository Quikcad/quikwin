//go:build linux

package x11

// Clipboard: in-process fallback. Copy/paste within the same window works; this
// does not yet own the X11 CLIPBOARD selection, so it does not exchange text
// with other applications. Native X11 selection support (XSetSelectionOwner +
// SelectionRequest replies, XConvertSelection + SelectionNotify reads) is a
// follow-up; the selection/atom FFI scaffolding for it already exists here.

var _ interface {
	ClipboardText() string
	SetClipboardText(string)
} = (*window)(nil)

func (w *window) ClipboardText() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.clipText
}

func (w *window) SetClipboardText(text string) {
	w.mu.Lock()
	w.clipText = text
	w.mu.Unlock()
}
