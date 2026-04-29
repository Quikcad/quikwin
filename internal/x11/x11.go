//go:build linux

package x11

import (
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"unsafe"

	"github.com/Quikcad/quikwin/internal/wtypes"
	"github.com/go-webgpu/goffi/ffi"
	vk "github.com/lukem570/vulkan-go/pkg/raw"
)

// XEvent on 64-bit Linux is a 192-byte union (long pad[24]).
type xEvent [192]byte

func (e *xEvent) typ() int32       { return *(*int32)(unsafe.Pointer(&e[0])) }
func (e *xEvent) window() uint64   { return *(*uint64)(unsafe.Pointer(&e[32])) }
func (e *xEvent) keyState() uint32 { return *(*uint32)(unsafe.Pointer(&e[80])) }
func (e *xEvent) keyCode() uint32  { return *(*uint32)(unsafe.Pointer(&e[84])) }
func (e *xEvent) btnButton() uint32 { return *(*uint32)(unsafe.Pointer(&e[84])) }
func (e *xEvent) btnState() uint32  { return *(*uint32)(unsafe.Pointer(&e[80])) }
func (e *xEvent) btnX() int32       { return *(*int32)(unsafe.Pointer(&e[64])) }
func (e *xEvent) btnY() int32       { return *(*int32)(unsafe.Pointer(&e[68])) }
func (e *xEvent) motionX() int32    { return *(*int32)(unsafe.Pointer(&e[64])) }
func (e *xEvent) motionY() int32    { return *(*int32)(unsafe.Pointer(&e[68])) }
func (e *xEvent) motionXRoot() int32 { return *(*int32)(unsafe.Pointer(&e[72])) }
func (e *xEvent) motionYRoot() int32 { return *(*int32)(unsafe.Pointer(&e[76])) }
func (e *xEvent) motionState() uint32 { return *(*uint32)(unsafe.Pointer(&e[80])) }
// XConfigureEvent: event=offset32, window=offset40, x=48, y=52, width=56, height=60
func (e *xEvent) configWidth() int32  { return *(*int32)(unsafe.Pointer(&e[56])) }
func (e *xEvent) configHeight() int32 { return *(*int32)(unsafe.Pointer(&e[60])) }
// XClientMessageEvent: message_type=offset40, data.l[0]=offset56
func (e *xEvent) clientMsgType() uint64 { return *(*uint64)(unsafe.Pointer(&e[40])) }
func (e *xEvent) clientMsgL0() uint64   { return *(*uint64)(unsafe.Pointer(&e[56])) }
func (e *xEvent) clientMsgL2() uint64   { return *(*uint64)(unsafe.Pointer(&e[72])) }
func (e *xEvent) clientMsgL3() uint64   { return *(*uint64)(unsafe.Pointer(&e[80])) }

// XWindowAttributes: x=0, y=4, width=8, height=12
type xWindowAttributes [232]byte

func (a *xWindowAttributes) x() int32 { return *(*int32)(unsafe.Pointer(&a[0])) }
func (a *xWindowAttributes) y() int32 { return *(*int32)(unsafe.Pointer(&a[4])) }

// XSizeHints: flags=0 (long/8 bytes), x=8, y=12, w=16, h=20, min_w=24, min_h=28
type xSizeHints [76]byte

func (h *xSizeHints) setMinSize(w, height int32) {
	*(*int64)(unsafe.Pointer(&h[0])) = 1 << 4  // PMinSize flag
	*(*int32)(unsafe.Pointer(&h[24])) = w
	*(*int32)(unsafe.Pointer(&h[28])) = height
}

// X11 event type constants
const (
	evKeyPress      = 2
	evKeyRelease    = 3
	evButtonPress   = 4
	evButtonRelease = 5
	evMotionNotify  = 6
	evFocusIn       = 9
	evFocusOut      = 10
	evConfigureNotify = 22
	evSelectionNotify = 31
	evClientMessage = 33
)

// X11 event mask constants
const (
	keyPressMask        int64 = 1 << 0
	keyReleaseMask      int64 = 1 << 1
	buttonPressMask     int64 = 1 << 2
	buttonReleaseMask   int64 = 1 << 3
	pointerMotionMask   int64 = 1 << 6
	structureNotifyMask int64 = 1 << 17
	focusChangeMask     int64 = 1 << 21
)

// X11 GrabMode constants
const (
	grabModeSync  int32 = 0
	grabModeAsync int32 = 1
)

