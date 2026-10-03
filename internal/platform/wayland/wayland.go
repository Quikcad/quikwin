//go:build linux

package wayland

import (
	"encoding/binary"
	"fmt"
	"math"
	"slices"
	"sync"
	"time"
	"unsafe"

	"github.com/Quikcad/quikwin/internal/platform/wake"
	"github.com/Quikcad/quikwin/internal/platform/wtypes"
	vk "github.com/lukem570/vulkan-go/pkg/raw"
	xdg "github.com/lukem570/wayland-go/pkg/protocols/stable/xdgshell"
	"github.com/lukem570/wayland-go/pkg/protocols/staging/cursorshapev1"
	xdgdeco "github.com/lukem570/wayland-go/pkg/protocols/unstable/xdgdecorationunstablev1"
	"github.com/lukem570/wayland-go/pkg/wayland"
	"github.com/lukem570/wayland-go/pkg/wl"
)

var (
	_ wtypes.Window                       = (*window)(nil)
	_ wayland.RegistryHandler             = (*window)(nil)
	_ wayland.SeatHandler                 = (*window)(nil)
	_ wayland.KeyboardHandler             = (*window)(nil)
	_ wayland.PointerHandler              = (*window)(nil)
	_ xdg.WmBaseHandler                   = (*window)(nil)
	_ xdg.SurfaceHandler                  = (*window)(nil)
	_ xdg.ToplevelHandler                 = (*window)(nil)
	_ xdgdeco.ToplevelDecorationV1Handler = (*window)(nil)
)

type window struct {
	mu sync.Mutex

	// wake releases a blocked dispatch when Post is called off the UI
	// goroutine.
	wake *wake.Pipe

	conn        *wl.Proxy
	display     *wayland.Display
	registry    *wayland.Registry
	compositor  *wayland.Compositor
	surface     *wayland.Surface
	wmBase      *xdg.WmBase
	xdgSurface  *xdg.Surface
	xdgToplevel *xdg.Toplevel
	seat        *wayland.Seat
	keyboard    *wayland.Keyboard
	pointer     *wayland.Pointer

	decorMgr      *xdgdeco.DecorationManagerV1
	toplevelDecor *xdgdeco.ToplevelDecorationV1
	cursorMgr     *cursorshapev1.ManagerV1
	cursorDev     *cursorshapev1.DeviceV1

	// Clipboard (wl_data_device). clipText/ownsClip describe the selection we
	// published; selOffer/selMimes the selection currently offered by another
	// client; pendingOffer/pendingMimes accumulate a data_offer's mime types
	// until the matching selection event names it. inputSerial is the latest
	// input event serial, required by set_selection.
	dataDevMgr   *wayland.DataDeviceManager
	dataDevice   *wayland.DataDevice
	inputSerial  uint32
	clipText     string
	ownsClip     bool
	clipSource   *wayland.DataSource
	selOffer     *wayland.DataOffer
	selMimes     []string
	pendingOffer *wayland.DataOffer
	pendingMimes []string

	// libxkbcommon state (opaque C pointers).
	xkbCtx    unsafe.Pointer
	xkbKeymap unsafe.Pointer
	xkbState  unsafe.Pointer

	// modState tracks held modifier keys directly from key press/release, so
	// modifiers are reported on key events even when the xkb modifier mask is
	// unavailable. OR'd with currentMods so either source suffices.
	modState wtypes.Mod

	globals map[string]globalEntry

	pendingSerial uint32
	configured    bool

	width, height uint32
	// The size bounds, in the surface coordinates the caller gave them in. What
	// xdg-shell is told is these less the frame inset; see applySizeLimits.
	//
	// maxWidth/maxHeight is the size a window that refuses to be resized is
	// pinned at, and zero where there is no maximum. It is the size the window
	// was *created* at and does not move: derived instead from the current size,
	// it follows the compositor down on a configure while the minimum stays put,
	// and the two cross — which is a protocol error that takes the connection
	// and every surface on it.
	minWidth, minHeight uint32
	maxWidth, maxHeight uint32
	resizePending       bool // a configure changed the size this poll cycle (guarded by mu)
	appID               string

	// inset is what the client declared through SetFrameInset, and geomW/geomH
	// the window geometry the compositor last configured.
	//
	// Both, because the surface is the geometry plus the inset and either can
	// move on its own. An inset that changes has to be re-applied to the size
	// the compositor asked for; adding the new one to a width that already
	// carries the old is how a window grows by a margin every time the shadow
	// is turned off and on again.
	//
	// geomConfigured tells "the compositor has not sized us yet" from "it sized
	// us at zero". Until the first configure carries a size, width and height
	// are what the caller asked for and there is no geometry to derive them
	// from.
	inset          wtypes.FrameInset
	geomW, geomH   uint32
	geomConfigured bool

	ptrX, ptrY   float64
	ptrSerial    uint32
	cursorSerial uint32

	resizable  bool
	decorated  bool
	dragging   bool
	maximized  bool
	tiled      bool
	fullscreen bool

	pendingMinimize       bool
	pendingToggleMaximize bool

	repeatRate  int32
	repeatDelay int32
	repeatStop  chan struct{}

	onResize      func(uint32, uint32)
	onLiveResize  func()
	onClose       func()
	onFocus       func(bool)
	onKey         func(wtypes.Key, wtypes.Action, wtypes.Mod)
	onChar        func(rune)
	onMouseButton func(wtypes.Button, wtypes.Action, wtypes.Mod)
	onMouseMove   func(float64, float64)
	onScroll      func(float64, float64, bool)
	onPinch       func(float64)
	onDragBegin   func(float64, float64)
	onDragMove    func(float64, float64)
	onDragEnd     func(float64, float64)
	onDrop        func([]string)
	onHitTest     func(float64, float64) wtypes.HitTestResult
	onCursorEnter func(float64, float64)

	cursorHidden bool
	// cursorShape is what the application asked for and shownCursor what the
	// compositor was last told. The two differ while the resize border is under
	// the pointer, which is the only thing that overrides the application — and
	// keeping both is what lets the override be taken back again. cursorSet
	// says whether shownCursor means anything yet: nothing has been sent before
	// the first pointer enter, and a hidden cursor is not a shape.
	cursorShape wtypes.CursorShape
	shownCursor wtypes.CursorShape
	cursorSet   bool

	shouldClose bool
	destroyed   bool
}

