//go:build linux

package wayland

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/Quikcad/quikwin/internal/wtypes"
	"github.com/go-webgpu/goffi/ffi"
	vk "github.com/lukem570/vulkan-go/pkg/raw"
	wl "github.com/lukem570/wayland-go/pkg/wayland"
	xdg "github.com/lukem570/wayland-go/pkg/protocols/stable/xdgshell"
	xdgdeco "github.com/lukem570/wayland-go/pkg/protocols/unstable/xdgdecorationunstablev1"
)

type window struct {
	mu sync.Mutex

	// libwayland-client objects (unsafe.Pointer for Vulkan interop)
	display     unsafe.Pointer // wl_display*
	registry    unsafe.Pointer // wl_registry*
	compositor  unsafe.Pointer // wl_compositor*
	surface     unsafe.Pointer // wl_surface*
	xdgWmBase   unsafe.Pointer // xdg_wm_base*
	xdgSurface  unsafe.Pointer // xdg_surface*
	xdgToplevel unsafe.Pointer // xdg_toplevel*
	seat        unsafe.Pointer // wl_seat*
	keyboard    unsafe.Pointer // wl_keyboard*
	pointer     unsafe.Pointer // wl_pointer*
	decorMgr    unsafe.Pointer // zxdg_decoration_manager_v1*
	toplevelDecor unsafe.Pointer // zxdg_toplevel_decoration_v1*
	cursorShapeMgr unsafe.Pointer // wp_cursor_shape_manager_v1*
	cursorShapeDev unsafe.Pointer // wp_cursor_shape_device_v1*

	// XKB keyboard state
	xkbCtx    unsafe.Pointer
	xkbKeymap unsafe.Pointer
	xkbState  unsafe.Pointer

	globals map[string]globalEntry

	pendingSerial uint32
	configured    bool

	width, height       uint32
	minWidth, minHeight uint32
	appID               string

	ptrX, ptrY    float64
	ptrSerial     uint32
	cursorSerial  uint32

	resizable bool
	decorated bool
	dragging  bool

	// Key repeat state
	repeatRate  int32 // characters per second (0 = disabled)
	repeatDelay int32 // milliseconds before repeat starts
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

	// Listener structs — must stay alive as long as the window lives.
	registryListener      [2]uintptr
	wmBaseListener        [1]uintptr
	surfaceListener       [1]uintptr
	toplevelListener      [4]uintptr
	seatListener          [2]uintptr
	keyboardListener      [6]uintptr
	pointerListener       [9]uintptr
	toplevelDecorListener [1]uintptr

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

	w := &window{
		width:     cfg.Width,
		height:    cfg.Height,
		minWidth:  cfg.MinWidth,
		minHeight: cfg.MinHeight,
		resizable: cfg.Resizable,
		decorated: cfg.Decorated,
		globals:   make(map[string]globalEntry),
	}

	w.display = wlDisplayConnect(nil)
	if w.display == nil {
		return nil, fmt.Errorf("quikwin/wayland: wl_display_connect failed (WAYLAND_DISPLAY not set?)")
	}

	flags := xkbContextNoFlags
	var xkbCtx unsafe.Pointer
	ffi.CallFunction(&cifXkbContextNew, _xkbContextNew, unsafe.Pointer(&xkbCtx),
		[]unsafe.Pointer{unsafe.Pointer(&flags)})
	if xkbCtx == nil {
		wlDisplayDisconnect(w.display)
		return nil, fmt.Errorf("quikwin/wayland: xkb_context_new failed")
	}
	w.xkbCtx = xkbCtx

	w.registry = wlDisplayGetRegistry(w.display)
	w.setupRegistryListener()
	wlDisplayRoundtrip(w.display)

	if err := w.bindGlobals(); err != nil {
		w.Destroy()
		return nil, err
	}

	w.surface = wlCompositorCreateSurface(w.compositor)
	w.xdgSurface = xdgWmBaseGetXdgSurface(w.xdgWmBase, w.surface)
	w.xdgToplevel = xdgSurfaceGetToplevel(w.xdgSurface)

	w.setupListeners()

	if w.decorMgr != nil {
		w.toplevelDecor = zxdgDecorationMgrGetToplevelDecoration(w.decorMgr, w.xdgToplevel)
		if w.toplevelDecor != nil {
			decorConfigureCB := func(data, decor unsafe.Pointer, mode uint32) {}
			w.toplevelDecorListener[0] = ffi.NewCallback(decorConfigureCB)
			addListener(w.toplevelDecor, &w.toplevelDecorListener[0], unsafe.Pointer(w))
			zxdgToplevelDecorSetMode(w.toplevelDecor, uint32(xdgdeco.ToplevelDecorationV1ModeServerSide))
		}
	}

	xdgToplevelSetTitle(w.xdgToplevel, cfg.Title)
	if cfg.MinWidth > 0 || cfg.MinHeight > 0 {
		xdgToplevelSetMinSize(w.xdgToplevel, int32(cfg.MinWidth), int32(cfg.MinHeight))
	}

	// Wayland has no direct resizable flag. To prevent resizing, set
	// max_size equal to the requested size so the compositor won't
	// offer a resize handle.
	if !cfg.Resizable {
		xdgToplevelSetMaxSize(w.xdgToplevel, int32(cfg.Width), int32(cfg.Height))
		xdgToplevelSetMinSize(w.xdgToplevel, int32(cfg.Width), int32(cfg.Height))
	}

	// Decorations: if the compositor supports xdg-decoration, request
	// server-side (decorated) or client-side (undecorated).
	if !cfg.Decorated && w.toplevelDecor != nil {
		zxdgToplevelDecorSetMode(w.toplevelDecor, uint32(xdgdeco.ToplevelDecorationV1ModeClientSide))
	}

	wlSurfaceCommit(w.surface)
	wlDisplayRoundtrip(w.display)

	return w, nil
}

// --- Global registry ---

func (w *window) setupRegistryListener() {
	globalCB := func(data, reg unsafe.Pointer, name uint32, ifaceName *byte, version uint32) {
		iface := ptrToString(unsafe.Pointer(ifaceName))
		w.mu.Lock()
		w.globals[iface] = globalEntry{name: name, version: version}
		w.mu.Unlock()
	}
	globalRemoveCB := func(data, reg unsafe.Pointer, name uint32) {}

	w.registryListener[0] = ffi.NewCallback(globalCB)
	w.registryListener[1] = ffi.NewCallback(globalRemoveCB)

	var result int32
	wData := unsafe.Pointer(w)
	implPtr := unsafe.Pointer(&w.registryListener[0])
	ffi.CallFunction(&cifWlProxyAddListener, _wlProxyAddListener, unsafe.Pointer(&result),
		[]unsafe.Pointer{unsafe.Pointer(&w.registry), unsafe.Pointer(&implPtr), unsafe.Pointer(&wData)})
}