// XChangeProperty mode
const (
	propModeReplace = int32(0)
)

// XA_ATOM type for XChangeProperty
const xaAtom = uint64(4)

// XA_CARDINAL
const xaCardinal = uint64(6)

// XA_STRING
const xaString = uint64(31)

// CurrentTime
const currentTime = uint64(0)

type window struct {
	mu  sync.Mutex
	dpy unsafe.Pointer // Display*
	win uint64         // Window XID

	width, height       uint32
	minWidth, minHeight uint32

	// Atoms
	wmDeleteWindow uint64
	wmProtocols    uint64
	// XDND atoms
	xdndAware      uint64
	xdndEnter      uint64
	xdndPosition   uint64
	xdndStatus     uint64
	xdndLeave      uint64
	xdndDrop       uint64
	xdndFinished   uint64
	xdndActionCopy uint64
	xdndSelection  uint64
	textUriList    uint64
	xdndTypeList   uint64

	// XDND drag-in state
	xdndSource    uint64
	xdndTimestamp uint64

	// Window-movement drag state
	dragging bool
	dragOffX int32
	dragOffY int32

	// Callbacks
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

	shouldClose bool
	destroyed   bool
}

// New creates an X11 window.
func New(title string, width, height, minWidth, minHeight uint32) (*window, error) {
	if err := ensureLoaded(); err != nil {
		return nil, err
	}

	dpy := xOpenDisplay(nil)
	if dpy == nil {
		return nil, fmt.Errorf("quikwin/x11: XOpenDisplay failed (DISPLAY not set?)")
	}

	root := xDefaultRootWindow(dpy)
	win := xCreateSimpleWindow(dpy, root, 0, 0, width, height, 0, 0, 0)

	w := &window{
		dpy:       dpy,
		win:       win,
		width:     width,
		height:    height,
		minWidth:  minWidth,
		minHeight: minHeight,
	}

	w.internAtoms()
	w.setupWMProtocols()
	w.setXdndAware()
	if minWidth > 0 || minHeight > 0 {
		w.applyMinSize()
	}

	eventMask := keyPressMask | keyReleaseMask | buttonPressMask | buttonReleaseMask |
		pointerMotionMask | structureNotifyMask | focusChangeMask
	xSelectInput(dpy, win, eventMask)

	xStoreName(dpy, win, title)
	xMapWindow(dpy, win)
	xFlush(dpy)

	return w, nil
}

func (w *window) internAtoms() {
	intern := func(name string, existing bool) uint64 {
		b := cstr(name)
		var pin runtime.Pinner
		pin.Pin(&b[0])
		defer pin.Unpin()
		var result uint64
		ffi.CallFunction(&cifXInternAtom, _XInternAtom, unsafe.Pointer(&result),
			[]unsafe.Pointer{unsafe.Pointer(&w.dpy), unsafe.Pointer(&b[0]), boolPtr(existing)})
		return result
	}
	w.wmDeleteWindow = intern("WM_DELETE_WINDOW", false)
	w.wmProtocols = intern("WM_PROTOCOLS", false)
	w.xdndAware = intern("XdndAware", false)
	w.xdndEnter = intern("XdndEnter", false)
	w.xdndPosition = intern("XdndPosition", false)
	w.xdndStatus = intern("XdndStatus", false)
	w.xdndLeave = intern("XdndLeave", false)
	w.xdndDrop = intern("XdndDrop", false)
	w.xdndFinished = intern("XdndFinished", false)
	w.xdndActionCopy = intern("XdndActionCopy", false)
	w.xdndSelection = intern("XdndSelection", false)
	w.textUriList = intern("text/uri-list", false)
	w.xdndTypeList = intern("XdndTypeList", false)
}

func (w *window) setupWMProtocols() {
	atom := w.wmDeleteWindow
	var pin runtime.Pinner
	pin.Pin(&atom)
	defer pin.Unpin()
	count := int32(1)
	var result int32
	ffi.CallFunction(&cifXSetWMProtocols, _XSetWMProtocols, unsafe.Pointer(&result),
		[]unsafe.Pointer{unsafe.Pointer(&w.dpy), unsafe.Pointer(&w.win), unsafe.Pointer(&atom), unsafe.Pointer(&count)})
}

