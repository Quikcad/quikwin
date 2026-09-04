//go:build linux

package wayland

import (
	"slices"
	"strings"
	"time"

	"github.com/Quikcad/quikwin/internal/platform/wake"

	"github.com/lukem570/wayland-go/pkg/wayland"
	"github.com/lukem570/wayland-go/pkg/wl"
)

var (
	_ wayland.DataDeviceHandler = (*window)(nil)
	_ wayland.DataSourceHandler = (*window)(nil)
	_ wayland.DataOfferHandler  = (*window)(nil)
	_ interface {
		ClipboardText() string
		SetClipboardText(string)
	} = (*window)(nil)
)

// clipboardMimes are the text mime types the entry advertises when it owns the
// selection, in descending preference. The same list ranks an incoming offer.
var clipboardMimes = []string{
	"text/plain;charset=utf-8",
	"text/plain",
	"UTF8_STRING",
	"STRING",
	"TEXT",
}

// pickTextMime returns the most preferred text mime among those offered, or the
// first text/* mime, or "" when none is text.
func pickTextMime(offered []string) string {
	for _, want := range clipboardMimes {
		if slices.Contains(offered, want) {
			return want
		}
	}
	for _, have := range offered {
		if strings.HasPrefix(have, "text/") {
			return have
		}
	}
	return ""
}

// SetClipboardText publishes text as the selection. It creates a wl_data_source
// offering the text mime types and owns the selection until another client takes
// it (wl_data_source.cancelled). The text is also kept in memory so a paste from
// this same window short-circuits the round trip.
func (w *window) SetClipboardText(text string) {
	w.mu.Lock()
	w.clipText = text
	w.ownsClip = true
	serial := w.inputSerial
	mgr := w.dataDevMgr
	dev := w.dataDevice
	if w.clipSource != nil {
		w.clipSource.Destroy()
		w.clipSource = nil
	}
	w.mu.Unlock()

	if mgr == nil || dev == nil {
		return // no data device; clipText still serves same-window pastes
	}

	src := mgr.CreateDataSource()
	src.SetHandler(w)
	for _, m := range clipboardMimes {
		src.Offer(m)
	}
	w.mu.Lock()
	w.clipSource = src
	w.mu.Unlock()

	dev.SetSelection(src, serial)
	wl.Flush(w.conn)
}

// ClipboardText returns the selection's text. When this window owns the
// selection it returns the in-memory copy (reading our own pipe would deadlock
// the single event-loop thread); otherwise it requests a transfer from the
// current offer and reads it.
func (w *window) ClipboardText() string {
	w.mu.Lock()
	if w.ownsClip {
		t := w.clipText
		w.mu.Unlock()
		return t
	}
	offer := w.selOffer
	mime := pickTextMime(w.selMimes)
	w.mu.Unlock()

	if offer == nil {
		return ""
	}
	if mime == "" {
		mime = "text/plain;charset=utf-8"
	}
	return w.readOffer(offer, mime)
}

// readOffer transfers the offer's data over a pipe and returns it. The source
// client writes from its own thread, so a blocking read here is safe; a poll
// timeout guards against a source that never writes.
func (w *window) readOffer(offer *wayland.DataOffer, mime string) string {
	r, wr, err := sysPipe()
	if err != nil {
		return ""
	}
	offer.Receive(mime, uintptr(wr))
	wl.Flush(w.conn)
	sysClose(wr) // close our write end so the read sees EOF
	defer sysClose(r)

	var out []byte
	buf := make([]byte, 4096)
	for {
		if !wake.Wait(int32(r), 200*time.Millisecond) {
			break
		}
		n, rerr := sysRead(r, buf)
		if n > 0 {
			out = append(out, buf[:n]...)
		}
		if n == 0 || rerr != nil {
			break
		}
	}
	return string(out)
}

// --- wl_data_source events (we own the selection) ---

func (w *window) HandleDataSourceSend(e wayland.DataSourceSendEvent) {
	w.mu.Lock()
	text := w.clipText
	w.mu.Unlock()
	fd := int(e.FD)
	// Write on its own goroutine: the pipe may block if the reader is slow, and
	// this fires from the event loop.
	go func() {
		b := []byte(text)
		for len(b) > 0 {
			n, err := sysWrite(fd, b)
			if n > 0 {
				b = b[n:]
			}
			if err != nil {
				break
			}
		}
		sysClose(fd)
	}()
}

func (w *window) HandleDataSourceCancelled(e wayland.DataSourceCancelledEvent) {
	w.mu.Lock()
	if w.clipSource != nil {
		w.clipSource.Destroy()
		w.clipSource = nil
	}
	w.ownsClip = false
	w.mu.Unlock()
}

func (w *window) HandleDataSourceTarget(e wayland.DataSourceTargetEvent)                     {}
func (w *window) HandleDataSourceAction(e wayland.DataSourceActionEvent)                     {}
func (w *window) HandleDataSourceDnDDropPerformed(e wayland.DataSourceDnDDropPerformedEvent) {}
func (w *window) HandleDataSourceDnDFinished(e wayland.DataSourceDnDFinishedEvent)           {}

// --- wl_data_device events (another client offers a selection) ---

func (w *window) HandleDataDeviceDataOffer(e wayland.DataDeviceDataOfferEvent) {
	offer := wayland.NewDataOffer(e.ID)
	offer.SetHandler(w)
	w.mu.Lock()
	w.pendingOffer = offer
	w.pendingMimes = nil
	w.mu.Unlock()
}

func (w *window) HandleDataDeviceSelection(e wayland.DataDeviceSelectionEvent) {
	w.mu.Lock()
	switch {
	case e.ID == nil:
		w.selOffer = nil
		w.selMimes = nil
	case w.pendingOffer != nil && w.pendingOffer.Proxy() == e.ID:
		w.selOffer = w.pendingOffer
		w.selMimes = w.pendingMimes
	default:
		w.selOffer = wayland.NewDataOffer(e.ID)
		w.selMimes = nil
	}
	// A selection we did not publish means we no longer own the clipboard.
	if w.clipSource == nil {
		w.ownsClip = false
	}
	w.mu.Unlock()
}

func (w *window) HandleDataDeviceEnter(e wayland.DataDeviceEnterEvent)   {}
func (w *window) HandleDataDeviceLeave(e wayland.DataDeviceLeaveEvent)   {}
func (w *window) HandleDataDeviceMotion(e wayland.DataDeviceMotionEvent) {}
func (w *window) HandleDataDeviceDrop(e wayland.DataDeviceDropEvent)     {}

// --- wl_data_offer events (mime types of the offered selection) ---

func (w *window) HandleDataOfferOffer(e wayland.DataOfferOfferEvent) {
	w.mu.Lock()
	w.pendingMimes = append(w.pendingMimes, e.MimeType)
	w.mu.Unlock()
}

func (w *window) HandleDataOfferSourceActions(e wayland.DataOfferSourceActionsEvent) {}
func (w *window) HandleDataOfferAction(e wayland.DataOfferActionEvent)               {}