type globalEntry struct {
	name    uint32
	version uint32
}

func New(cfg *wtypes.Config) (*window, error) {
	if err := ensureLoaded(); err != nil {
		return nil, err
	}

	conn, err := wl.Connect("")
	if err != nil {
		return nil, fmt.Errorf("quikwin/wayland: %w (WAYLAND_DISPLAY not set?)", err)
	}

	wakePipe, err := wake.NewPipe()
	if err != nil {
		wl.Disconnect(conn)
		return nil, err
	}

	w := &window{
		wake:      wakePipe,
		conn:      conn,
		width:     cfg.Width,
		height:    cfg.Height,
		minWidth:  cfg.MinWidth,
		minHeight: cfg.MinHeight,
		resizable: cfg.Resizable,
		decorated: cfg.Border && cfg.Titlebar,
		globals:   make(map[string]globalEntry),
	}

	w.xkbCtx = xkbContextNew()
	if w.xkbCtx == nil {
		wl.Disconnect(conn)
		return nil, fmt.Errorf("quikwin/wayland: xkb_context_new failed")
	}

	w.display = wayland.NewDisplay(conn)
	w.registry = w.display.GetRegistry()
	w.registry.SetHandler(w)
	if err := wl.Roundtrip(conn); err != nil {
		w.Destroy()
		return nil, err
	}

	if err := w.bindGlobals(); err != nil {
		w.Destroy()
		return nil, err
	}

	w.surface = w.compositor.CreateSurface()
	w.xdgSurface = w.wmBase.GetXdgSurface(w.surface)
	w.xdgToplevel = w.xdgSurface.GetToplevel()

	w.wmBase.SetHandler(w)
	w.xdgSurface.SetHandler(w)
	w.xdgToplevel.SetHandler(w)

	if w.decorMgr != nil {
		w.toplevelDecor = w.decorMgr.GetToplevelDecoration(w.xdgToplevel)
		w.toplevelDecor.SetHandler(w)
		w.toplevelDecor.SetMode(xdgdeco.ToplevelDecorationV1ModeServerSide)
	}

	w.xdgToplevel.SetTitle(cfg.Title)
	// Set an app_id so the compositor (e.g. KWin) keeps a stable, restorable
	// taskbar entry — without one a minimized window can become unrecoverable.
	w.appID = cfg.Title
	w.xdgToplevel.SetAppID(cfg.Title)
	// Wayland has no direct resizable flag. To prevent resizing, pin max_size to
	// min_size so the compositor won't offer a resize handle. Both are kept
	// rather than sent, because what the compositor is told is these less the
	// frame inset and the inset is not declared yet; see applySizeLimits.
	if !cfg.Resizable {
		w.minWidth, w.minHeight = cfg.Width, cfg.Height
		w.maxWidth, w.maxHeight = cfg.Width, cfg.Height
	}
	if w.minWidth > 0 || w.minHeight > 0 || w.maxWidth > 0 {
		w.applySizeLimits()
	}

	// Request client-side decorations when either border or titlebar is
	// disabled — xdg-decoration is all-or-nothing, so dropping either means the
	// app draws all of its own chrome.
	if (!cfg.Border || !cfg.Titlebar) && w.toplevelDecor != nil {
		w.toplevelDecor.SetMode(xdgdeco.ToplevelDecorationV1ModeClientSide)
	}

	w.surface.Commit()
	if err := wl.Roundtrip(conn); err != nil {
		w.Destroy()
		return nil, err
	}

	return w, nil
}

func (w *window) bindGlobals() error {
	reg := w.registry.Proxy()

	comp, ok := w.globals[wayland.CompositorName]
	if !ok {
		return fmt.Errorf("quikwin/wayland: compositor does not advertise %s", wayland.CompositorName)
	}
	w.compositor = wayland.BindCompositor(reg, comp.name, comp.version)

	wm, ok := w.globals[xdg.WmBaseName]
	if !ok {
		return fmt.Errorf("quikwin/wayland: compositor does not advertise %s", xdg.WmBaseName)
	}
	w.wmBase = xdg.BindWmBase(reg, wm.name, wm.version)

	if e, ok := w.globals[xdgdeco.DecorationManagerV1Name]; ok {
		w.decorMgr = xdgdeco.BindDecorationManagerV1(reg, e.name, e.version)
	}
	if e, ok := w.globals[wayland.SeatName]; ok {
		w.seat = wayland.BindSeat(reg, e.name, e.version)
		w.seat.SetHandler(w)
	}
	if e, ok := w.globals[cursorshapev1.ManagerV1Name]; ok {
		w.cursorMgr = cursorshapev1.BindManagerV1(reg, e.name, e.version)
	}
	if e, ok := w.globals[wayland.DataDeviceManagerName]; ok {
		w.dataDevMgr = wayland.BindDataDeviceManager(reg, e.name, e.version)
	}
	if w.dataDevMgr != nil && w.seat != nil {
		w.dataDevice = w.dataDevMgr.GetDataDevice(w.seat)
		w.dataDevice.SetHandler(w)
	}
	return nil
}

// --- wl_registry events ---

func (w *window) HandleRegistryGlobal(e wayland.RegistryGlobalEvent) {
	w.mu.Lock()
	w.globals[e.Interface] = globalEntry{name: e.Name, version: e.Version}
	w.mu.Unlock()
}

func (w *window) HandleRegistryGlobalRemove(e wayland.RegistryGlobalRemoveEvent) {}

// --- wl_seat events ---

func (w *window) HandleSeatCapabilities(e wayland.SeatCapabilitiesEvent) {
	if e.Capabilities&wayland.SeatCapabilityKeyboard != 0 && w.keyboard == nil {
		w.keyboard = w.seat.GetKeyboard()
		w.keyboard.SetHandler(w)
	}
	if e.Capabilities&wayland.SeatCapabilityPointer != 0 && w.pointer == nil {
		w.pointer = w.seat.GetPointer()
		w.pointer.SetHandler(w)
		if w.cursorMgr != nil {
			w.cursorDev = w.cursorMgr.GetPointer(w.pointer)
		}
	}
}

func (w *window) HandleSeatName(e wayland.SeatNameEvent) {}

// --- xdg_wm_base events ---