func (w *window) setXdndAware() {
	// Set XdndAware property to version 5
	version := uint64(5)
	var pin runtime.Pinner
	pin.Pin(&version)
	defer pin.Unpin()
	var result int32
	n := int32(1)
	ffi.CallFunction(&cifXChangeProperty, _XChangeProperty, unsafe.Pointer(&result),
		[]unsafe.Pointer{
			unsafe.Pointer(&w.dpy), unsafe.Pointer(&w.win),
			unsafe.Pointer(&w.xdndAware), unsafe.Pointer(&xaCardinal_),
			unsafe.Pointer(&i32_32_), unsafe.Pointer(&propModeReplace_),
			unsafe.Pointer(&version), unsafe.Pointer(&n),
		})
}

func (w *window) applyMinSize() {
	var hints xSizeHints
	hints.setMinSize(int32(w.minWidth), int32(w.minHeight))
	var pin runtime.Pinner
	pin.Pin(&hints)
	defer pin.Unpin()
	ffi.CallFunction(&cifXSetWMNormalHints, _XSetWMNormalHints, nil,
		[]unsafe.Pointer{unsafe.Pointer(&w.dpy), unsafe.Pointer(&w.win), unsafe.Pointer(&hints)})
}

// ─── window.Window interface ──────────────────────────────────────────────────

func (w *window) Size() (uint32, uint32) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.width, w.height
}

func (w *window) Scale() float32 {
	key := cstr("Xft")
	val := cstr("dpi")
	var pin runtime.Pinner
	pin.Pin(&key[0])
	pin.Pin(&val[0])
	defer pin.Unpin()
	var result unsafe.Pointer
	ffi.CallFunction(&cifXGetDefault, _XGetDefault, unsafe.Pointer(&result),
		[]unsafe.Pointer{unsafe.Pointer(&w.dpy), unsafe.Pointer(&key[0]), unsafe.Pointer(&val[0])})
	if result == nil {
		return 1.0
	}
	s := ptrToString(result)
	dpi, err := strconv.ParseFloat(strings.TrimSpace(s), 32)
	if err != nil || dpi <= 0 {
		return 1.0
	}
	return float32(dpi) / 96.0
}

func (w *window) ShouldClose() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.shouldClose
}

func (w *window) PollEvents() {
	for xPending(w.dpy) > 0 {
		var ev xEvent
		xNextEvent(w.dpy, &ev)
		w.processEvent(&ev)
	}
	xFlush(w.dpy)
}

func (w *window) SetTitle(title string) {
	xStoreName(w.dpy, w.win, title)
	xFlush(w.dpy)
}

func (w *window) SetCursor(shape wtypes.CursorShape) {
	id := xfontCursorID(shape)
	cursor := xCreateFontCursor(w.dpy, id)
	xDefineCursor(w.dpy, w.win, cursor)
	xFlush(w.dpy)
}

func (w *window) SetMinSize(mw, mh uint32) {
	w.mu.Lock()
	w.minWidth = mw
	w.minHeight = mh
	w.mu.Unlock()
	w.applyMinSize()
	xFlush(w.dpy)
}

func (w *window) SetSize(sw, sh uint32) {
	xResizeWindow(w.dpy, w.win, sw, sh)
	xFlush(w.dpy)
}

func (w *window) Destroy() {
	w.mu.Lock()
	if w.destroyed {
		w.mu.Unlock()
		return
	}
	w.destroyed = true
	w.mu.Unlock()
	xDestroyWindow(w.dpy, w.win)
	xCloseDisplay(w.dpy)
}