func (w *window) bindGlobals() error {
	bind := func(iface string, dest *unsafe.Pointer, ifaceDesc *wlInterface, version uint32) error {
		e, ok := w.globals[iface]
		if !ok {
			return fmt.Errorf("quikwin/wayland: compositor does not advertise %s", iface)
		}
		if e.version < version {
			version = e.version
		}
		p := registryBind(w.registry, e.name, ifaceDesc, version)
		if p == nil {
			return fmt.Errorf("quikwin/wayland: failed to bind %s", iface)
		}
		*dest = p
		return nil
	}

	if err := bind(wl.CompositorName, &w.compositor, &ifaceWlCompositor, 4); err != nil {
		return err
	}

	if err := bind(xdg.WmBaseName, &w.xdgWmBase, &ifaceXdgWmBase, 2); err != nil {
		return err
	}

	if e, ok := w.globals[xdgdeco.DecorationManagerV1Name]; ok {
		ver := e.version
		if ver > 1 {
			ver = 1
		}
		p := registryBind(w.registry, e.name, &ifaceZxdgDecorationManagerV1, ver)
		if p != nil {
			w.decorMgr = p
		}
	}

	if e, ok := w.globals[wl.SeatName]; ok {
		ver := e.version
		if ver > 5 {
			ver = 5
		}
		p := registryBind(w.registry, e.name, &ifaceWlSeat, ver)
		if p != nil {
			w.seat = p
		}
	}

	if e, ok := w.globals["wp_cursor_shape_manager_v1"]; ok {
		ver := e.version
		if ver > 1 {
			ver = 1
		}
		p := registryBind(w.registry, e.name, &ifaceWpCursorShapeManagerV1, ver)
		if p != nil {
			w.cursorShapeMgr = p
		}
	}

	return nil
}

// --- Listener setup ---