func (w *window) HandleWmBasePing(e xdg.WmBasePingEvent) {
	w.wmBase.Pong(e.Serial)
	wl.Flush(w.conn)
}

// --- xdg_surface events ---

func (w *window) HandleSurfaceConfigure(e xdg.SurfaceConfigureEvent) {
	w.mu.Lock()
	w.pendingSerial = e.Serial
	w.mu.Unlock()
	w.xdgSurface.AckConfigure(e.Serial)
	if !w.configured {
		w.configured = true
		w.surface.Commit()
	}
}

// --- xdg_toplevel events ---

// HandleToplevelConfigure takes the compositor's new size and states.
//
// The size is in window geometry, which is the surface less the frame inset —
// so it is kept as the geometry it is and the surface derived from it. The
// states come first because they decide whether the inset applies at all: a
// window that has just been maximized has no margin, and the size in this very
// event is the one it has to be read with.
func (w *window) HandleToplevelConfigure(e xdg.ToplevelConfigureEvent) {
	maximized := statesContain(e.States, xdg.ToplevelStateMaximized)
	fullscreen := statesContain(e.States, xdg.ToplevelStateFullscreen)
	tiled := statesContainAny(e.States,
		xdg.ToplevelStateTiledLeft,
		xdg.ToplevelStateTiledRight,
		xdg.ToplevelStateTiledTop,
		xdg.ToplevelStateTiledBottom,
	)

	w.mu.Lock()
	framed := w.effectiveInset()
	w.maximized = maximized
	w.tiled = tiled
	w.fullscreen = fullscreen
	if e.Width > 0 && e.Height > 0 {
		w.geomW, w.geomH = uint32(e.Width), uint32(e.Height)
		w.geomConfigured = true
	}
	sizeChanged := w.syncSurfaceSizeLocked()
	insetChanged := w.effectiveInset() != framed
	w.mu.Unlock()

	if !sizeChanged && !insetChanged {
		return
	}
	w.applyWindowGeometry()
	// The maximum that pins a non-resizable window is the current size, and the
	// minimum is in window geometry — so what both have to be sent as moves with
	// the size and with the inset, and the inset stops applying the moment the
	// window is maximized with nothing else to notice.
	w.applySizeLimits()
	wl.Flush(w.conn)
}

func (w *window) HandleToplevelClose(e xdg.ToplevelCloseEvent) {
	w.mu.Lock()
	w.shouldClose = true
	w.mu.Unlock()
	if fn := w.onClose; fn != nil {
		fn()
	}
}

func (w *window) HandleToplevelConfigureBounds(e xdg.ToplevelConfigureBoundsEvent) {}
func (w *window) HandleToplevelWmCapabilities(e xdg.ToplevelWmCapabilitiesEvent)   {}

// --- zxdg_toplevel_decoration_v1 events ---

func (w *window) HandleToplevelDecorationV1Configure(e xdgdeco.ToplevelDecorationV1ConfigureEvent) {}

// --- wl_keyboard events ---

func (w *window) HandleKeyboardKeymap(e wayland.KeyboardKeymapEvent) {
	if e.Format != wayland.KeyboardKeymapFormatXkbV1 {
		return
	}
	b := readFD(int(e.FD), int(e.Size))
	if b == nil {
		return
	}
	km := xkbKeymapNewFromString(w.xkbCtx, b)
	if km == nil {
		return
	}
	st := xkbStateNew(km)
	w.mu.Lock()
	if w.xkbKeymap != nil {
		xkbKeymapUnref(w.xkbKeymap)
	}
	if w.xkbState != nil {
		xkbStateUnref(w.xkbState)
	}
	w.xkbKeymap = km
	w.xkbState = st
	w.mu.Unlock()
}

func (w *window) HandleKeyboardEnter(e wayland.KeyboardEnterEvent) {
	w.mu.Lock()
	w.inputSerial = e.Serial
	w.mu.Unlock()
	if fn := w.onFocus; fn != nil {
		fn(true)
	}
}

func (w *window) HandleKeyboardLeave(e wayland.KeyboardLeaveEvent) {
	w.stopRepeat()
	w.mu.Lock()
	w.modState = 0
	w.mu.Unlock()
	if fn := w.onFocus; fn != nil {
		fn(false)
	}
}

// modBit maps a modifier key to its Mod bit, reporting ok=false for non-modifier
// keys. Used to track held modifiers directly from key events.
func modBit(k wtypes.Key) (wtypes.Mod, bool) {
	switch k {
	case wtypes.KeyLeftShift, wtypes.KeyRightShift:
		return wtypes.ModShift, true
	case wtypes.KeyLeftControl, wtypes.KeyRightControl:
		return wtypes.ModControl, true
	case wtypes.KeyLeftAlt, wtypes.KeyRightAlt:
		return wtypes.ModAlt, true
	case wtypes.KeyLeftSuper, wtypes.KeyRightSuper:
		return wtypes.ModSuper, true
	}
	return 0, false
}

func (w *window) HandleKeyboardKey(e wayland.KeyboardKeyEvent) {
	w.stopRepeat()

	w.mu.Lock()
	w.inputSerial = e.Serial
	w.mu.Unlock()

	keycode := e.Key + 8
	w.mu.Lock()
	st := w.xkbState
	w.mu.Unlock()
	if st == nil {
		return
	}
	k := keysymToKey(uint64(xkbStateKeyGetOneSym(st, keycode)))
	action := wtypes.Press
	if e.State == wayland.KeyboardKeyStateReleased {
		action = wtypes.Release
	}
	if bit, ok := modBit(k); ok {
		w.mu.Lock()
		if action == wtypes.Release {
			w.modState &^= bit
		} else {
			w.modState |= bit
		}
		w.mu.Unlock()
	}
	w.mu.Lock()
	tracked := w.modState
	w.mu.Unlock()
	mods := w.currentMods() | tracked
	if fn := w.onKey; fn != nil {
		fn(k, action, mods)
	}
	if e.State == wayland.KeyboardKeyStatePressed {
		cp := xkbStateKeyGetUtf32(st, keycode)
		if cp >= 32 && cp != 127 {
			if fn := w.onChar; fn != nil {
				fn(rune(cp))
			}
		}
		w.startRepeat(k, rune(cp), mods)
	}
}