func (w *window) BeginDrag() {
	// Record current window and pointer root positions.
	var rootWin, childWin uint64
	var rootX, rootY, winX, winY int32
	var mask uint32
	var pin runtime.Pinner
	pin.Pin(&rootWin)
	pin.Pin(&childWin)
	pin.Pin(&rootX)
	pin.Pin(&rootY)
	pin.Pin(&winX)
	pin.Pin(&winY)
	pin.Pin(&mask)
	var ok int32
	ffi.CallFunction(&cifXQueryPointer, _XQueryPointer, unsafe.Pointer(&ok),
		[]unsafe.Pointer{
			unsafe.Pointer(&w.dpy), unsafe.Pointer(&w.win),
			unsafe.Pointer(&rootWin), unsafe.Pointer(&childWin),
			unsafe.Pointer(&rootX), unsafe.Pointer(&rootY),
			unsafe.Pointer(&winX), unsafe.Pointer(&winY),
			unsafe.Pointer(&mask),
		})

	var attrs xWindowAttributes
	var pin2 runtime.Pinner
	pin2.Pin(&attrs)
	defer pin2.Unpin()
	var status int32
	ffi.CallFunction(&cifXGetWindowAttributes, _XGetWindowAttributes, unsafe.Pointer(&status),
		[]unsafe.Pointer{unsafe.Pointer(&w.dpy), unsafe.Pointer(&w.win), unsafe.Pointer(&attrs)})
	pin.Unpin()

	w.mu.Lock()
	w.dragging = true
	w.dragOffX = rootX - attrs.x()
	w.dragOffY = rootY - attrs.y()
	w.mu.Unlock()

	// Grab pointer to receive all subsequent events.
	evMask := uint32(buttonReleaseMask | pointerMotionMask)
	none := uint64(0)
	var grabResult int32
	ffi.CallFunction(&cifXGrabPointer, _XGrabPointer, unsafe.Pointer(&grabResult),
		[]unsafe.Pointer{
			unsafe.Pointer(&w.dpy), unsafe.Pointer(&w.win),
			boolPtr(false), unsafe.Pointer(&evMask),
			pI32(grabModeAsync), pI32(grabModeAsync),
			unsafe.Pointer(&none), unsafe.Pointer(&none),
			unsafe.Pointer(&none),
		})

	if fn := w.onDragBegin; fn != nil {
		fn(float64(winX), float64(winY))
	}
}

// ─── Event registration ───────────────────────────────────────────────────────

func (w *window) OnResize(fn func(uint32, uint32))           { w.onResize = fn }
func (w *window) OnLiveResize(fn func())                     { w.onLiveResize = fn }
func (w *window) OnClose(fn func())                          { w.onClose = fn }
func (w *window) OnFocus(fn func(bool))                      { w.onFocus = fn }
func (w *window) OnKey(fn func(wtypes.Key, wtypes.Action, wtypes.Mod)) { w.onKey = fn }
func (w *window) OnChar(fn func(rune))                       { w.onChar = fn }
func (w *window) OnMouseButton(fn func(wtypes.Button, wtypes.Action, wtypes.Mod)) {
	w.onMouseButton = fn
}
func (w *window) OnMouseMove(fn func(float64, float64))      { w.onMouseMove = fn }
func (w *window) OnScroll(fn func(float64, float64))         { w.onScroll = fn }
func (w *window) OnDragBegin(fn func(float64, float64))      { w.onDragBegin = fn }
func (w *window) OnDragMove(fn func(float64, float64))       { w.onDragMove = fn }
func (w *window) OnDragEnd(fn func(float64, float64))        { w.onDragEnd = fn }
func (w *window) OnDrop(fn func([]string))                   { w.onDrop = fn }

// ─── X11Window interface ──────────────────────────────────────────────────────

func (w *window) Display() (uintptr, error) {
	return uintptr(w.dpy), nil
}

func (w *window) XID() (uint32, error) {
	return uint32(w.win), nil
}

func (w *window) SetNetWMState(atoms ...string) error {
	if len(atoms) == 0 {
		return nil
	}
	vals := make([]uint64, len(atoms))
	for i, name := range atoms {
		b := cstr(name)
		var pin runtime.Pinner
		pin.Pin(&b[0])
		var a uint64
		var existing int32 = 0
		ffi.CallFunction(&cifXInternAtom, _XInternAtom, unsafe.Pointer(&a),
			[]unsafe.Pointer{unsafe.Pointer(&w.dpy), unsafe.Pointer(&b[0]), unsafe.Pointer(&existing)})
		pin.Unpin()
		vals[i] = a
	}
	netWmState := internAtomRaw(w.dpy, "_NET_WM_STATE")
	var pin runtime.Pinner
	pin.Pin(&vals[0])
	defer pin.Unpin()
	count := int32(len(vals))
	var result int32
	ffi.CallFunction(&cifXChangeProperty, _XChangeProperty, unsafe.Pointer(&result),
		[]unsafe.Pointer{
			unsafe.Pointer(&w.dpy), unsafe.Pointer(&w.win),
			unsafe.Pointer(&netWmState), unsafe.Pointer(&xaAtom_),
			unsafe.Pointer(&i32_32_), unsafe.Pointer(&propModeReplace_),
			unsafe.Pointer(&vals[0]), unsafe.Pointer(&count),
		})
	xFlush(w.dpy)
	return nil
}