func (w *window) setupListeners() {
	pingCB := func(data, base unsafe.Pointer, serial uint32) {
		xdgWmBasePong(base, serial)
		wlDisplayFlush(w.display)
	}
	w.wmBaseListener[0] = ffi.NewCallback(pingCB)
	addListener(w.xdgWmBase, &w.wmBaseListener[0], unsafe.Pointer(w))

	surfConfigureCB := func(data, surf unsafe.Pointer, serial uint32) {
		w.mu.Lock()
		w.pendingSerial = serial
		w.mu.Unlock()
		xdgSurfaceAckConfigure(surf, serial)
		if !w.configured {
			w.configured = true
			wlSurfaceCommit(w.surface)
		}
	}
	w.surfaceListener[0] = ffi.NewCallback(surfConfigureCB)
	addListener(w.xdgSurface, &w.surfaceListener[0], unsafe.Pointer(w))

	toplevelConfigureCB := func(data, tl unsafe.Pointer, width, height int32, states unsafe.Pointer) {
		if width <= 0 || height <= 0 {
			return
		}
		nw, nh := uint32(width), uint32(height)
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
	toplevelCloseCB := func(data, tl unsafe.Pointer) {
		w.mu.Lock()
		w.shouldClose = true
		w.mu.Unlock()
		if fn := w.onClose; fn != nil {
			fn()
		}
	}
	toplevelConfigureBoundsCB := func(data, tl unsafe.Pointer, width, height int32) {}
	toplevelWMCapsCB := func(data, tl unsafe.Pointer, caps unsafe.Pointer) {}
	w.toplevelListener[0] = ffi.NewCallback(toplevelConfigureCB)
	w.toplevelListener[1] = ffi.NewCallback(toplevelCloseCB)
	w.toplevelListener[2] = ffi.NewCallback(toplevelConfigureBoundsCB)
	w.toplevelListener[3] = ffi.NewCallback(toplevelWMCapsCB)
	addListener(w.xdgToplevel, &w.toplevelListener[0], unsafe.Pointer(w))

	if w.seat != nil {
		w.setupSeatListeners()
	}
}

func (w *window) setupSeatListeners() {
	seatCapCB := func(data, seat unsafe.Pointer, caps uint32) {
		c := wl.SeatCapability(caps)
		if c&wl.SeatCapabilityKeyboard != 0 && w.keyboard == nil {
			w.keyboard = wlSeatGetKeyboard(seat)
			w.setupKeyboardListeners()
		}
		if c&wl.SeatCapabilityPointer != 0 && w.pointer == nil {
			w.pointer = wlSeatGetPointer(seat)
			w.setupPointerListeners()
			if w.cursorShapeMgr != nil {
				w.cursorShapeDev = wpCursorShapeMgrGetPointer(w.cursorShapeMgr, w.pointer)
			}
		}
	}
	seatNameCB := func(data, seat unsafe.Pointer, name *byte) {}
	w.seatListener[0] = ffi.NewCallback(seatCapCB)
	w.seatListener[1] = ffi.NewCallback(seatNameCB)
	addListener(w.seat, &w.seatListener[0], unsafe.Pointer(w))
}

func (w *window) setupKeyboardListeners() {
	keymapCB := func(data, kb unsafe.Pointer, format, fd uint32, size uint32) {
		if format != uint32(wl.KeyboardKeymapFormatXkbV1) {
			return
		}
		b := readFD(int(fd), int(size))
		if b == nil {
			return
		}
		var pin runtime.Pinner
		pin.Pin(&b[0])
		defer pin.Unpin()
		strPtr := unsafe.Pointer(&b[0])
		compileFlags := xkbKeymapCompileNoFlags
		var km unsafe.Pointer
		ffi.CallFunction(&cifXkbKeymapNewFromString, _xkbKeymapNewFromString, unsafe.Pointer(&km),
			[]unsafe.Pointer{
				unsafe.Pointer(&w.xkbCtx),
				unsafe.Pointer(&strPtr),
				unsafe.Pointer(&varXkbKeymapFormatTextV1),
				unsafe.Pointer(&compileFlags),
			})
		if km == nil {
			return
		}
		var state unsafe.Pointer
		ffi.CallFunction(&cifXkbStateNew, _xkbStateNew, unsafe.Pointer(&state),
			[]unsafe.Pointer{unsafe.Pointer(&km)})
		w.mu.Lock()
		if w.xkbKeymap != nil {
			old := w.xkbKeymap
			var r int32
			ffi.CallFunction(&cifXkbKeymapUnref, _xkbKeymapUnref, unsafe.Pointer(&r), []unsafe.Pointer{unsafe.Pointer(&old)})
		}
		if w.xkbState != nil {
			old := w.xkbState
			var r int32
			ffi.CallFunction(&cifXkbStateUnref, _xkbStateUnref, unsafe.Pointer(&r), []unsafe.Pointer{unsafe.Pointer(&old)})
		}
		w.xkbKeymap = km
		w.xkbState = state
		w.mu.Unlock()
	}
	enterCB := func(data, kb unsafe.Pointer, serial uint32, surf unsafe.Pointer, keys unsafe.Pointer) {
		if fn := w.onFocus; fn != nil {
			fn(true)
		}
	}
	leaveCB := func(data, kb unsafe.Pointer, serial uint32, surf unsafe.Pointer) {
		w.stopRepeat()
		if fn := w.onFocus; fn != nil {
			fn(false)
		}
	}
	keyCB := func(data, kb unsafe.Pointer, serial, time, key, state uint32) {
		// Stop any existing key repeat.
		w.stopRepeat()

		xkbKeycode := key + 8
		w.mu.Lock()
		xkbState := w.xkbState
		w.mu.Unlock()
		if xkbState == nil {
			return
		}
		var sym uint32
		ffi.CallFunction(&cifXkbStateKeyGetOneSym, _xkbStateKeyGetOneSym, unsafe.Pointer(&sym),
			[]unsafe.Pointer{unsafe.Pointer(&xkbState), unsafe.Pointer(&xkbKeycode)})
		k := keysymToKey(uint64(sym))
		action := wtypes.Press
		if wl.KeyboardKeyState(state) == wl.KeyboardKeyStateReleased {
			action = wtypes.Release
		}
		mods := w.currentMods()
		if fn := w.onKey; fn != nil {
			fn(k, action, mods)
		}
		if wl.KeyboardKeyState(state) == wl.KeyboardKeyStatePressed {
			var cp uint32
			ffi.CallFunction(&cifXkbStateKeyGetUtf32, _xkbStateKeyGetUtf32, unsafe.Pointer(&cp),
				[]unsafe.Pointer{unsafe.Pointer(&xkbState), unsafe.Pointer(&xkbKeycode)})
			if cp >= 32 && cp != 127 {
				if fn := w.onChar; fn != nil {
					fn(rune(cp))
				}
			}
			w.startRepeat(k, rune(cp), mods)
		}
	}
	modCB := func(data, kb unsafe.Pointer, serial, depressed, latched, locked, group uint32) {
		w.mu.Lock()
		st := w.xkbState
		w.mu.Unlock()
		if st == nil {
			return
		}
		var comp uint32
		ffi.CallFunction(&cifXkbStateUpdateMask, _xkbStateUpdateMask, unsafe.Pointer(&comp),
			[]unsafe.Pointer{
				unsafe.Pointer(&st),
				unsafe.Pointer(&depressed), unsafe.Pointer(&latched),
				unsafe.Pointer(&locked),
				unsafe.Pointer(&group), unsafe.Pointer(&group), unsafe.Pointer(&group),
			})
	}
	repeatInfoCB := func(data, kb unsafe.Pointer, rate, delay int32) {
		w.mu.Lock()
		w.repeatRate = rate
		w.repeatDelay = delay
		w.mu.Unlock()
	}

	w.keyboardListener[0] = ffi.NewCallback(keymapCB)
	w.keyboardListener[1] = ffi.NewCallback(enterCB)
	w.keyboardListener[2] = ffi.NewCallback(leaveCB)
	w.keyboardListener[3] = ffi.NewCallback(keyCB)
	w.keyboardListener[4] = ffi.NewCallback(modCB)
	w.keyboardListener[5] = ffi.NewCallback(repeatInfoCB)
	addListener(w.keyboard, &w.keyboardListener[0], unsafe.Pointer(w))
}

// stopRepeat cancels any running key repeat goroutine.
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

func (w *window) setupPointerListeners() {
	enterCB := func(data, ptr unsafe.Pointer, serial uint32, surf unsafe.Pointer, sx, sy int32) {
		x := wlFixedToFloat(sx)
		y := wlFixedToFloat(sy)
		w.mu.Lock()
		w.ptrSerial = serial
		w.cursorSerial = serial
		w.ptrX, w.ptrY = x, y
		w.mu.Unlock()

		if fn := w.onMouseMove; fn != nil {
			fn(x, y)
		}

		if !w.decorated && w.resizable && w.cursorShapeDev != nil {
			edge := wtypes.DetectEdge(x, y, float64(w.width), float64(w.height), wtypes.BorderWidth)
			if edge != wtypes.EdgeNone {
				wpCursorShapeDevSetShape(w.cursorShapeDev, serial, cursorShapeMap(wtypes.EdgeCursorShape(edge)))
				wlDisplayFlush(w.display)
			}
		}
	}
	leaveCB := func(data, ptr unsafe.Pointer, serial uint32, surf unsafe.Pointer) {}
	motionCB := func(data, ptr unsafe.Pointer, time uint32, sx, sy int32) {
		x := wlFixedToFloat(sx)
		y := wlFixedToFloat(sy)
		w.mu.Lock()
		w.ptrX, w.ptrY = x, y
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
		if !w.decorated && w.resizable && w.cursorShapeDev != nil {
			edge := wtypes.DetectEdge(x, y, float64(w.width), float64(w.height), wtypes.BorderWidth)
			if edge != wtypes.EdgeNone {
				serial := w.cursorSerial
				wpCursorShapeDevSetShape(w.cursorShapeDev, serial, cursorShapeMap(wtypes.EdgeCursorShape(edge)))
				wlDisplayFlush(w.display)
			}
		}
	}
	buttonCB := func(data, ptr unsafe.Pointer, serial, time, btn, state uint32) {
		w.mu.Lock()
		w.ptrSerial = serial
		w.mu.Unlock()
		b, ok := evdevButton(btn)
		if !ok {
			return
		}
		action := wtypes.Press
		if wl.PointerButtonState(state) == wl.PointerButtonStateReleased {
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
					xdgToplevelResize(w.xdgToplevel, w.seat, serial, uint32(edge))
					return
				}
			}
			if fn := w.onHitTest; fn != nil {
				if fn(w.ptrX, w.ptrY) == wtypes.HitTestDrag {
					xdgToplevelMove(w.xdgToplevel, w.seat, serial)
					return
				}
			}
		}
		if fn := w.onMouseButton; fn != nil {
			fn(b, action, mods)
		}
	}
	axisCB := func(data, ptr unsafe.Pointer, time, axis uint32, value int32) {
		v := wlFixedToFloat(value)
		if fn := w.onScroll; fn != nil {
			if wl.PointerAxis(axis) == wl.PointerAxisVerticalScroll {
				fn(0, -v/10)
			} else {
				fn(v/10, 0)
			}
		}
	}
	frameCB := func(data, ptr unsafe.Pointer) {}
	axisSourceCB := func(data, ptr unsafe.Pointer, source uint32) {}
	axisStopCB := func(data, ptr unsafe.Pointer, time, axis uint32) {}
	axisDiscreteCB := func(data, ptr unsafe.Pointer, axis uint32, discrete int32) {}

	w.pointerListener[0] = ffi.NewCallback(enterCB)
	w.pointerListener[1] = ffi.NewCallback(leaveCB)
	w.pointerListener[2] = ffi.NewCallback(motionCB)
	w.pointerListener[3] = ffi.NewCallback(buttonCB)
	w.pointerListener[4] = ffi.NewCallback(axisCB)
	w.pointerListener[5] = ffi.NewCallback(frameCB)
	w.pointerListener[6] = ffi.NewCallback(axisSourceCB)
	w.pointerListener[7] = ffi.NewCallback(axisStopCB)
	w.pointerListener[8] = ffi.NewCallback(axisDiscreteCB)
	addListener(w.pointer, &w.pointerListener[0], unsafe.Pointer(w))
}