func (w *window) HandleKeyboardModifiers(e wayland.KeyboardModifiersEvent) {
	w.mu.Lock()
	st := w.xkbState
	w.mu.Unlock()
	if st == nil {
		return
	}
	xkbStateUpdateMask(st, e.ModsDepressed, e.ModsLatched, e.ModsLocked, e.Group)
}

func (w *window) HandleKeyboardRepeatInfo(e wayland.KeyboardRepeatInfoEvent) {
	w.mu.Lock()
	w.repeatRate = e.Rate
	w.repeatDelay = e.Delay
	w.mu.Unlock()
}

// stopRepeat cancels any running key-repeat goroutine.
func (w *window) stopRepeat() {
	if w.repeatStop != nil {
		close(w.repeatStop)
		w.repeatStop = nil
	}
}

// startRepeat begins key repeat for the given key and character.
func (w *window) startRepeat(k wtypes.Key, ch rune, mods wtypes.Mod) {
	w.mu.Lock()
	rate := w.repeatRate
	delay := w.repeatDelay
	w.mu.Unlock()
	if rate <= 0 {
		return
	}

	stop := make(chan struct{})
	w.repeatStop = stop

	interval := time.Second / time.Duration(rate)

	go func() {
		timer := time.NewTimer(time.Duration(delay) * time.Millisecond)
		defer timer.Stop()

		select {
		case <-stop:
			return
		case <-timer.C:
		}

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			if fn := w.onKey; fn != nil {
				fn(k, wtypes.Repeat, mods)
			}
			if ch >= 32 && ch != 127 {
				if fn := w.onChar; fn != nil {
					fn(ch)
				}
			}

			select {
			case <-stop:
				return
			case <-ticker.C:
			}
		}
	}()
}

// --- wl_pointer events ---

func (w *window) HandlePointerEnter(e wayland.PointerEnterEvent) {
	x := wlFixedToFloat(e.SurfaceX)
	y := wlFixedToFloat(e.SurfaceY)
	w.mu.Lock()
	w.ptrSerial = e.Serial
	w.cursorSerial = e.Serial
	w.ptrX, w.ptrY = x, y
	w.mu.Unlock()

	if fn := w.onCursorEnter; fn != nil {
		fn(x, y)
	}
	w.refreshCursor(x, y, e.Serial)
}

// HandlePointerLeave forgets what the compositor was told.
//
// The cursor belongs to whoever the pointer is over, so once it has left this
// surface the shape is somebody else's and what we last set is not what is
// showing. Remembering it would make the next enter a no-op — the pointer would
// come back carrying the last client's cursor.
func (w *window) HandlePointerLeave(e wayland.PointerLeaveEvent) {
	w.mu.Lock()
	w.cursorSet = false
	w.mu.Unlock()
}

func (w *window) HandlePointerMotion(e wayland.PointerMotionEvent) {
	x := wlFixedToFloat(e.SurfaceX)
	y := wlFixedToFloat(e.SurfaceY)
	w.mu.Lock()
	w.ptrX, w.ptrY = x, y
	serial := w.cursorSerial
	w.mu.Unlock()
	if w.dragging {
		if fn := w.onDragMove; fn != nil {
			fn(x, y)
		}
		return
	}
	if fn := w.onMouseMove; fn != nil {
		fn(x, y)
	}
	w.refreshCursor(x, y, serial)
}

// refreshCursor keeps the pointer's shape in step with where the pointer is:
// the matching resize cursor over an undecorated window's invisible resize
// border, and the application's own everywhere else.
//
// **R.** The resize cursor stuck. The version this replaces set a shape when the
// pointer was on the border and did nothing at all when it was not, so nothing
// ever took the resize arrow back. And because it wrote past the application's
// choice without recording it, the application could not take it back either:
// quikgui calls SetCursor only when *its* answer changes, and its answer had not
// changed — so the arrow stayed until the pointer happened to cross a widget
// that wanted a different one.
//
// So the border is an override with a withdrawal, and one place decides what is
// showing. Called from every pointer enter and motion, and it sends only on a
// change, which is the per-motion call the old early return was there to avoid.
func (w *window) refreshCursor(x, y float64, serial uint32) {
	w.mu.Lock()
	shape := w.cursorForLocked(x, y)
	send := w.claimCursorLocked(shape)
	w.mu.Unlock()
	if !send {
		return
	}
	w.cursorDev.SetShape(serial, cursorShapeMap(shape))
	wl.Flush(w.conn)
}

// cursorForLocked is the shape the pointer should have at (x, y): the resize
// border's where there is one, and the application's everywhere else. Call with
// mu held.
//
// The border wins over the application, because the application is answering
// about the widget under the pointer and over the border the pointer is not
// there to use the widget.
func (w *window) cursorForLocked(x, y float64) wtypes.CursorShape {
	if !w.decorated && w.resizable {
		if edge := w.detectEdgeLocked(x, y); edge != wtypes.EdgeNone {
			return wtypes.EdgeCursorShape(edge)
		}
	}
	return w.cursorShape
}

// claimCursorLocked records that the compositor is about to be told shape, and
// reports whether it needs telling. Call with mu held.
func (w *window) claimCursorLocked(shape wtypes.CursorShape) bool {
	if w.cursorDev == nil || w.cursorHidden {
		return false
	}
	if w.cursorSet && w.shownCursor == shape {
		return false
	}
	w.shownCursor, w.cursorSet = shape, true
	return true
}

func (w *window) HandlePointerButton(e wayland.PointerButtonEvent) {
	w.mu.Lock()
	w.ptrSerial = e.Serial
	w.inputSerial = e.Serial
	w.mu.Unlock()
	b, ok := evdevButton(e.Button)
	if !ok {
		return
	}
	action := wtypes.Press
	if e.State == wayland.PointerButtonStateReleased {
		action = wtypes.Release
	}
	mods := w.currentMods()
	if action == wtypes.Release && w.dragging {
		w.mu.Lock()
		w.dragging = false
		w.mu.Unlock()
		if fn := w.onDragEnd; fn != nil {
			fn(w.ptrX, w.ptrY)
		}
		return
	}
	if action == wtypes.Press && b == wtypes.ButtonLeft {
		if !w.decorated && w.resizable {
			edge := w.detectEdge(w.ptrX, w.ptrY)
			if edge != wtypes.EdgeNone {
				w.xdgToplevel.Resize(w.seat, e.Serial, xdg.ToplevelResizeEdge(edge))
				return
			}
		}
		if fn := w.onHitTest; fn != nil {
			if fn(w.ptrX, w.ptrY) == wtypes.HitTestDrag {
				w.xdgToplevel.Move(w.seat, e.Serial)
				return
			}
		}
	}
	if fn := w.onMouseButton; fn != nil {
		fn(b, action, mods)
	}
}

