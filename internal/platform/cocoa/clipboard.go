//go:build darwin

package cocoa

// Clipboard: in-process fallback. Copy/paste within the same window works; it
// does not yet exchange text with other applications via NSPasteboard. That
// native path is a follow-up.

var _ interface {
	ClipboardText() string
	SetClipboardText(string)
} = (*window)(nil)

func (w *window) ClipboardText() string {
	return w.clipText
}

func (w *window) SetClipboardText(text string) {
	w.clipText = text
}