// --- window.Window interface ---

func (w *window) Size() (uint32, uint32) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.width, w.height
}

func (w *window) Scale() float32 { return 1.0 }

func (w *window) ShouldClose() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.shouldClose
}

func (w *window) PollEvents() {
	wlDisplayDispatchPending(w.display)
	for wlDisplayPrepareRead(w.display) != 0 {
		wlDisplayDispatchPending(w.display)
	}
	wlDisplayFlush(w.display)
	if fdReadable(wlDisplayGetFd(w.display)) {
		wlDisplayReadEvents(w.display)
	} else {
		wlDisplayCancelRead(w.display)
	}
	wlDisplayDispatchPending(w.display)
}

func (w *window) SetTitle(title string) {
	xdgToplevelSetTitle(w.xdgToplevel, title)
	wlDisplayFlush(w.display)
}

func (w *window) SetCursor(shape wtypes.CursorShape) {
	w.mu.Lock()
	w.cursorShape = shape
	hidden := w.cursorHidden
	serial := w.cursorSerial
	w.mu.Unlock()
	if hidden || w.cursorShapeDev == nil {
		return
	}
	wpCursorShapeDevSetShape(w.cursorShapeDev, serial, cursorShapeMap(shape))
	wlDisplayFlush(w.display)
}

func (w *window) HideCursor() {
	w.mu.Lock()
	w.cursorHidden = true
	serial := w.cursorSerial
	w.mu.Unlock()
	if w.pointer == nil {
		return
	}
	wlPointerSetCursorNull(w.pointer, serial)
	wlDisplayFlush(w.display)
}

func (w *window) ShowCursor() {
	w.mu.Lock()
	w.cursorHidden = false
	serial := w.cursorSerial
	shape := w.cursorShape
	w.mu.Unlock()
	if w.cursorShapeDev == nil {
		return
	}
	wpCursorShapeDevSetShape(w.cursorShapeDev, serial, cursorShapeMap(shape))
	wlDisplayFlush(w.display)
}

func (w *window) SetMinSize(mw, mh uint32) {
	w.mu.Lock()
	w.minWidth = mw
	w.minHeight = mh
	w.mu.Unlock()
	xdgToplevelSetMinSize(w.xdgToplevel, int32(mw), int32(mh))
	wlSurfaceCommit(w.surface)
	wlDisplayFlush(w.display)
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

	proxyDestroy := func(p *unsafe.Pointer) {
		if *p != nil {
			wlProxyDestroy(*p)
			*p = nil
		}
	}
	proxyDestroy(&w.cursorShapeDev)
	proxyDestroy(&w.cursorShapeMgr)
	proxyDestroy(&w.pointer)
	proxyDestroy(&w.keyboard)
	proxyDestroy(&w.seat)
	proxyDestroy(&w.toplevelDecor)
	proxyDestroy(&w.decorMgr)
	proxyDestroy(&w.xdgToplevel)
	proxyDestroy(&w.xdgSurface)
	proxyDestroy(&w.surface)
	proxyDestroy(&w.xdgWmBase)
	proxyDestroy(&w.compositor)
	proxyDestroy(&w.registry)

	if w.xkbState != nil {
		var r int32
		ffi.CallFunction(&cifXkbStateUnref, _xkbStateUnref, unsafe.Pointer(&r), []unsafe.Pointer{unsafe.Pointer(&w.xkbState)})
	}
	if w.xkbKeymap != nil {
		var r int32
		ffi.CallFunction(&cifXkbKeymapUnref, _xkbKeymapUnref, unsafe.Pointer(&r), []unsafe.Pointer{unsafe.Pointer(&w.xkbKeymap)})
	}
	if w.xkbCtx != nil {
		var r int32
		ffi.CallFunction(&cifXkbContextUnref, _xkbContextUnref, unsafe.Pointer(&r), []unsafe.Pointer{unsafe.Pointer(&w.xkbCtx)})
	}
	wlDisplayDisconnect(w.display)
}

func (w *window) BeginDrag() {
	w.mu.Lock()
	serial := w.ptrSerial
	w.mu.Unlock()
	xdgToplevelMove(w.xdgToplevel, w.seat, serial)
}

// --- Event registration ---