func (w *window) HandlePointerAxis(e wayland.PointerAxisEvent) {
	v := wlFixedToFloat(e.Value)
	if fn := w.onScroll; fn != nil {
		if e.Axis == wayland.PointerAxisVerticalScroll {
			fn(0, -v/10, false)
		} else {
			fn(v/10, 0, false)
		}
	}
}

func (w *window) HandlePointerFrame(e wayland.PointerFrameEvent)                                 {}
func (w *window) HandlePointerAxisSource(e wayland.PointerAxisSourceEvent)                       {}
func (w *window) HandlePointerAxisStop(e wayland.PointerAxisStopEvent)                           {}
func (w *window) HandlePointerAxisDiscrete(e wayland.PointerAxisDiscreteEvent)                   {}
func (w *window) HandlePointerAxisValue120(e wayland.PointerAxisValue120Event)                   {}
func (w *window) HandlePointerAxisRelativeDirection(e wayland.PointerAxisRelativeDirectionEvent) {}

// --- window.Window interface ---

func (w *window) Size() (uint32, uint32) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.width, w.height
}

func (w *window) IsMaximized() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.maximized
}

func (w *window) IsTiled() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.tiled
}

func (w *window) IsFullscreen() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.fullscreen
}

// --- frame inset: the margin a client paints its shadow in ---

func (w *window) SetFrameInset(inset wtypes.FrameInset) {
	inset = inset.Normalized()

	w.mu.Lock()
	if inset == w.inset {
		w.mu.Unlock()
		return
	}
	w.inset = inset
	w.syncSurfaceSizeLocked()
	w.mu.Unlock()

	w.applyWindowGeometry()
	w.applySizeLimits()
	wl.Flush(w.conn)
}

// effectiveInset is the declared inset where it applies and nothing where it
// does not. Call with mu held.
//
// A maximized, tiled or fullscreen window sits flush against a screen edge. The
// margin would be painted into space the compositor has given to something
// else, so a client does not paint it there and the geometry must not claim it.
func (w *window) effectiveInset() wtypes.FrameInset {
	if w.maximized || w.tiled || w.fullscreen {
		return wtypes.FrameInset{}
	}
	return w.inset
}

// insetPixels is the inset rounded to whole pixels, which is what xdg-shell's
// geometry is measured in.
func insetPixels(in wtypes.FrameInset) (l, t, r, b int32) {
	return int32(math.Round(in.Left)), int32(math.Round(in.Top)),
		int32(math.Round(in.Right)), int32(math.Round(in.Bottom))
}

// syncSurfaceSizeLocked recomputes the surface from the window geometry the
// compositor configured and the margin painted around it, and reports whether
// it moved. Call with mu held.
//
// Before the first configure that carried a size there is no geometry to derive
// anything from, and width and height are the size the caller asked for.
func (w *window) syncSurfaceSizeLocked() bool {
	if !w.geomConfigured {
		return false
	}
	l, t, r, b := insetPixels(w.effectiveInset())
	nw := uint32(int32(w.geomW) + l + r)
	nh := uint32(int32(w.geomH) + t + b)
	if nw == w.width && nh == w.height {
		return false
	}
	w.width, w.height = nw, nh
	w.resizePending = true
	return true
}

// applyWindowGeometry tells the compositor which part of the surface is the
// window. Everything outside it is margin, and the compositor leaves it out of
// snapping, tiling, maximizing and its own idea of where the window's edges are.
//
// A geometry that does not intersect the surface is a protocol error, so a
// margin wider than the window it surrounds is dropped rather than sent.
func (w *window) applyWindowGeometry() {
	if w.xdgSurface == nil {
		return
	}
	w.mu.Lock()
	l, t, r, b := insetPixels(w.effectiveInset())
	gw := int32(w.width) - l - r
	gh := int32(w.height) - t - b
	w.mu.Unlock()

	if gw < 1 || gh < 1 {
		return
	}
	w.xdgSurface.SetWindowGeometry(l, t, gw, gh)
}

// applySizeLimits sends the size bounds in the coordinates xdg-shell measures
// them in, which is window geometry rather than the surface the caller paints.
//
// The two differ by the margin. Sending the surface minimum unadjusted reserves
// the margin a second time, and the window stops shrinking a shadow's width
// before it has to.
//
// A minimum above the maximum is a protocol error that takes the connection
// with it, so the pair is reconciled here rather than forwarded. The caller is
// describing a window that cannot exist; the compositor's answer to that is to
// disconnect the process, which is not an answer a caller can act on.
func (w *window) applySizeLimits() {
	if w.xdgToplevel == nil {
		return
	}
	w.mu.Lock()
	l, t, r, b := insetPixels(w.effectiveInset())
	minW, minH := max(int32(w.minWidth)-l-r, 0), max(int32(w.minHeight)-t-b, 0)
	maxW, maxH := max(int32(w.maxWidth)-l-r, 0), max(int32(w.maxHeight)-t-b, 0)
	hasMax := w.maxWidth > 0 || w.maxHeight > 0
	w.mu.Unlock()

	if hasMax {
		minW, minH = min(minW, maxW), min(minH, maxH)
	}
	w.xdgToplevel.SetMinSize(minW, minH)
	// Wayland has no resizable flag; a maximum pinned to the size the window was
	// made at is how the compositor is told not to offer a handle. See New.
	if hasMax {
		w.xdgToplevel.SetMaxSize(maxW, maxH)
	}
}

// detectEdgeLocked is [wtypes.DetectEdge] against the window rather than the
// surface. Call with mu held.
//
// The resize border belongs on the window's edge. Measured against the surface
// it sits out where a client-drawn shadow fades away, with a dead strip the
// pointer crosses on the way in — which is the whole of what the margin looks
// like from outside. A point in the margin maps to a negative coordinate and so
// still reads as the edge nearest it: the shadow is grabbable, which is what a
// user aiming at the corner of a window expects.
func (w *window) detectEdgeLocked(x, y float64) wtypes.ResizeEdge {
	in := w.effectiveInset()
	return wtypes.DetectEdge(
		x-in.Left, y-in.Top,
		float64(w.width)-in.Horizontal(), float64(w.height)-in.Vertical(),
		wtypes.BorderWidth,
	)
}