// ─── vkwin.Window interface ───────────────────────────────────────────────────

func (w *window) NewSurface(instance vk.Instance) (*vk.SurfaceKHR, error) {
	return instance.CreateXlibSurfaceKHR(&vk.XlibSurfaceCreateInfoKHR{
		Dpy:    w.dpy,
		Window: uintptr(w.win),
	}, nil)
}

// ─── Event processing ─────────────────────────────────────────────────────────

func (w *window) processEvent(ev *xEvent) {
	switch ev.typ() {
	case evKeyPress, evKeyRelease:
		sym := xLookupKeysym(ev, 0)
		key := keysymToKey(sym)
		action := wtypes.Press
		if ev.typ() == evKeyRelease {
			action = wtypes.Release
		}
		mods := modFromState(ev.keyState())
		if fn := w.onKey; fn != nil {
			fn(key, action, mods)
		}
		if ev.typ() == evKeyPress {
			if fn := w.onChar; fn != nil {
				if r := keysymToRune(sym); r != 0 {
					fn(r)
				}
			}
		}

	case evButtonPress:
		btn := ev.btnButton()
		switch btn {
		case 4:
			if fn := w.onScroll; fn != nil {
				fn(0, 1)
			}
		case 5:
			if fn := w.onScroll; fn != nil {
				fn(0, -1)
			}
		case 6:
			if fn := w.onScroll; fn != nil {
				fn(-1, 0)
			}
		case 7:
			if fn := w.onScroll; fn != nil {
				fn(1, 0)
			}
		default:
			b, ok := x11Button(btn)
			if ok {
				mods := modFromState(ev.btnState())
				if fn := w.onMouseButton; fn != nil {
					fn(b, wtypes.Press, mods)
				}
			}
		}

	case evButtonRelease:
		btn := ev.btnButton()
		if btn >= 4 && btn <= 7 {
			break
		}
		if w.dragging {
			w.mu.Lock()
			w.dragging = false
			w.mu.Unlock()
			var t uint64
			ffi.CallFunction(&cifXUngrabPointer, _XUngrabPointer, nil,
				[]unsafe.Pointer{unsafe.Pointer(&w.dpy), unsafe.Pointer(&t)})
			if fn := w.onDragEnd; fn != nil {
				fn(float64(ev.btnX()), float64(ev.btnY()))
			}
		} else {
			b, ok := x11Button(btn)
			if ok {
				mods := modFromState(ev.btnState())
				if fn := w.onMouseButton; fn != nil {
					fn(b, wtypes.Release, mods)
				}
			}
		}

	case evMotionNotify:
		if w.dragging {
			newX := ev.motionXRoot() - w.dragOffX
			newY := ev.motionYRoot() - w.dragOffY
			xMoveWindow(w.dpy, w.win, newX, newY)
			if fn := w.onDragMove; fn != nil {
				fn(float64(ev.motionX()), float64(ev.motionY()))
			}
		} else {
			if fn := w.onMouseMove; fn != nil {
				fn(float64(ev.motionX()), float64(ev.motionY()))
			}
		}

	case evFocusIn:
		if fn := w.onFocus; fn != nil {
			fn(true)
		}
	case evFocusOut:
		if fn := w.onFocus; fn != nil {
			fn(false)
		}

	case evConfigureNotify:
		nw := uint32(ev.configWidth())
		nh := uint32(ev.configHeight())
		w.mu.Lock()
		changed := nw != w.width || nh != w.height
		w.width = nw
		w.height = nh
		w.mu.Unlock()
		if changed {
			if fn := w.onResize; fn != nil {
				fn(nw, nh)
			}
			if fn := w.onLiveResize; fn != nil {
				fn()
			}
		}

	case evClientMessage:
		msgType := ev.clientMsgType()
		if msgType == w.wmProtocols && ev.clientMsgL0() == w.wmDeleteWindow {
			w.mu.Lock()
			w.shouldClose = true
			w.mu.Unlock()
			if fn := w.onClose; fn != nil {
				fn()
			}
		} else if msgType == w.xdndEnter {
			w.xdndSource = ev.window()
		} else if msgType == w.xdndPosition {
			w.xdndSource = ev.clientMsgL0()
			w.sendXdndStatus()
		} else if msgType == w.xdndDrop {
			w.xdndTimestamp = ev.clientMsgL3()
			xConvertSelection(w.dpy, w.xdndSelection, w.textUriList, w.xdndSelection, w.win, w.xdndTimestamp)
		} else if msgType == w.xdndLeave {
			w.xdndSource = 0
		}

	case evSelectionNotify:
		// Selection data is ready - read and fire OnDrop.
		w.readXdndSelection()
	}
}