func (w *window) OnResize(fn func(uint32, uint32))                              { w.onResize = fn }
func (w *window) OnLiveResize(fn func())                                        { w.onLiveResize = fn }
func (w *window) OnClose(fn func())                                             { w.onClose = fn }
func (w *window) OnFocus(fn func(bool))                                         { w.onFocus = fn }
func (w *window) OnKey(fn func(wtypes.Key, wtypes.Action, wtypes.Mod))          { w.onKey = fn }
func (w *window) OnChar(fn func(rune))                                          { w.onChar = fn }
func (w *window) OnMouseButton(fn func(wtypes.Button, wtypes.Action, wtypes.Mod)) {
	w.onMouseButton = fn
}
func (w *window) OnMouseMove(fn func(float64, float64))  { w.onMouseMove = fn }
func (w *window) OnScroll(fn func(float64, float64))     { w.onScroll = fn }
func (w *window) OnDragBegin(fn func(float64, float64))  { w.onDragBegin = fn }
func (w *window) OnDragMove(fn func(float64, float64))   { w.onDragMove = fn }
func (w *window) OnDragEnd(fn func(float64, float64))    { w.onDragEnd = fn }
func (w *window) OnDrop(fn func([]string))               { w.onDrop = fn }
func (w *window) OnHitTest(fn func(float64, float64) wtypes.HitTestResult) { w.onHitTest = fn }

// --- WaylandWindow interface ---

func (w *window) WlDisplay() uintptr { return uintptr(w.display) }
func (w *window) WlSurface() uintptr { return uintptr(w.surface) }

func (w *window) SetAppID(id string) {
	w.mu.Lock()
	w.appID = id
	w.mu.Unlock()
	xdgToplevelSetAppID(w.xdgToplevel, id)
	wlDisplayFlush(w.display)
}

// --- vkwin.Window interface ---

func (w *window) NewSurface(instance vk.Instance) (*vk.SurfaceKHR, error) {
	return instance.CreateWaylandSurfaceKHR(&vk.WaylandSurfaceCreateInfoKHR{
		Display: w.display,
		Surface: w.surface,
	}, nil)
}

// --- goffi wrappers ---

func wlDisplayConnect(name []byte) unsafe.Pointer {
	var namePtr unsafe.Pointer
	if name != nil {
		namePtr = unsafe.Pointer(&name[0])
	}
	var result unsafe.Pointer
	ffi.CallFunction(&cifWlDisplayConnect, _wlDisplayConnect, unsafe.Pointer(&result),
		[]unsafe.Pointer{unsafe.Pointer(&namePtr)})
	return result
}

func wlDisplayDisconnect(dpy unsafe.Pointer) {
	var r int32
	ffi.CallFunction(&cifWlDisplayDisconnect, _wlDisplayDisconnect, unsafe.Pointer(&r),
		[]unsafe.Pointer{unsafe.Pointer(&dpy)})
}

func wlDisplayRoundtrip(dpy unsafe.Pointer) {
	var result int32
	ffi.CallFunction(&cifWlDisplayRoundtrip, _wlDisplayRoundtrip, unsafe.Pointer(&result),
		[]unsafe.Pointer{unsafe.Pointer(&dpy)})
}

func wlDisplayFlush(dpy unsafe.Pointer) {
	var result int32
	ffi.CallFunction(&cifWlDisplayFlush, _wlDisplayFlush, unsafe.Pointer(&result),
		[]unsafe.Pointer{unsafe.Pointer(&dpy)})
}

func wlDisplayDispatchPending(dpy unsafe.Pointer) {
	var result int32
	ffi.CallFunction(&cifWlDisplayDispatchPending, _wlDisplayDispatchPending, unsafe.Pointer(&result),
		[]unsafe.Pointer{unsafe.Pointer(&dpy)})
}

func wlDisplayGetFd(dpy unsafe.Pointer) int32 {
	var result int32
	ffi.CallFunction(&cifWlDisplayGetFd, _wlDisplayGetFd, unsafe.Pointer(&result),
		[]unsafe.Pointer{unsafe.Pointer(&dpy)})
	return result
}

func wlDisplayPrepareRead(dpy unsafe.Pointer) int32 {
	var result int32
	ffi.CallFunction(&cifWlDisplayPrepareRead, _wlDisplayPrepareRead, unsafe.Pointer(&result),
		[]unsafe.Pointer{unsafe.Pointer(&dpy)})
	return result
}

func wlDisplayReadEvents(dpy unsafe.Pointer) {
	var result int32
	ffi.CallFunction(&cifWlDisplayReadEvents, _wlDisplayReadEvents, unsafe.Pointer(&result),
		[]unsafe.Pointer{unsafe.Pointer(&dpy)})
}

func wlDisplayCancelRead(dpy unsafe.Pointer) {
	ffi.CallFunction(&cifWlDisplayCancelRead, _wlDisplayCancelRead, nil,
		[]unsafe.Pointer{unsafe.Pointer(&dpy)})
}

func wlDisplayGetRegistry(dpy unsafe.Pointer) unsafe.Pointer {
	iface := unsafe.Pointer(&ifaceWlRegistry)
	var null unsafe.Pointer
	var result unsafe.Pointer
	ffi.CallFunction(&cifWlProxyMarshalConstructor, _wlProxyMarshalConstructor,
		unsafe.Pointer(&result),
		[]unsafe.Pointer{
			unsafe.Pointer(&dpy),
			unsafe.Pointer(&opcWlDisplayGetRegistry),
			unsafe.Pointer(&iface),
			unsafe.Pointer(&null),
		})
	return result
}

func registryBind(registry unsafe.Pointer, name uint32, iface *wlInterface, version uint32) unsafe.Pointer {
	ifaceName := iface.name
	args := [4]wlArgument{
		wlArgUint(name),
		wlArgStr(ifaceName),
		wlArgUint(version),
		wlArgNewID(),
	}
	var pin runtime.Pinner
	pin.Pin(&args[0])
	defer pin.Unpin()
	argsPtr := unsafe.Pointer(&args[0])
	ifacePtr := unsafe.Pointer(iface)
	var result unsafe.Pointer
	ffi.CallFunction(&cifWlProxyMarshalArrayConstructorVersioned, _wlProxyMarshalArrayConstructorVersioned,
		unsafe.Pointer(&result),
		[]unsafe.Pointer{
			unsafe.Pointer(&registry),
			unsafe.Pointer(&opcWlRegistryBind),
			unsafe.Pointer(&argsPtr),
			unsafe.Pointer(&ifacePtr),
			unsafe.Pointer(&version),
		})
	return result
}

func wlProxyDestroy(proxy unsafe.Pointer) {
	var r int32
	ffi.CallFunction(&cifWlProxyDestroy, _wlProxyDestroy, unsafe.Pointer(&r),
		[]unsafe.Pointer{unsafe.Pointer(&proxy)})
}