func (w *window) detectEdge(x, y float64) wtypes.ResizeEdge {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.detectEdgeLocked(x, y)
}

// ClientDecorated reports whether the window draws its own decorations, which
// is the case whenever border or titlebar was disabled at creation.
func (w *window) ClientDecorated() bool { return !w.decorated }

func (w *window) Scale() float32 { return 1.0 }

func (w *window) ShouldClose() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.shouldClose
}

func (w *window) PollEvents() { w.dispatch(0) }

func (w *window) WaitEvents() { w.dispatch(-1) }

func (w *window) WaitEventsTimeout(d time.Duration) {
	if d <= 0 {
		w.dispatch(0)
		return
	}
	w.dispatch(d)
}

func (w *window) Post() { w.wake.Signal() }

// dispatch drains the compositor queue. A negative timeout blocks until an
// event or a Post arrives, zero returns at once, and a positive one blocks for
// at most that long.
//
// The prepare_read/poll/read_events sequence is the wl_display protocol for
// waiting on the connection: the read intent is registered before the poll so
// that an event arriving in the window between the two still wakes it.
func (w *window) dispatch(timeout time.Duration) {
	w.mu.Lock()
	destroyed := w.destroyed
	w.mu.Unlock()
	if destroyed {
		return
	}
	d := w.conn.Pointer()
	// Anything already queued is work to hand back now, so it cancels the
	// wait: blocking after dispatching it would hold the caller's frame for
	// the whole timeout.
	if displayDispatchPending(d) > 0 {
		timeout = 0
	}
	for displayPrepareRead(d) != 0 {
		if displayDispatchPending(d) > 0 {
			timeout = 0
		}
	}
	// Issue deferred window-state requests now, outside any dispatch callback.
	w.applyPending()
	wl.Flush(w.conn)
	display, posted := wake.WaitEither(displayGetFd(d), w.wake.ReadFD(), timeout)
	if posted {
		w.wake.Drain()
	}
	if display {
		displayReadEvents(d)
	} else {
		displayCancelRead(d)
	}
	displayDispatchPending(d)
	if w.applyPending() {
		wl.Flush(w.conn)
	}
	w.flushResize()
}

// flushResize fires a single onResize for the final size seen this poll cycle.
// A compositor-driven resize delivers a burst of xdg_toplevel configure events;
// collapsing them to one callback avoids recreating the swapchain (and
// re-rendering) once per event. onLiveResize is intentionally not driven here —
// unlike macOS, the Wayland event loop does not block during a resize, so
// App.Tick renders the new size itself right after PollEvents returns.
func (w *window) flushResize() {
	w.mu.Lock()
	pending := w.resizePending
	w.resizePending = false
	nw, nh := w.width, w.height
	w.mu.Unlock()
	if !pending {
		return
	}
	if fn := w.onResize; fn != nil {
		fn(nw, nh)
	}
}

func (w *window) SetTitle(title string) {
	w.xdgToplevel.SetTitle(title)
	wl.Flush(w.conn)
}

// Minimize and ToggleMaximize only record intent; the xdg_toplevel request is
// issued from PollEvents (applyPending). They are typically called from inside
// an event-dispatch callback, so deferring keeps window-state changes on the
// main loop and ordered after the latest compositor configure.
func (w *window) Minimize() {
	w.pendingMinimize = true
}

func (w *window) ToggleMaximize() {
	w.pendingToggleMaximize = true
}

// applyPending issues any deferred window-state requests. It must run on the
// main loop, never from within an event-dispatch callback. Returns true if it
// marshalled anything (so the caller can flush).
func (w *window) applyPending() bool {
	if w.xdgToplevel == nil {
		w.pendingMinimize = false
		w.pendingToggleMaximize = false
		return false
	}
	did := false
	if w.pendingMinimize {
		w.pendingMinimize = false
		w.xdgToplevel.SetMinimized()
		did = true
	}
	if w.pendingToggleMaximize {
		w.pendingToggleMaximize = false
		if w.maximized {
			w.xdgToplevel.UnsetMaximized()
		} else {
			w.xdgToplevel.SetMaximized()
		}
		w.maximized = !w.maximized
		did = true
	}
	return did
}

// SetCursor records what the application wants and shows it, unless the pointer
// is on the resize border — see cursorForLocked.
func (w *window) SetCursor(shape wtypes.CursorShape) {
	w.mu.Lock()
	w.cursorShape = shape
	eff := w.cursorForLocked(w.ptrX, w.ptrY)
	send := w.claimCursorLocked(eff)
	serial := w.cursorSerial
	w.mu.Unlock()
	if !send {
		return
	}
	w.cursorDev.SetShape(serial, cursorShapeMap(eff))
	wl.Flush(w.conn)
}

func (w *window) HideCursor() {
	w.mu.Lock()
	w.cursorHidden = true
	// A hidden pointer is not a shape the compositor is holding, so there is
	// nothing for the next claim to match against.
	w.cursorSet = false
	serial := w.cursorSerial
	w.mu.Unlock()
	if w.pointer == nil {
		return
	}
	w.pointer.SetCursor(serial, nil, 0, 0)
	wl.Flush(w.conn)
}

func (w *window) ShowCursor() {
	w.mu.Lock()
	w.cursorHidden = false
	shape := w.cursorForLocked(w.ptrX, w.ptrY)
	send := w.claimCursorLocked(shape)
	serial := w.cursorSerial
	w.mu.Unlock()
	if !send {
		return
	}
	w.cursorDev.SetShape(serial, cursorShapeMap(shape))
	wl.Flush(w.conn)
}

// SetMinSize takes the smallest surface the caller can paint. What reaches the
// compositor is that less the frame inset; see applySizeLimits.
func (w *window) SetMinSize(mw, mh uint32) {
	w.mu.Lock()
	w.minWidth = mw
	w.minHeight = mh
	w.mu.Unlock()
	w.applySizeLimits()
	w.surface.Commit()
	wl.Flush(w.conn)
}