func (w *window) sendXdndStatus() {
	var ev xEvent
	*(*int32)(unsafe.Pointer(&ev[0])) = evClientMessage     // type
	*(*uint64)(unsafe.Pointer(&ev[32])) = w.xdndSource       // window
	*(*uint64)(unsafe.Pointer(&ev[40])) = w.xdndStatus       // message_type
	*(*int32)(unsafe.Pointer(&ev[48])) = 32                   // format
	*(*uint64)(unsafe.Pointer(&ev[56])) = w.win               // data.l[0] = our window
	*(*uint64)(unsafe.Pointer(&ev[64])) = 1                   // data.l[1] bit 0: accept
	*(*uint64)(unsafe.Pointer(&ev[88])) = w.xdndActionCopy   // data.l[4] = action

	src := w.xdndSource
	propagate := int32(0)
	mask := int64(0)
	var result int32
	var pin runtime.Pinner
	pin.Pin(&ev)
	defer pin.Unpin()
	ffi.CallFunction(&cifXSendEvent, _XSendEvent, unsafe.Pointer(&result),
		[]unsafe.Pointer{
			unsafe.Pointer(&w.dpy), unsafe.Pointer(&src),
			unsafe.Pointer(&propagate), unsafe.Pointer(&mask),
			unsafe.Pointer(&ev),
		})
}

func (w *window) readXdndSelection() {
	var actualType, actualFormat uint64
	var nitems, bytesAfter uint64
	var data unsafe.Pointer
	var pin runtime.Pinner
	pin.Pin(&actualType)
	pin.Pin(&actualFormat)
	pin.Pin(&nitems)
	pin.Pin(&bytesAfter)
	pin.Pin(&data)
	defer pin.Unpin()

	offset := int64(0)
	length := int64(65536 / 4)
	deleteIt := int32(1)
	var result int32
	ffi.CallFunction(&cifXGetWindowProperty, _XGetWindowProperty, unsafe.Pointer(&result),
		[]unsafe.Pointer{
			unsafe.Pointer(&w.dpy), unsafe.Pointer(&w.win),
			unsafe.Pointer(&w.xdndSelection),
			unsafe.Pointer(&offset), unsafe.Pointer(&length),
			unsafe.Pointer(&deleteIt), unsafe.Pointer(&xaAtom_),
			unsafe.Pointer(&actualType), unsafe.Pointer(&actualFormat),
			unsafe.Pointer(&nitems), unsafe.Pointer(&bytesAfter),
			unsafe.Pointer(&data),
		})

	if result != 0 || data == nil {
		return
	}

	raw := unsafe.Slice((*byte)(data), nitems)
	paths := parseURIList(string(raw))
	xFree(data)

	// Send XdndFinished to the source.
	w.sendXdndFinished()

	if fn := w.onDrop; fn != nil && len(paths) > 0 {
		fn(paths)
	}
}

func (w *window) sendXdndFinished() {
	var ev xEvent
	*(*int32)(unsafe.Pointer(&ev[0])) = evClientMessage
	*(*uint64)(unsafe.Pointer(&ev[32])) = w.xdndSource
	*(*uint64)(unsafe.Pointer(&ev[40])) = w.xdndFinished
	*(*int32)(unsafe.Pointer(&ev[48])) = 32
	*(*uint64)(unsafe.Pointer(&ev[56])) = w.win
	*(*uint64)(unsafe.Pointer(&ev[64])) = 1 // accepted
	*(*uint64)(unsafe.Pointer(&ev[72])) = w.xdndActionCopy

	src := w.xdndSource
	propagate := int32(0)
	mask := int64(0)
	var result int32
	var pin runtime.Pinner
	pin.Pin(&ev)
	defer pin.Unpin()
	ffi.CallFunction(&cifXSendEvent, _XSendEvent, unsafe.Pointer(&result),
		[]unsafe.Pointer{
			unsafe.Pointer(&w.dpy), unsafe.Pointer(&src),
			unsafe.Pointer(&propagate), unsafe.Pointer(&mask),
			unsafe.Pointer(&ev),
		})
}

// ─── goffi wrappers ───────────────────────────────────────────────────────────

