//go:build linux

package wayland

import (
	"encoding/binary"
	"fmt"
	"sync"
	"time"
	"unsafe"

	"github.com/Quikcad/quikwin/internal/wtypes"
	vk "github.com/lukem570/vulkan-go/pkg/raw"
	xdg "github.com/lukem570/wayland-go/pkg/protocols/stable/xdgshell"
	"github.com/lukem570/wayland-go/pkg/protocols/staging/cursorshapev1"
	xdgdeco "github.com/lukem570/wayland-go/pkg/protocols/unstable/xdgdecorationunstablev1"
	"github.com/lukem570/wayland-go/pkg/wayland"
	"github.com/lukem570/wayland-go/pkg/wl"
)

var (
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

	// libxkbcommon state (opaque C pointers).
	xkbCtx    unsafe.Pointer
	xkbKeymap unsafe.Pointer
	xkbState  unsafe.Pointer

	globals map[string]globalEntry

	pendingSerial uint32
	configured    bool

	width, height       uint32
	minWidth, minHeight uint32
	appID               string

	ptrX, ptrY   float64
	ptrSerial    uint32
	cursorSerial uint32

	resizable bool
	decorated bool
	dragging  bool
	maximized bool

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
	onScroll      func(float64, float64)
	onDragBegin   func(float64, float64)
	onDragMove    func(float64, float64)
	onDragEnd     func(float64, float64)
	onDrop        func([]string)
	onHitTest     func(float64, float64) wtypes.HitTestResult
	onCursorEnter func(float64, float64)

	cursorHidden bool
	cursorShape  wtypes.CursorShape

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

	w := &window{
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
	if cfg.MinWidth > 0 || cfg.MinHeight > 0 {
		w.xdgToplevel.SetMinSize(int32(cfg.MinWidth), int32(cfg.MinHeight))
	}

	// Wayland has no direct resizable flag. To prevent resizing, pin max_size
	// to min_size so the compositor won't offer a resize handle.
	if !cfg.Resizable {
		w.xdgToplevel.SetMaxSize(int32(cfg.Width), int32(cfg.Height))
		w.xdgToplevel.SetMinSize(int32(cfg.Width), int32(cfg.Height))
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

func (w *window) HandleToplevelConfigure(e xdg.ToplevelConfigureEvent) {
	maximized := statesContain(e.States, xdg.ToplevelStateMaximized)
	w.mu.Lock()
	w.maximized = maximized
	w.mu.Unlock()
	if e.Width <= 0 || e.Height <= 0 {
		return
	}
	nw, nh := uint32(e.Width), uint32(e.Height)
	w.mu.Lock()
	changed := nw != w.width || nh != w.height
	w.width, w.height = nw, nh
	w.mu.Unlock()
	if changed {
		if fn := w.onResize; fn != nil {
			fn(nw, nh)
		}
		if fn := w.onLiveResize; fn != nil {
			fn()
		}
	}
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
	if fn := w.onFocus; fn != nil {
		fn(true)
	}
}

func (w *window) HandleKeyboardLeave(e wayland.KeyboardLeaveEvent) {
	w.stopRepeat()
	if fn := w.onFocus; fn != nil {
		fn(false)
	}
}

func (w *window) HandleKeyboardKey(e wayland.KeyboardKeyEvent) {
	w.stopRepeat()

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
	mods := w.currentMods()
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
	w.applyEdgeCursor(x, y, e.Serial)
}

func (w *window) HandlePointerLeave(e wayland.PointerLeaveEvent) {}

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
	w.applyEdgeCursor(x, y, serial)
}

// applyEdgeCursor shows the matching resize cursor when the pointer is over an
// undecorated window's invisible resize border.
func (w *window) applyEdgeCursor(x, y float64, serial uint32) {
	if w.decorated || !w.resizable || w.cursorDev == nil {
		return
	}
	edge := wtypes.DetectEdge(x, y, float64(w.width), float64(w.height), wtypes.BorderWidth)
	if edge == wtypes.EdgeNone {
		return
	}
	w.cursorDev.SetShape(serial, cursorShapeMap(wtypes.EdgeCursorShape(edge)))
	wl.Flush(w.conn)
}

func (w *window) HandlePointerButton(e wayland.PointerButtonEvent) {
	w.mu.Lock()
	w.ptrSerial = e.Serial
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
			edge := wtypes.DetectEdge(w.ptrX, w.ptrY, float64(w.width), float64(w.height), wtypes.BorderWidth)
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
			fn(0, -v/10)
		} else {
			fn(v/10, 0)
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

// ClientDecorated reports whether the window draws its own decorations, which
// is the case whenever border or titlebar was disabled at creation.
func (w *window) ClientDecorated() bool { return !w.decorated }

func (w *window) Scale() float32 { return 1.0 }

func (w *window) ShouldClose() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.shouldClose
}

func (w *window) PollEvents() {
	d := w.conn.Pointer()
	wl.DispatchPending(w.conn)
	for displayPrepareRead(d) != 0 {
		wl.DispatchPending(w.conn)
	}
	// Issue deferred window-state requests now, outside any dispatch callback.
	w.applyPending()
	wl.Flush(w.conn)
	if fdReadable(displayGetFd(d)) {
		displayReadEvents(d)
	} else {
		displayCancelRead(d)
	}
	wl.DispatchPending(w.conn)
	if w.applyPending() {
		wl.Flush(w.conn)
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

func (w *window) SetCursor(shape wtypes.CursorShape) {
	w.mu.Lock()
	w.cursorShape = shape
	hidden := w.cursorHidden
	serial := w.cursorSerial
	w.mu.Unlock()
	if hidden || w.cursorDev == nil {
		return
	}
	w.cursorDev.SetShape(serial, cursorShapeMap(shape))
	wl.Flush(w.conn)
}

func (w *window) HideCursor() {
	w.mu.Lock()
	w.cursorHidden = true
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
	serial := w.cursorSerial
	shape := w.cursorShape
	w.mu.Unlock()
	if w.cursorDev == nil {
		return
	}
	w.cursorDev.SetShape(serial, cursorShapeMap(shape))
	wl.Flush(w.conn)
}

func (w *window) SetMinSize(mw, mh uint32) {
	w.mu.Lock()
	w.minWidth = mw
	w.minHeight = mh
	w.mu.Unlock()
	w.xdgToplevel.SetMinSize(int32(mw), int32(mh))
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

	destroy := func(p *wl.Proxy) {
		if p != nil {
			p.Destroy()
		}
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
func (w *window) OnScroll(fn func(float64, float64))                       { w.onScroll = fn }
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