func addListener(proxy unsafe.Pointer, impl *uintptr, data unsafe.Pointer) {
	var result int32
	implPtr := unsafe.Pointer(impl)
	ffi.CallFunction(&cifWlProxyAddListener, _wlProxyAddListener, unsafe.Pointer(&result),
		[]unsafe.Pointer{unsafe.Pointer(&proxy), unsafe.Pointer(&implPtr), unsafe.Pointer(&data)})
}

func wlCompositorCreateSurface(compositor unsafe.Pointer) unsafe.Pointer {
	args := [1]wlArgument{wlArgNewID()}
	var pin runtime.Pinner
	pin.Pin(&args[0])
	defer pin.Unpin()
	argsPtr := unsafe.Pointer(&args[0])
	ifacePtr := unsafe.Pointer(&ifaceWlSurface)
	var result unsafe.Pointer
	ffi.CallFunction(&cifWlProxyMarshalArrayConstructorVersioned, _wlProxyMarshalArrayConstructorVersioned,
		unsafe.Pointer(&result),
		[]unsafe.Pointer{
			unsafe.Pointer(&compositor),
			unsafe.Pointer(&opcWlCompositorCreateSurface),
			unsafe.Pointer(&argsPtr),
			unsafe.Pointer(&ifacePtr),
			pU32(1),
		})
	return result
}

func xdgWmBaseGetXdgSurface(wmBase, surface unsafe.Pointer) unsafe.Pointer {
	args := [2]wlArgument{wlArgNewID(), wlArgPtr(surface)}
	var pin runtime.Pinner
	pin.Pin(&args[0])
	defer pin.Unpin()
	argsPtr := unsafe.Pointer(&args[0])
	ifacePtr := unsafe.Pointer(&ifaceXdgSurface)
	var result unsafe.Pointer
	ffi.CallFunction(&cifWlProxyMarshalArrayConstructorVersioned, _wlProxyMarshalArrayConstructorVersioned,
		unsafe.Pointer(&result),
		[]unsafe.Pointer{
			unsafe.Pointer(&wmBase),
			unsafe.Pointer(&opcXdgWmBaseGetXdgSurface),
			unsafe.Pointer(&argsPtr),
			unsafe.Pointer(&ifacePtr),
			pU32(1),
		})
	return result
}

func xdgSurfaceGetToplevel(surf unsafe.Pointer) unsafe.Pointer {
	args := [1]wlArgument{wlArgNewID()}
	var pin runtime.Pinner
	pin.Pin(&args[0])
	defer pin.Unpin()
	argsPtr := unsafe.Pointer(&args[0])
	ifacePtr := unsafe.Pointer(&ifaceXdgToplevel)
	var result unsafe.Pointer
	ffi.CallFunction(&cifWlProxyMarshalArrayConstructorVersioned, _wlProxyMarshalArrayConstructorVersioned,
		unsafe.Pointer(&result),
		[]unsafe.Pointer{
			unsafe.Pointer(&surf),
			unsafe.Pointer(&opcXdgSurfaceGetToplevel),
			unsafe.Pointer(&argsPtr),
			unsafe.Pointer(&ifacePtr),
			pU32(1),
		})
	return result
}

func xdgWmBasePong(wmBase unsafe.Pointer, serial uint32) {
	args := [1]wlArgument{wlArgUint(serial)}
	var pin runtime.Pinner
	pin.Pin(&args[0])
	defer pin.Unpin()
	argsPtr := unsafe.Pointer(&args[0])
	var r int32
	ffi.CallFunction(&cifWlProxyMarshalArray, _wlProxyMarshalArray, unsafe.Pointer(&r),
		[]unsafe.Pointer{
			unsafe.Pointer(&wmBase),
			unsafe.Pointer(&opcXdgWmBasePong),
			unsafe.Pointer(&argsPtr),
		})
}

func xdgSurfaceAckConfigure(surf unsafe.Pointer, serial uint32) {
	args := [1]wlArgument{wlArgUint(serial)}
	var pin runtime.Pinner
	pin.Pin(&args[0])
	defer pin.Unpin()
	argsPtr := unsafe.Pointer(&args[0])
	var r int32
	ffi.CallFunction(&cifWlProxyMarshalArray, _wlProxyMarshalArray, unsafe.Pointer(&r),
		[]unsafe.Pointer{
			unsafe.Pointer(&surf),
			unsafe.Pointer(&opcXdgSurfaceAckConfigure),
			unsafe.Pointer(&argsPtr),
		})
}

func xdgToplevelSetTitle(tl unsafe.Pointer, title string) {
	b := cstr(title)
	var pin runtime.Pinner
	pin.Pin(&b[0])
	defer pin.Unpin()
	args := [1]wlArgument{wlArgStr(&b[0])}
	var pin2 runtime.Pinner
	pin2.Pin(&args[0])
	defer pin2.Unpin()
	argsPtr := unsafe.Pointer(&args[0])
	var r int32
	ffi.CallFunction(&cifWlProxyMarshalArray, _wlProxyMarshalArray, unsafe.Pointer(&r),
		[]unsafe.Pointer{
			unsafe.Pointer(&tl),
			unsafe.Pointer(&opcXdgToplevelSetTitle),
			unsafe.Pointer(&argsPtr),
		})
}

func xdgToplevelSetAppID(tl unsafe.Pointer, id string) {
	b := cstr(id)
	var pin runtime.Pinner
	pin.Pin(&b[0])
	defer pin.Unpin()
	args := [1]wlArgument{wlArgStr(&b[0])}
	var pin2 runtime.Pinner
	pin2.Pin(&args[0])
	defer pin2.Unpin()
	argsPtr := unsafe.Pointer(&args[0])
	var r int32
	ffi.CallFunction(&cifWlProxyMarshalArray, _wlProxyMarshalArray, unsafe.Pointer(&r),
		[]unsafe.Pointer{
			unsafe.Pointer(&tl),
			unsafe.Pointer(&opcXdgToplevelSetAppID),
			unsafe.Pointer(&argsPtr),
		})
}