func xOpenDisplay(name []byte) unsafe.Pointer {
	var namePtr unsafe.Pointer
	if name != nil {
		namePtr = unsafe.Pointer(&name[0])
	}
	var result unsafe.Pointer
	ffi.CallFunction(&cifXOpenDisplay, _XOpenDisplay, unsafe.Pointer(&result),
		[]unsafe.Pointer{unsafe.Pointer(&namePtr)})
	return result
}

func xCloseDisplay(dpy unsafe.Pointer) {
	var result int32
	ffi.CallFunction(&cifXCloseDisplay, _XCloseDisplay, unsafe.Pointer(&result),
		[]unsafe.Pointer{unsafe.Pointer(&dpy)})
}

func xDefaultRootWindow(dpy unsafe.Pointer) uint64 {
	var result uint64
	ffi.CallFunction(&cifXDefaultRootWindow, _XDefaultRootWindow, unsafe.Pointer(&result),
		[]unsafe.Pointer{unsafe.Pointer(&dpy)})
	return result
}

func xCreateSimpleWindow(dpy unsafe.Pointer, parent, x, y uint64, w, h, bw uint32, border, bg uint64) uint64 {
	var result uint64
	xi := int32(x)
	yi := int32(y)
	ffi.CallFunction(&cifXCreateSimpleWindow, _XCreateSimpleWindow, unsafe.Pointer(&result),
		[]unsafe.Pointer{
			unsafe.Pointer(&dpy), unsafe.Pointer(&parent),
			unsafe.Pointer(&xi), unsafe.Pointer(&yi),
			unsafe.Pointer(&w), unsafe.Pointer(&h),
			unsafe.Pointer(&bw), unsafe.Pointer(&border),
			unsafe.Pointer(&bg),
		})
	return result
}

func xMapWindow(dpy unsafe.Pointer, win uint64) {
	var result int32
	ffi.CallFunction(&cifXMapWindow, _XMapWindow, unsafe.Pointer(&result),
		[]unsafe.Pointer{unsafe.Pointer(&dpy), unsafe.Pointer(&win)})
}

func xDestroyWindow(dpy unsafe.Pointer, win uint64) {
	var result int32
	ffi.CallFunction(&cifXDestroyWindow, _XDestroyWindow, unsafe.Pointer(&result),
		[]unsafe.Pointer{unsafe.Pointer(&dpy), unsafe.Pointer(&win)})
}

func xSelectInput(dpy unsafe.Pointer, win uint64, mask int64) {
	var result int32
	ffi.CallFunction(&cifXSelectInput, _XSelectInput, unsafe.Pointer(&result),
		[]unsafe.Pointer{unsafe.Pointer(&dpy), unsafe.Pointer(&win), unsafe.Pointer(&mask)})
}

func xPending(dpy unsafe.Pointer) int32 {
	var result int32
	ffi.CallFunction(&cifXPending, _XPending, unsafe.Pointer(&result),
		[]unsafe.Pointer{unsafe.Pointer(&dpy)})
	return result
}

func xNextEvent(dpy unsafe.Pointer, ev *xEvent) {
	var result int32
	ffi.CallFunction(&cifXNextEvent, _XNextEvent, unsafe.Pointer(&result),
		[]unsafe.Pointer{unsafe.Pointer(&dpy), unsafe.Pointer(ev)})
}

func xStoreName(dpy unsafe.Pointer, win uint64, name string) {
	b := cstr(name)
	var pin runtime.Pinner
	pin.Pin(&b[0])
	defer pin.Unpin()
	var result int32
	ffi.CallFunction(&cifXStoreName, _XStoreName, unsafe.Pointer(&result),
		[]unsafe.Pointer{unsafe.Pointer(&dpy), unsafe.Pointer(&win), unsafe.Pointer(&b[0])})
}

func xResizeWindow(dpy unsafe.Pointer, win uint64, w, h uint32) {
	var result int32
	ffi.CallFunction(&cifXResizeWindow, _XResizeWindow, unsafe.Pointer(&result),
		[]unsafe.Pointer{unsafe.Pointer(&dpy), unsafe.Pointer(&win), unsafe.Pointer(&w), unsafe.Pointer(&h)})
}

func xCreateFontCursor(dpy unsafe.Pointer, shape uint32) uint64 {
	var result uint64
	ffi.CallFunction(&cifXCreateFontCursor, _XCreateFontCursor, unsafe.Pointer(&result),
		[]unsafe.Pointer{unsafe.Pointer(&dpy), unsafe.Pointer(&shape)})
	return result
}