func (w *window) SetSize(sw, sh uint32) {
	w.mu.Lock()
	w.width = sw
	w.height = sh
	w.mu.Unlock()
	if fn := w.onResize; fn != nil {
		fn(sw, sh)
	}
}

func (w *window) Destroy() {
	w.stopRepeat()
	w.mu.Lock()
	if w.destroyed {
		w.mu.Unlock()
		return
	}
	w.destroyed = true
	w.mu.Unlock()

	w.wake.Close()

	destroy := func(p *wl.Proxy) {
		if p != nil {
			p.Destroy()
		}
	}
	if w.clipSource != nil {
		destroy(w.clipSource.Proxy())
	}
	if w.dataDevice != nil {
		destroy(w.dataDevice.Proxy())
	}
	if w.dataDevMgr != nil {
		destroy(w.dataDevMgr.Proxy())
	}
	if w.cursorDev != nil {
		destroy(w.cursorDev.Proxy())
	}
	if w.cursorMgr != nil {
		destroy(w.cursorMgr.Proxy())
	}
	if w.pointer != nil {
		destroy(w.pointer.Proxy())
	}
	if w.keyboard != nil {
		destroy(w.keyboard.Proxy())
	}
	if w.seat != nil {
		destroy(w.seat.Proxy())
	}
	if w.toplevelDecor != nil {
		destroy(w.toplevelDecor.Proxy())
	}
	if w.decorMgr != nil {
		destroy(w.decorMgr.Proxy())
	}
	if w.xdgToplevel != nil {
		destroy(w.xdgToplevel.Proxy())
	}
	if w.xdgSurface != nil {
		destroy(w.xdgSurface.Proxy())
	}
	if w.surface != nil {
		destroy(w.surface.Proxy())
	}
	if w.wmBase != nil {
		destroy(w.wmBase.Proxy())
	}
	if w.compositor != nil {
		destroy(w.compositor.Proxy())
	}
	if w.registry != nil {
		destroy(w.registry.Proxy())
	}

	if w.xkbState != nil {
		xkbStateUnref(w.xkbState)
		w.xkbState = nil
	}
	if w.xkbKeymap != nil {
		xkbKeymapUnref(w.xkbKeymap)
		w.xkbKeymap = nil
	}
	if w.xkbCtx != nil {
		xkbContextUnref(w.xkbCtx)
		w.xkbCtx = nil
	}
	wl.Disconnect(w.conn)
}

func (w *window) BeginDrag() {
	w.mu.Lock()
	serial := w.ptrSerial
	w.mu.Unlock()
	w.xdgToplevel.Move(w.seat, serial)
}

// --- Event registration ---

func (w *window) OnResize(fn func(uint32, uint32))                     { w.onResize = fn }
func (w *window) OnLiveResize(fn func())                               { w.onLiveResize = fn }
func (w *window) OnClose(fn func())                                    { w.onClose = fn }
func (w *window) OnFocus(fn func(bool))                                { w.onFocus = fn }
func (w *window) OnKey(fn func(wtypes.Key, wtypes.Action, wtypes.Mod)) { w.onKey = fn }
func (w *window) OnChar(fn func(rune))                                 { w.onChar = fn }
func (w *window) OnMouseButton(fn func(wtypes.Button, wtypes.Action, wtypes.Mod)) {
	w.onMouseButton = fn
}
func (w *window) OnMouseMove(fn func(float64, float64))                    { w.onMouseMove = fn }
func (w *window) OnScroll(fn func(float64, float64, bool))                 { w.onScroll = fn }
func (w *window) OnPinch(fn func(float64))                                 { w.onPinch = fn }
func (w *window) OnDragBegin(fn func(float64, float64))                    { w.onDragBegin = fn }
func (w *window) OnDragMove(fn func(float64, float64))                     { w.onDragMove = fn }
func (w *window) OnDragEnd(fn func(float64, float64))                      { w.onDragEnd = fn }
func (w *window) OnDrop(fn func([]string))                                 { w.onDrop = fn }
func (w *window) OnHitTest(fn func(float64, float64) wtypes.HitTestResult) { w.onHitTest = fn }
func (w *window) OnCursorEnter(fn func(float64, float64))                  { w.onCursorEnter = fn }

// --- WaylandWindow interface ---

func (w *window) WlDisplay() uintptr { return uintptr(w.conn.Pointer()) }
func (w *window) WlSurface() uintptr { return uintptr(w.surface.Proxy().Pointer()) }

func (w *window) SetAppID(id string) {
	w.mu.Lock()
	w.appID = id
	w.mu.Unlock()
	w.xdgToplevel.SetAppID(id)
	wl.Flush(w.conn)
}

// --- vkwin.Window interface ---

func (w *window) NewSurface(instance vk.Instance) (*vk.SurfaceKHR, error) {
	return instance.CreateWaylandSurfaceKHR(&vk.WaylandSurfaceCreateInfoKHR{
		Display: w.conn.Pointer(),
		Surface: w.surface.Proxy().Pointer(),
	}, nil)
}

// --- Helpers ---

// statesContain reports whether the packed little-endian uint32 array carries
// the given xdg_toplevel state.
func statesContain(states []byte, want xdg.ToplevelState) bool {
	for i := 0; i+4 <= len(states); i += 4 {
		if xdg.ToplevelState(binary.LittleEndian.Uint32(states[i:])) == want {
			return true
		}
	}
	return false
}

// statesContainAny reports whether the packed array carries any of the given
// xdg_toplevel states. Used to collapse the four tiled_* edges into a single
// "tiled" predicate (KWin quick-tile sets a left/right edge plus top+bottom).
func statesContainAny(states []byte, want ...xdg.ToplevelState) bool {
	for i := 0; i+4 <= len(states); i += 4 {
		got := xdg.ToplevelState(binary.LittleEndian.Uint32(states[i:]))
		if slices.Contains(want, got) {
			return true
		}
	}
	return false
}

func wlFixedToFloat(v int32) float64 { return float64(v) / 256.0 }

func evdevButton(code uint32) (wtypes.Button, bool) {
	switch code {
	case 0x110:
		return wtypes.ButtonLeft, true
	case 0x111:
		return wtypes.ButtonRight, true
	case 0x112:
		return wtypes.ButtonMiddle, true
	case 0x113:
		return wtypes.Button4, true
	case 0x114:
		return wtypes.Button5, true
	}
	return 0, false
}

