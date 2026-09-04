//go:build windows

package win32

// Clipboard: in-process fallback. Copy/paste within the same window works; it
// does not yet exchange text with other applications via the Win32 clipboard
// (OpenClipboard/SetClipboardData/GetClipboardData). That native path is a
// follow-up.

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