func xDefineCursor(dpy unsafe.Pointer, win, cursor uint64) {
	var result int32
	ffi.CallFunction(&cifXDefineCursor, _XDefineCursor, unsafe.Pointer(&result),
		[]unsafe.Pointer{unsafe.Pointer(&dpy), unsafe.Pointer(&win), unsafe.Pointer(&cursor)})
}

func xFlush(dpy unsafe.Pointer) {
	var result int32
	ffi.CallFunction(&cifXFlush, _XFlush, unsafe.Pointer(&result),
		[]unsafe.Pointer{unsafe.Pointer(&dpy)})
}

func xMoveWindow(dpy unsafe.Pointer, win uint64, x, y int32) {
	var result int32
	ffi.CallFunction(&cifXMoveWindow, _XMoveWindow, unsafe.Pointer(&result),
		[]unsafe.Pointer{unsafe.Pointer(&dpy), unsafe.Pointer(&win), unsafe.Pointer(&x), unsafe.Pointer(&y)})
}

func xLookupKeysym(ev *xEvent, index int32) uint64 {
	var result uint64
	ffi.CallFunction(&cifXLookupKeysym, _XLookupKeysym, unsafe.Pointer(&result),
		[]unsafe.Pointer{unsafe.Pointer(ev), unsafe.Pointer(&index)})
	return result
}

func xConvertSelection(dpy unsafe.Pointer, selection, target, property, requestor, time uint64) {
	var result int32
	ffi.CallFunction(&cifXConvertSelection, _XConvertSelection, unsafe.Pointer(&result),
		[]unsafe.Pointer{
			unsafe.Pointer(&dpy), unsafe.Pointer(&selection),
			unsafe.Pointer(&target), unsafe.Pointer(&property),
			unsafe.Pointer(&requestor), unsafe.Pointer(&time),
		})
}

func xFree(data unsafe.Pointer) {
	var result int32
	ffi.CallFunction(&cifXFree, _XFree, unsafe.Pointer(&result),
		[]unsafe.Pointer{unsafe.Pointer(&data)})
}

func internAtomRaw(dpy unsafe.Pointer, name string) uint64 {
	b := cstr(name)
	var pin runtime.Pinner
	pin.Pin(&b[0])
	defer pin.Unpin()
	var result uint64
	existing := int32(0)
	ffi.CallFunction(&cifXInternAtom, _XInternAtom, unsafe.Pointer(&result),
		[]unsafe.Pointer{unsafe.Pointer(&dpy), unsafe.Pointer(&b[0]), unsafe.Pointer(&existing)})
	return result
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func cstr(s string) []byte {
	return append([]byte(s), 0)
}

func ptrToString(p unsafe.Pointer) string {
	b := (*[1 << 28]byte)(p)
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return ""
}

func boolPtr(v bool) unsafe.Pointer {
	if v {
		one := int32(1)
		return unsafe.Pointer(&one)
	}
	zero := int32(0)
	return unsafe.Pointer(&zero)
}

func pI32(v int32) unsafe.Pointer { return unsafe.Pointer(&v) }

// Package-level vars to avoid taking address of const expressions.
var (
	xaAtom_        = xaAtom
	xaCardinal_    = xaCardinal
	i32_32_        = int32(32)
	propModeReplace_ = propModeReplace
)

func x11Button(btn uint32) (wtypes.Button, bool) {
	switch btn {
	case 1:
		return wtypes.ButtonLeft, true
	case 2:
		return wtypes.ButtonMiddle, true
	case 3:
		return wtypes.ButtonRight, true
	case 8:
		return wtypes.Button4, true
	case 9:
		return wtypes.Button5, true
	}
	return 0, false
}

func keysymToRune(sym uint64) rune {
	if sym < 0x100 {
		r := rune(sym)
		if r >= 32 && r != 127 {
			return r
		}
	}
	// UCS keysyms: 0x01000000 + codepoint
	if sym&0xff000000 == 0x01000000 {
		return rune(sym & 0x00ffffff)
	}
	return 0
}

func parseURIList(raw string) []string {
	var paths []string
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.HasPrefix(line, "file://") {
			path := strings.TrimPrefix(line, "file://")
			path = strings.TrimSpace(path)
			if path != "" {
				paths = append(paths, path)
			}
		}
	}
	return paths
}