func xdgToplevelMove(tl, seat unsafe.Pointer, serial uint32) {
	args := [2]wlArgument{wlArgPtr(seat), wlArgUint(serial)}
	var pin runtime.Pinner
	pin.Pin(&args[0])
	defer pin.Unpin()
	argsPtr := unsafe.Pointer(&args[0])
	var r int32
	ffi.CallFunction(&cifWlProxyMarshalArray, _wlProxyMarshalArray, unsafe.Pointer(&r),
		[]unsafe.Pointer{
			unsafe.Pointer(&tl),
			unsafe.Pointer(&opcXdgToplevelMove),
			unsafe.Pointer(&argsPtr),
		})
}

func xdgToplevelResize(tl, seat unsafe.Pointer, serial, edges uint32) {
	args := [3]wlArgument{wlArgPtr(seat), wlArgUint(serial), wlArgUint(edges)}
	var pin runtime.Pinner
	pin.Pin(&args[0])
	defer pin.Unpin()
	argsPtr := unsafe.Pointer(&args[0])
	var r int32
	ffi.CallFunction(&cifWlProxyMarshalArray, _wlProxyMarshalArray, unsafe.Pointer(&r),
		[]unsafe.Pointer{
			unsafe.Pointer(&tl),
			unsafe.Pointer(&opcXdgToplevelResize),
			unsafe.Pointer(&argsPtr),
		})
}

func xdgToplevelSetMinSize(tl unsafe.Pointer, w, h int32) {
	args := [2]wlArgument{}
	*(*int32)(unsafe.Pointer(&args[0][0])) = w
	*(*int32)(unsafe.Pointer(&args[1][0])) = h
	var pin runtime.Pinner
	pin.Pin(&args[0])
	defer pin.Unpin()
	argsPtr := unsafe.Pointer(&args[0])
	var r int32
	ffi.CallFunction(&cifWlProxyMarshalArray, _wlProxyMarshalArray, unsafe.Pointer(&r),
		[]unsafe.Pointer{
			unsafe.Pointer(&tl),
			unsafe.Pointer(&opcXdgToplevelSetMinSize),
			unsafe.Pointer(&argsPtr),
		})
}

func xdgToplevelSetMaxSize(tl unsafe.Pointer, w, h int32) {
	args := [2]wlArgument{}
	*(*int32)(unsafe.Pointer(&args[0][0])) = w
	*(*int32)(unsafe.Pointer(&args[1][0])) = h
	var pin runtime.Pinner
	pin.Pin(&args[0])
	defer pin.Unpin()
	argsPtr := unsafe.Pointer(&args[0])
	var r int32
	ffi.CallFunction(&cifWlProxyMarshalArray, _wlProxyMarshalArray, unsafe.Pointer(&r),
		[]unsafe.Pointer{
			unsafe.Pointer(&tl),
			unsafe.Pointer(&opcXdgToplevelSetMaxSize),
			unsafe.Pointer(&argsPtr),
		})
}

func wlSurfaceCommit(surf unsafe.Pointer) {
	var result int32
	var nullArgs unsafe.Pointer
	ffi.CallFunction(&cifWlProxyMarshalArray, _wlProxyMarshalArray, unsafe.Pointer(&result),
		[]unsafe.Pointer{
			unsafe.Pointer(&surf),
			unsafe.Pointer(&opcWlSurfaceCommit),
			unsafe.Pointer(&nullArgs),
		})
}

func wlSeatGetKeyboard(seat unsafe.Pointer) unsafe.Pointer {
	args := [1]wlArgument{wlArgNewID()}
	var pin runtime.Pinner
	pin.Pin(&args[0])
	defer pin.Unpin()
	argsPtr := unsafe.Pointer(&args[0])
	ifacePtr := unsafe.Pointer(&ifaceWlKeyboard)
	var result unsafe.Pointer
	ffi.CallFunction(&cifWlProxyMarshalArrayConstructorVersioned, _wlProxyMarshalArrayConstructorVersioned,
		unsafe.Pointer(&result),
		[]unsafe.Pointer{
			unsafe.Pointer(&seat),
			unsafe.Pointer(&opcWlSeatGetKeyboard),
			unsafe.Pointer(&argsPtr),
			unsafe.Pointer(&ifacePtr),
			pU32(1),
		})
	return result
}

func wlSeatGetPointer(seat unsafe.Pointer) unsafe.Pointer {
	args := [1]wlArgument{wlArgNewID()}
	var pin runtime.Pinner
	pin.Pin(&args[0])
	defer pin.Unpin()
	argsPtr := unsafe.Pointer(&args[0])
	ifacePtr := unsafe.Pointer(&ifaceWlPointer)
	var result unsafe.Pointer
	ffi.CallFunction(&cifWlProxyMarshalArrayConstructorVersioned, _wlProxyMarshalArrayConstructorVersioned,
		unsafe.Pointer(&result),
		[]unsafe.Pointer{
			unsafe.Pointer(&seat),
			unsafe.Pointer(&opcWlSeatGetPointer),
			unsafe.Pointer(&argsPtr),
			unsafe.Pointer(&ifacePtr),
			pU32(1),
		})
	return result
}

func zxdgDecorationMgrGetToplevelDecoration(mgr, toplevel unsafe.Pointer) unsafe.Pointer {
	args := [2]wlArgument{wlArgNewID(), wlArgPtr(toplevel)}
	var pin runtime.Pinner
	pin.Pin(&args[0])
	defer pin.Unpin()
	argsPtr := unsafe.Pointer(&args[0])
	ifacePtr := unsafe.Pointer(&ifaceZxdgToplevelDecorationV1)
	var result unsafe.Pointer
	ffi.CallFunction(&cifWlProxyMarshalArrayConstructorVersioned, _wlProxyMarshalArrayConstructorVersioned,
		unsafe.Pointer(&result),
		[]unsafe.Pointer{
			unsafe.Pointer(&mgr),
			unsafe.Pointer(&opcZxdgDecorationMgrGetToplevelDecor),
			unsafe.Pointer(&argsPtr),
			unsafe.Pointer(&ifacePtr),
			pU32(1),
		})
	return result
}

func zxdgToplevelDecorSetMode(decor unsafe.Pointer, mode uint32) {
	args := [1]wlArgument{wlArgUint(mode)}
	var pin runtime.Pinner
	pin.Pin(&args[0])
	defer pin.Unpin()
	argsPtr := unsafe.Pointer(&args[0])
	var r int32
	ffi.CallFunction(&cifWlProxyMarshalArray, _wlProxyMarshalArray, unsafe.Pointer(&r),
		[]unsafe.Pointer{
			unsafe.Pointer(&decor),
			unsafe.Pointer(&opcZxdgToplevelDecorSetMode),
			unsafe.Pointer(&argsPtr),
		})
}