func cursorShapeMap(shape wtypes.CursorShape) cursorshapev1.DeviceV1Shape {
	switch shape {
	case wtypes.CursorArrow:
		return cursorshapev1.DeviceV1ShapeDefault
	case wtypes.CursorIBeam:
		return cursorshapev1.DeviceV1ShapeText
	case wtypes.CursorCrosshair:
		return cursorshapev1.DeviceV1ShapeCrosshair
	case wtypes.CursorHand:
		return cursorshapev1.DeviceV1ShapePointer
	case wtypes.CursorHResize:
		return cursorshapev1.DeviceV1ShapeEwResize
	case wtypes.CursorVResize:
		return cursorshapev1.DeviceV1ShapeNsResize
	case wtypes.CursorNWSEResize:
		return cursorshapev1.DeviceV1ShapeNwseResize
	case wtypes.CursorNESWResize:
		return cursorshapev1.DeviceV1ShapeNeswResize
	case wtypes.CursorAllResize:
		return cursorshapev1.DeviceV1ShapeAllScroll
	case wtypes.CursorNotAllowed:
		return cursorshapev1.DeviceV1ShapeNotAllowed
	default:
		return cursorshapev1.DeviceV1ShapeDefault
	}
}

func (w *window) currentMods() wtypes.Mod {
	w.mu.Lock()
	state := w.xkbState
	keymap := w.xkbKeymap
	w.mu.Unlock()
	if state == nil || keymap == nil {
		return 0
	}
	active := func(name string) bool {
		return xkbStateModIndexIsActive(state, xkbKeymapModGetIndex(keymap, name))
	}
	var m wtypes.Mod
	if active("Shift") {
		m |= wtypes.ModShift
	}
	if active("Control") {
		m |= wtypes.ModControl
	}
	if active("Mod1") {
		m |= wtypes.ModAlt
	}
	if active("Mod4") {
		m |= wtypes.ModSuper
	}
	if active("Lock") {
		m |= wtypes.ModCapsLock
	}
	if active("Mod2") {
		m |= wtypes.ModNumLock
	}
	return m
}

// readFD reads exactly size bytes from the keymap fd. It reads from absolute
// offset 0 with pread: the fd arrives via SCM_RIGHTS and shares the
// compositor's open file description, whose offset is already at end-of-file,
// so a plain read would return EOF immediately and busy-loop here forever.
func readFD(fd, size int) []byte {
	if size <= 0 {
		return nil
	}
	b := make([]byte, size)
	off := 0
	for off < size {
		nn, err := sysPread(fd, b[off:], int64(off))
		if nn > 0 {
			off += nn
		}
		if err != nil || nn == 0 {
			break
		}
	}
	sysClose(fd)
	if off < size {
		return nil
	}
	return b
}

func keysymToKey(sym uint64) wtypes.Key {
	switch {
	case sym == 0x0020:
		return wtypes.KeySpace
	case sym == 0x0027:
		return wtypes.KeyApostrophe
	case sym == 0x002c:
		return wtypes.KeyComma
	case sym == 0x002d:
		return wtypes.KeyMinus
	case sym == 0x002e:
		return wtypes.KeyPeriod
	case sym == 0x002f:
		return wtypes.KeySlash
	case sym >= 0x0030 && sym <= 0x0039:
		return wtypes.Key(wtypes.Key0 + wtypes.Key(sym-0x0030))
	case sym == 0x003b:
		return wtypes.KeySemicolon
	case sym == 0x003d:
		return wtypes.KeyEqual
	case sym >= 0x0061 && sym <= 0x007a:
		return wtypes.Key(wtypes.KeyA + wtypes.Key(sym-0x0061))
	case sym >= 0x0041 && sym <= 0x005a:
		return wtypes.Key(wtypes.KeyA + wtypes.Key(sym-0x0041))
	case sym == 0x005b:
		return wtypes.KeyLeftBracket
	case sym == 0x005c:
		return wtypes.KeyBackslash
	case sym == 0x005d:
		return wtypes.KeyRightBracket
	case sym == 0x0060:
		return wtypes.KeyGraveAccent
	case sym == 0xff1b:
		return wtypes.KeyEscape
	case sym == 0xff0d:
		return wtypes.KeyEnter
	case sym == 0xff09:
		return wtypes.KeyTab
	case sym == 0xff08:
		return wtypes.KeyBackspace
	case sym == 0xff63:
		return wtypes.KeyInsert
	case sym == 0xffff:
		return wtypes.KeyDelete
	case sym == 0xff53:
		return wtypes.KeyRight
	case sym == 0xff51:
		return wtypes.KeyLeft
	case sym == 0xff54:
		return wtypes.KeyDown
	case sym == 0xff52:
		return wtypes.KeyUp
	case sym == 0xff55:
		return wtypes.KeyPageUp
	case sym == 0xff56:
		return wtypes.KeyPageDown
	case sym == 0xff50:
		return wtypes.KeyHome
	case sym == 0xff57:
		return wtypes.KeyEnd
	case sym == 0xffe5:
		return wtypes.KeyCapsLock
	case sym == 0xff14:
		return wtypes.KeyScrollLock
	case sym == 0xff7f:
		return wtypes.KeyNumLock
	case sym == 0xff61:
		return wtypes.KeyPrintScreen
	case sym == 0xff13:
		return wtypes.KeyPause
	case sym >= 0xffbe && sym <= 0xffc9:
		return wtypes.Key(wtypes.KeyF1 + wtypes.Key(sym-0xffbe))
	case sym == 0xffe1:
		return wtypes.KeyLeftShift
	case sym == 0xffe2:
		return wtypes.KeyRightShift
	case sym == 0xffe3:
		return wtypes.KeyLeftControl
	case sym == 0xffe4:
		return wtypes.KeyRightControl
	case sym == 0xffe9:
		return wtypes.KeyLeftAlt
	case sym == 0xffea:
		return wtypes.KeyRightAlt
	case sym == 0xffeb:
		return wtypes.KeyLeftSuper
	case sym == 0xffec:
		return wtypes.KeyRightSuper
	case sym == 0xff67:
		return wtypes.KeyMenu
	}
	return wtypes.KeyUnknown
}