func wlPointerSetCursorNull(pointer unsafe.Pointer, serial uint32) {
	args := [4]wlArgument{
		wlArgUint(serial),
		{}, // NULL surface
		wlArgUint(0),
		wlArgUint(0),
	}
	var pin runtime.Pinner
	pin.Pin(&args[0])
	defer pin.Unpin()
	argsPtr := unsafe.Pointer(&args[0])
	var r int32
	ffi.CallFunction(&cifWlProxyMarshalArray, _wlProxyMarshalArray, unsafe.Pointer(&r),
		[]unsafe.Pointer{
			unsafe.Pointer(&pointer),
			unsafe.Pointer(&opcWlPointerSetCursor),
			unsafe.Pointer(&argsPtr),
		})
}

func wpCursorShapeMgrGetPointer(mgr, pointer unsafe.Pointer) unsafe.Pointer {
	args := [2]wlArgument{wlArgNewID(), wlArgPtr(pointer)}
	var pin runtime.Pinner
	pin.Pin(&args[0])
	defer pin.Unpin()
	argsPtr := unsafe.Pointer(&args[0])
	ifacePtr := unsafe.Pointer(&ifaceWpCursorShapeDeviceV1)
	var result unsafe.Pointer
	ffi.CallFunction(&cifWlProxyMarshalArrayConstructorVersioned, _wlProxyMarshalArrayConstructorVersioned,
		unsafe.Pointer(&result),
		[]unsafe.Pointer{
			unsafe.Pointer(&mgr),
			unsafe.Pointer(&opcWpCursorShapeMgrGetPointer),
			unsafe.Pointer(&argsPtr),
			unsafe.Pointer(&ifacePtr),
			pU32(1),
		})
	return result
}

func wpCursorShapeDevSetShape(dev unsafe.Pointer, serial, shape uint32) {
	args := [2]wlArgument{wlArgUint(serial), wlArgUint(shape)}
	var pin runtime.Pinner
	pin.Pin(&args[0])
	defer pin.Unpin()
	argsPtr := unsafe.Pointer(&args[0])
	var r int32
	ffi.CallFunction(&cifWlProxyMarshalArray, _wlProxyMarshalArray, unsafe.Pointer(&r),
		[]unsafe.Pointer{
			unsafe.Pointer(&dev),
			unsafe.Pointer(&opcWpCursorShapeDevSetShape),
			unsafe.Pointer(&argsPtr),
		})
}

// --- Helpers ---

func cstr(s string) []byte { return append([]byte(s), 0) }

func ptrToString(p unsafe.Pointer) string {
	if p == nil {
		return ""
	}
	b := (*[1 << 20]byte)(p)
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return ""
}

func pU32(v uint32) unsafe.Pointer { return unsafe.Pointer(&v) }

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

// wp_cursor_shape_device_v1.shape enum values.
const (
	wpCursorShapeDefault    = uint32(1)
	wpCursorShapePointer    = uint32(4)
	wpCursorShapeCrosshair  = uint32(8)
	wpCursorShapeText       = uint32(9)
	wpCursorShapeNotAllowed = uint32(15)
	wpCursorShapeEwResize   = uint32(26)
	wpCursorShapeNsResize   = uint32(27)
	wpCursorShapeNeswResize = uint32(28)
	wpCursorShapeNwseResize = uint32(29)
	wpCursorShapeAllScroll  = uint32(32)
)

func cursorShapeMap(shape wtypes.CursorShape) uint32 {
	switch shape {
	case wtypes.CursorArrow:
		return wpCursorShapeDefault
	case wtypes.CursorIBeam:
		return wpCursorShapeText
	case wtypes.CursorCrosshair:
		return wpCursorShapeCrosshair
	case wtypes.CursorHand:
		return wpCursorShapePointer
	case wtypes.CursorHResize:
		return wpCursorShapeEwResize
	case wtypes.CursorVResize:
		return wpCursorShapeNsResize
	case wtypes.CursorNWSEResize:
		return wpCursorShapeNwseResize
	case wtypes.CursorNESWResize:
		return wpCursorShapeNeswResize
	case wtypes.CursorAllResize:
		return wpCursorShapeAllScroll
	case wtypes.CursorNotAllowed:
		return wpCursorShapeNotAllowed
	default:
		return wpCursorShapeDefault
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
	modIdx := func(name string) uint32 {
		b := cstr(name)
		var pin runtime.Pinner
		pin.Pin(&b[0])
		defer pin.Unpin()
		namePtr := unsafe.Pointer(&b[0])
		var idx uint32
		ffi.CallFunction(&cifXkbKeymapModGetIndex, _xkbKeymapModGetIndex, unsafe.Pointer(&idx),
			[]unsafe.Pointer{unsafe.Pointer(&keymap), unsafe.Pointer(&namePtr)})
		return idx
	}
	isActive := func(idx uint32) bool {
		comp := xkbStateModsEffective
		var r int32
		ffi.CallFunction(&cifXkbStateModIndexIsActive, _xkbStateModIndexIsActive, unsafe.Pointer(&r),
			[]unsafe.Pointer{unsafe.Pointer(&state), unsafe.Pointer(&idx), unsafe.Pointer(&comp)})
		return r > 0
	}
	var m wtypes.Mod
	if isActive(modIdx("Shift")) {
		m |= wtypes.ModShift
	}
	if isActive(modIdx("Control")) {
		m |= wtypes.ModControl
	}
	if isActive(modIdx("Mod1")) {
		m |= wtypes.ModAlt
	}
	if isActive(modIdx("Mod4")) {
		m |= wtypes.ModSuper
	}
	if isActive(modIdx("Lock")) {
		m |= wtypes.ModCapsLock
	}
	if isActive(modIdx("Mod2")) {
		m |= wtypes.ModNumLock
	}
	return m
}

func readFD(fd, size int) []byte {
	if size <= 0 {
		return nil
	}
	b := make([]byte, size)
	n := 0
	for n < size {
		nn, err := sysRead(fd, b[n:])
		n += nn
		if err != nil {
			break
		}
	}
	sysClose(fd)
	if n < size {
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

func parseURIList(raw string) []string {
	var paths []string
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.HasPrefix(line, "file://") {
			p := strings.TrimPrefix(line, "file://")
			if p != "" {
				paths = append(paths, p)
			}
		}
	}
	return paths
}

