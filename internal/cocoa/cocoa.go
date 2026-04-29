//go:build darwin

package cocoa

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/Quikcad/quikwin/internal/wtypes"
	"github.com/go-webgpu/goffi/ffi"
	vk "github.com/lukem570/vulkan-go/pkg/raw"
)

// ---------------------------------------------------------------------------
// Selectors (registered once on first use)
// ---------------------------------------------------------------------------

var (
	selInit                           unsafe.Pointer
	selAlloc                          unsafe.Pointer
	selRelease                        unsafe.Pointer
	selSharedApplication              unsafe.Pointer
	selSetActivationPolicy            unsafe.Pointer
	selActivateIgnoringOtherApps      unsafe.Pointer
	selNextEventMatchingMask          unsafe.Pointer
	selSendEvent                      unsafe.Pointer
	selUpdateWindows                  unsafe.Pointer
	selSetDelegate                    unsafe.Pointer
	selDistantPast                    unsafe.Pointer
	selStringWithUTF8String           unsafe.Pointer
	selInitWithContentRect            unsafe.Pointer
	selMakeKeyAndOrderFront           unsafe.Pointer
	selSetTitle                       unsafe.Pointer
	selSetReleasedWhenClosed          unsafe.Pointer
	selSetMinSize                     unsafe.Pointer
	selSetContentSize                 unsafe.Pointer
	selSetStyleMask                   unsafe.Pointer
	selClose                          unsafe.Pointer
	selContentView                    unsafe.Pointer
	selSetWantsLayer                  unsafe.Pointer
	selLayer                          unsafe.Pointer
	selBackingScaleFactor             unsafe.Pointer
	selMainScreen                     unsafe.Pointer
	selType                           unsafe.Pointer
	selKeyCode                        unsafe.Pointer
	selModifierFlags                  unsafe.Pointer
	selIsARepeat                      unsafe.Pointer
	selCharacters                     unsafe.Pointer
	selUTF8String                     unsafe.Pointer
	selButtonNumber                   unsafe.Pointer
	selDeltaX                         unsafe.Pointer
	selDeltaY                         unsafe.Pointer
	selScrollingDeltaX                unsafe.Pointer
	selScrollingDeltaY                unsafe.Pointer
	selSetTitlebarAppearsTransparent  unsafe.Pointer
	selSetTitleVisibility             unsafe.Pointer
	selStandardWindowButton           unsafe.Pointer
	selSetFrameOrigin                 unsafe.Pointer

	selsOnce sync.Once
)

func initSels() {
	selsOnce.Do(func() {
		reg := func(name string) unsafe.Pointer {
			p := bstr(name)
			var r unsafe.Pointer
			ffi.CallFunction(&cifSelRegister, _selRegisterName, unsafe.Pointer(&r),
				[]unsafe.Pointer{unsafe.Pointer(p)})
			return r
		}
		selInit = reg("init")
		selAlloc = reg("alloc")
		selRelease = reg("release")
		selSharedApplication = reg("sharedApplication")
		selSetActivationPolicy = reg("setActivationPolicy:")
		selActivateIgnoringOtherApps = reg("activateIgnoringOtherApps:")
		selNextEventMatchingMask = reg("nextEventMatchingMask:untilDate:inMode:dequeue:")
		selSendEvent = reg("sendEvent:")
		selUpdateWindows = reg("updateWindows")
		selSetDelegate = reg("setDelegate:")
		selDistantPast = reg("distantPast")
		selStringWithUTF8String = reg("stringWithUTF8String:")
		selInitWithContentRect = reg("initWithContentRect:styleMask:backing:defer:")
		selMakeKeyAndOrderFront = reg("makeKeyAndOrderFront:")
		selSetTitle = reg("setTitle:")
		selSetReleasedWhenClosed = reg("setReleasedWhenClosed:")
		selSetMinSize = reg("setMinSize:")
		selSetContentSize = reg("setContentSize:")
		selSetStyleMask = reg("setStyleMask:")
		selClose = reg("close")
		selContentView = reg("contentView")
		selSetWantsLayer = reg("setWantsLayer:")
		selLayer = reg("layer")
		selBackingScaleFactor = reg("backingScaleFactor")
		selMainScreen = reg("mainScreen")
		selType = reg("type")
		selKeyCode = reg("keyCode")
		selModifierFlags = reg("modifierFlags")
		selIsARepeat = reg("isARepeat")
		selCharacters = reg("characters")
		selUTF8String = reg("UTF8String")
		selButtonNumber = reg("buttonNumber")
		selDeltaX = reg("deltaX")
		selDeltaY = reg("deltaY")
		selScrollingDeltaX = reg("scrollingDeltaX")
		selScrollingDeltaY = reg("scrollingDeltaY")
		selSetTitlebarAppearsTransparent = reg("setTitlebarAppearsTransparent:")
		selSetTitleVisibility = reg("setTitleVisibility:")
		selStandardWindowButton = reg("standardWindowButton:")
		selSetFrameOrigin = reg("setFrameOrigin:")
	})
}

func bstr(s string) *byte {
	b := make([]byte, len(s)+1)
	copy(b, s)
	return &b[0]
}

// ---------------------------------------------------------------------------
// ObjC message send helpers
// ---------------------------------------------------------------------------

func msgSend0(recv, sel unsafe.Pointer) unsafe.Pointer {
	var ret unsafe.Pointer
	ffi.CallFunction(&cifMsg0, _objcMsgSend, unsafe.Pointer(&ret),
		[]unsafe.Pointer{recv, sel})
	return ret
}

func msgSend1p(recv, sel, arg unsafe.Pointer) unsafe.Pointer {
	var ret unsafe.Pointer
	ffi.CallFunction(&cifMsg1p, _objcMsgSend, unsafe.Pointer(&ret),
		[]unsafe.Pointer{recv, sel, arg})
	return ret
}

func msgSend1pVoid(recv, sel, arg unsafe.Pointer) {
	ffi.CallFunction(&cifMsg1pVoid, _objcMsgSend, nil,
		[]unsafe.Pointer{recv, sel, arg})
}

func msgSend1iVoid(recv, sel unsafe.Pointer, v int64) {
	ffi.CallFunction(&cifMsg1iVoid, _objcMsgSend, nil,
		[]unsafe.Pointer{recv, sel, unsafe.Pointer(&v)})
}

func msgSend1bVoid(recv, sel unsafe.Pointer, v int32) {
	ffi.CallFunction(&cifMsg1bVoid, _objcMsgSend, nil,
		[]unsafe.Pointer{recv, sel, unsafe.Pointer(&v)})
}

func msgSend0i(recv, sel unsafe.Pointer) int64 {
	var ret int64
	ffi.CallFunction(&cifMsg0i, _objcMsgSend, unsafe.Pointer(&ret),
		[]unsafe.Pointer{recv, sel})
	return ret
}

func msgSend0u(recv, sel unsafe.Pointer) uint64 {
	var ret uint64
	ffi.CallFunction(&cifMsg0u, _objcMsgSend, unsafe.Pointer(&ret),
		[]unsafe.Pointer{recv, sel})
	return ret
}

func msgSend0f(recv, sel unsafe.Pointer) float64 {
	var ret float64
	// arm64: plain objc_msgSend; x86_64: double returned in XMM0, libffi reads
	// it correctly with DoubleType return descriptor.
	ffi.CallFunction(&cifMsg0f, _objcMsgSend, unsafe.Pointer(&ret),
		[]unsafe.Pointer{recv, sel})
	return ret
}

func msgSend2fVoid(recv, sel unsafe.Pointer, x, y float64) {
	ffi.CallFunction(&cifMsg2fVoid, _objcMsgSend, nil,
		[]unsafe.Pointer{recv, sel, unsafe.Pointer(&x), unsafe.Pointer(&y)})
}

func msgSendInitWindow(recv, sel unsafe.Pointer, x, y, w, h float64, style, backing uint64, deferred int32) unsafe.Pointer {
	var ret unsafe.Pointer
	ffi.CallFunction(&cifMsgInitWindow, _objcMsgSend, unsafe.Pointer(&ret),
		[]unsafe.Pointer{recv, sel,
			unsafe.Pointer(&x), unsafe.Pointer(&y), unsafe.Pointer(&w), unsafe.Pointer(&h),
			unsafe.Pointer(&style), unsafe.Pointer(&backing), unsafe.Pointer(&deferred)})
	return ret
}

func nsString(s string) unsafe.Pointer {
	p := bstr(s)
	cls := getClass("NSString")
	return msgSend1p(cls, selStringWithUTF8String, unsafe.Pointer(p))
}

func getClass(name string) unsafe.Pointer {
	p := bstr(name)
	var ret unsafe.Pointer
	ffi.CallFunction(&cifGetClass, _objcGetClass, unsafe.Pointer(&ret),
		[]unsafe.Pointer{unsafe.Pointer(p)})
	return ret
}

// ---------------------------------------------------------------------------
// NSApplication setup (once per process)
// ---------------------------------------------------------------------------

var (
	nsApp     unsafe.Pointer
	nsAppOnce sync.Once
)

func initApp() {
	nsAppOnce.Do(func() {
		cls := getClass("NSApplication")
		nsApp = msgSend0(cls, selSharedApplication)
		var policy int64 // NSApplicationActivationPolicyRegular = 0
		ffi.CallFunction(&cifMsg1iVoid, _objcMsgSend, nil,
			[]unsafe.Pointer{nsApp, selSetActivationPolicy, unsafe.Pointer(&policy)})
		var yes int32 = 1
		ffi.CallFunction(&cifMsg1bVoid, _objcMsgSend, nil,
			[]unsafe.Pointer{nsApp, selActivateIgnoringOtherApps, unsafe.Pointer(&yes)})
	})
}

// ---------------------------------------------------------------------------
// Delegate class
// ---------------------------------------------------------------------------

var (
	delegateClass     unsafe.Pointer
	delegateClassOnce sync.Once

	delegateMu  sync.Mutex
	delegateMap = map[uintptr]*window{}
)

func lookupDelegate(self unsafe.Pointer) *window {
	delegateMu.Lock()
	w := delegateMap[uintptr(self)]
	delegateMu.Unlock()
	return w
}

func registerDelegate(self unsafe.Pointer, w *window) {
	delegateMu.Lock()
	delegateMap[uintptr(self)] = w
	delegateMu.Unlock()
}

func unregisterDelegate(self unsafe.Pointer) {
	delegateMu.Lock()
	delete(delegateMap, uintptr(self))
	delegateMu.Unlock()
}

func initDelegateClass() {
	delegateClassOnce.Do(func() {
		superCls := getClass("NSObject")
		className := bstr("QuikwinWindowDelegate")
		var extraBytes uint64
		ffi.CallFunction(&cifAllocClassPair, _objcAllocateClassPair, unsafe.Pointer(&delegateClass),
			[]unsafe.Pointer{superCls, unsafe.Pointer(className), unsafe.Pointer(&extraBytes)})

		addMethod("windowWillClose:", "v@:@", func(self, _cmd, notif uintptr) {
			if w := lookupDelegate(unsafe.Pointer(self)); w != nil {
				w.closed.Store(true)
				if fn := w.onClose; fn != nil {
					fn()
				}
			}
		})
		addMethod("windowDidResize:", "v@:@", func(self, _cmd, notif uintptr) {
			if w := lookupDelegate(unsafe.Pointer(self)); w != nil {
				if fn := w.onLiveResize; fn != nil {
					fn()
				}
			}
		})
		addMethod("windowDidBecomeKey:", "v@:@", func(self, _cmd, notif uintptr) {
			if w := lookupDelegate(unsafe.Pointer(self)); w != nil {
				if fn := w.onFocus; fn != nil {
					fn(true)
				}
			}
		})
		addMethod("windowDidResignKey:", "v@:@", func(self, _cmd, notif uintptr) {
			if w := lookupDelegate(unsafe.Pointer(self)); w != nil {
				if fn := w.onFocus; fn != nil {
					fn(false)
				}
			}
		})
		addMethod("windowShouldClose:", "i@:@", func(self, _cmd, sender uintptr) uintptr {
			return 1 // YES
		})

		ffi.CallFunction(&cifRegisterClassPair, _objcRegisterClassPair, nil,
			[]unsafe.Pointer{delegateClass})
	})
}

// addMethod registers a Go callback as an ObjC IMP on delegateClass.
// All params must be uintptr-sized (ObjC always passes id/SEL as pointer-width words).
func addMethod(selName, typeStr string, fn any) {
	sel := bstr(selName)
	var selPtr unsafe.Pointer
	ffi.CallFunction(&cifSelRegister, _selRegisterName, unsafe.Pointer(&selPtr),
		[]unsafe.Pointer{unsafe.Pointer(sel)})

	imp := unsafe.Pointer(ffi.NewCallback(fn))
	ts := bstr(typeStr)
	ffi.CallFunction(&cifClassAddMethod, _classAddMethod, nil,
		[]unsafe.Pointer{delegateClass, selPtr, imp, unsafe.Pointer(ts)})
}

// ---------------------------------------------------------------------------
// NSEvent type constants
// ---------------------------------------------------------------------------

const (
	nsEventTypeLeftMouseDown     int64 = 1
	nsEventTypeLeftMouseUp       int64 = 2
	nsEventTypeRightMouseDown    int64 = 3
	nsEventTypeRightMouseUp      int64 = 4
	nsEventTypeMouseMoved        int64 = 5
	nsEventTypeLeftMouseDragged  int64 = 6
	nsEventTypeRightMouseDragged int64 = 7
	nsEventTypeKeyDown           int64 = 10
	nsEventTypeKeyUp             int64 = 11
	nsEventTypeFlagsChanged      int64 = 12
	nsEventTypeScrollWheel       int64 = 22
	nsEventTypeOtherMouseDown    int64 = 25
	nsEventTypeOtherMouseUp      int64 = 26
	nsEventTypeOtherMouseDragged int64 = 27
)

const nsEventMaskAny uint64 = ^uint64(0) // NSEventMaskAny

// NSWindow style masks
const (
	nsWindowStyleMaskTitled              uint64 = 1 << 0
	nsWindowStyleMaskClosable            uint64 = 1 << 1
	nsWindowStyleMaskMiniaturizable      uint64 = 1 << 2
	nsWindowStyleMaskResizable           uint64 = 1 << 3
	nsWindowStyleMaskFullSizeContentView uint64 = 1 << 15
)

const nsBackingStoreBuffered uint64 = 2

// NSWindowButton
const (
	nsWindowCloseButton       int64 = 0
	nsWindowMiniaturizeButton int64 = 1
	nsWindowZoomButton        int64 = 2
)

// NSEventModifierFlags
const (
	nsEventModifierFlagCapsLock uint64 = 1 << 16
	nsEventModifierFlagShift    uint64 = 1 << 17
	nsEventModifierFlagControl  uint64 = 1 << 18
	nsEventModifierFlagOption   uint64 = 1 << 19
	nsEventModifierFlagCommand  uint64 = 1 << 20
)

// ---------------------------------------------------------------------------
// macOS virtual key → wtypes.Key
// ---------------------------------------------------------------------------

var vkToKey = [256]wtypes.Key{
	0x00: wtypes.KeyA,
	0x01: wtypes.KeyS,
	0x02: wtypes.KeyD,
	0x03: wtypes.KeyF,
	0x04: wtypes.KeyH,
	0x05: wtypes.KeyG,
	0x06: wtypes.KeyZ,
	0x07: wtypes.KeyX,
	0x08: wtypes.KeyC,
	0x09: wtypes.KeyV,
	0x0B: wtypes.KeyB,
	0x0C: wtypes.KeyQ,
	0x0D: wtypes.KeyW,
	0x0E: wtypes.KeyE,
	0x0F: wtypes.KeyR,
	0x10: wtypes.KeyY,
	0x11: wtypes.KeyT,
	0x12: wtypes.Key1,
	0x13: wtypes.Key2,
	0x14: wtypes.Key3,
	0x15: wtypes.Key4,
	0x16: wtypes.Key6,
	0x17: wtypes.Key5,
	0x18: wtypes.KeyEqual,
	0x19: wtypes.Key9,
	0x1A: wtypes.Key7,
	0x1B: wtypes.KeyMinus,
	0x1C: wtypes.Key8,
	0x1D: wtypes.Key0,
	0x1E: wtypes.KeyRightBracket,
	0x1F: wtypes.KeyO,
	0x20: wtypes.KeyU,
	0x21: wtypes.KeyLeftBracket,
	0x22: wtypes.KeyI,
	0x23: wtypes.KeyP,
	0x24: wtypes.KeyEnter,
	0x25: wtypes.KeyL,
	0x26: wtypes.KeyJ,
	0x27: wtypes.KeyApostrophe,
	0x28: wtypes.KeyK,
	0x29: wtypes.KeySemicolon,
	0x2A: wtypes.KeyBackslash,
	0x2B: wtypes.KeyComma,
	0x2C: wtypes.KeySlash,
	0x2D: wtypes.KeyN,
	0x2E: wtypes.KeyM,
	0x2F: wtypes.KeyPeriod,
	0x30: wtypes.KeyTab,
	0x31: wtypes.KeySpace,
	0x32: wtypes.KeyGraveAccent,
	0x33: wtypes.KeyBackspace,
	0x35: wtypes.KeyEscape,
	0x38: wtypes.KeyLeftShift,
	0x3A: wtypes.KeyLeftAlt,
	0x3B: wtypes.KeyLeftControl,
	0x3C: wtypes.KeyRightShift,
	0x3D: wtypes.KeyRightAlt,
	0x3E: wtypes.KeyRightControl,
	0x47: wtypes.KeyNumLock,
	0x60: wtypes.KeyF5,
	0x61: wtypes.KeyF6,
	0x62: wtypes.KeyF7,
	0x63: wtypes.KeyF3,
	0x64: wtypes.KeyF8,
	0x65: wtypes.KeyF9,
	0x67: wtypes.KeyF11,
	0x69: wtypes.KeyPrintScreen,
	0x6D: wtypes.KeyF10,
	0x6F: wtypes.KeyF12,
	0x72: wtypes.KeyInsert,
	0x73: wtypes.KeyHome,
	0x74: wtypes.KeyPageUp,
	0x75: wtypes.KeyDelete,
	0x76: wtypes.KeyF4,
	0x77: wtypes.KeyEnd,
	0x78: wtypes.KeyF2,
	0x79: wtypes.KeyPageDown,
	0x7A: wtypes.KeyF1,
	0x7B: wtypes.KeyLeft,
	0x7C: wtypes.KeyRight,
	0x7D: wtypes.KeyDown,
	0x7E: wtypes.KeyUp,
}

func macKeyToKey(vkCode uint64) wtypes.Key {
	if vkCode < 256 {
		return vkToKey[vkCode]
	}
	return wtypes.KeyUnknown
}

func modFlagsToMod(flags uint64) wtypes.Mod {
	var m wtypes.Mod
	if flags&nsEventModifierFlagShift != 0 {
		m |= wtypes.ModShift
	}
	if flags&nsEventModifierFlagControl != 0 {
		m |= wtypes.ModControl
	}
	if flags&nsEventModifierFlagOption != 0 {
		m |= wtypes.ModAlt
	}
	if flags&nsEventModifierFlagCommand != 0 {
		m |= wtypes.ModSuper
	}
	if flags&nsEventModifierFlagCapsLock != 0 {
		m |= wtypes.ModCapsLock
	}
	return m
}

// ---------------------------------------------------------------------------
// window
// ---------------------------------------------------------------------------

type window struct {
	nswin    unsafe.Pointer
	delegate unsafe.Pointer
	view     unsafe.Pointer

	width, height uint32
	scale         float32
	mouseX, mouseY float64
	closed         atomic.Bool
	styleMask      uint64

	onResize      func(w, h uint32)
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
}

func New(title string, width, height, minW, minH uint32) (*window, error) {
	if err := ensureLoaded(); err != nil {
		return nil, err
	}
	initSels()
	initApp()
	initDelegateClass()

	runtime.LockOSThread()

	style := nsWindowStyleMaskTitled | nsWindowStyleMaskClosable |
		nsWindowStyleMaskMiniaturizable | nsWindowStyleMaskResizable

	nsCls := getClass("NSWindow")
	raw := msgSend0(nsCls, selAlloc)
	nswin := msgSendInitWindow(raw, selInitWithContentRect,
		0, 0, float64(width), float64(height),
		style, nsBackingStoreBuffered, 0)
	if nswin == nil {
		return nil, fmt.Errorf("quikwin/cocoa: NSWindow init failed")
	}

	msgSend1bVoid(nswin, selSetReleasedWhenClosed, 0)
	msgSend1pVoid(nswin, selSetTitle, nsString(title))

	if minW > 0 || minH > 0 {
		msgSend2fVoid(nswin, selSetMinSize, float64(minW), float64(minH))
	}

	view := msgSend0(nswin, selContentView)
	msgSend1bVoid(view, selSetWantsLayer, 1)

	screen := msgSend0(getClass("NSScreen"), selMainScreen)
	scale := float32(1.0)
	if screen != nil {
		scale = float32(msgSend0f(screen, selBackingScaleFactor))
	}

	w := &window{
		nswin:     nswin,
		view:      view,
		width:     width,
		height:    height,
		scale:     scale,
		styleMask: style,
	}

	del := msgSend0(msgSend0(delegateClass, selAlloc), selInit)
	registerDelegate(del, w)
	w.delegate = del
	msgSend1pVoid(nswin, selSetDelegate, del)

	msgSend1pVoid(nswin, selMakeKeyAndOrderFront, nil)
	return w, nil
}

func (w *window) Size() (uint32, uint32) { return w.width, w.height }
func (w *window) Scale() float32         { return w.scale }
func (w *window) ShouldClose() bool      { return w.closed.Load() }

func (w *window) PollEvents() {
	if w.closed.Load() {
		return
	}
	mode := nsString("kCFRunLoopDefaultMode")
	dateCls := getClass("NSDate")
	distantPast := msgSend0(dateCls, selDistantPast)
	mask := nsEventMaskAny
	var dequeue int32 = 1

	for {
		var event unsafe.Pointer
		ffi.CallFunction(&cifNextEvent, _objcMsgSend, unsafe.Pointer(&event),
			[]unsafe.Pointer{nsApp, selNextEventMatchingMask,
				unsafe.Pointer(&mask), distantPast, mode, unsafe.Pointer(&dequeue)})
		if event == nil {
			break
		}
		w.handleEvent(event)
		msgSend1pVoid(nsApp, selSendEvent, event)
	}
	msgSend0(nsApp, selUpdateWindows)
}

func (w *window) handleEvent(event unsafe.Pointer) {
	evType := msgSend0i(event, selType)
	flags := msgSend0u(event, selModifierFlags)
	mods := modFlagsToMod(flags)

	switch evType {
	case nsEventTypeKeyDown, nsEventTypeKeyUp:
		vkCode := uint64(msgSend0i(event, selKeyCode))
		key := macKeyToKey(vkCode)
		isRepeat := msgSend0i(event, selIsARepeat) != 0
		action := wtypes.Press
		if evType == nsEventTypeKeyUp {
			action = wtypes.Release
		} else if isRepeat {
			action = wtypes.Repeat
		}
		if fn := w.onKey; fn != nil && key != wtypes.KeyUnknown {
			fn(key, action, mods)
		}
		if evType == nsEventTypeKeyDown {
			if nsChars := msgSend0(event, selCharacters); nsChars != nil {
				if utf8ptr := msgSend0(nsChars, selUTF8String); utf8ptr != nil {
					for _, r := range goString(utf8ptr) {
						if fn := w.onChar; fn != nil {
							fn(r)
						}
					}
				}
			}
		}

	case nsEventTypeFlagsChanged:
		vkCode := uint64(msgSend0i(event, selKeyCode))
		key := macKeyToKey(vkCode)
		if key == wtypes.KeyUnknown {
			return
		}
		action := wtypes.Press
		switch key {
		case wtypes.KeyLeftShift, wtypes.KeyRightShift:
			if flags&nsEventModifierFlagShift == 0 {
				action = wtypes.Release
			}
		case wtypes.KeyLeftControl, wtypes.KeyRightControl:
			if flags&nsEventModifierFlagControl == 0 {
				action = wtypes.Release
			}
		case wtypes.KeyLeftAlt, wtypes.KeyRightAlt:
			if flags&nsEventModifierFlagOption == 0 {
				action = wtypes.Release
			}
		case wtypes.KeyCapsLock:
			if flags&nsEventModifierFlagCapsLock == 0 {
				action = wtypes.Release
			}
		}
		if fn := w.onKey; fn != nil {
			fn(key, action, mods)
		}

	case nsEventTypeLeftMouseDown, nsEventTypeRightMouseDown, nsEventTypeOtherMouseDown:
		if fn := w.onMouseButton; fn != nil {
			fn(mouseButton(evType, event), wtypes.Press, mods)
		}

	case nsEventTypeLeftMouseUp, nsEventTypeRightMouseUp, nsEventTypeOtherMouseUp:
		if fn := w.onMouseButton; fn != nil {
			fn(mouseButton(evType, event), wtypes.Release, mods)
		}

	case nsEventTypeMouseMoved,
		nsEventTypeLeftMouseDragged,
		nsEventTypeRightMouseDragged,
		nsEventTypeOtherMouseDragged:
		dx := msgSend0f(event, selDeltaX)
		dy := msgSend0f(event, selDeltaY)
		w.mouseX += dx
		w.mouseY += dy
		if w.mouseX < 0 {
			w.mouseX = 0
		}
		if w.mouseY < 0 {
			w.mouseY = 0
		}
		if fn := w.onMouseMove; fn != nil {
			fn(w.mouseX, w.mouseY)
		}

	case nsEventTypeScrollWheel:
		dx := msgSend0f(event, selScrollingDeltaX)
		dy := msgSend0f(event, selScrollingDeltaY)
		if fn := w.onScroll; fn != nil {
			fn(dx, dy)
		}
	}
}

func mouseButton(evType int64, event unsafe.Pointer) wtypes.Button {
	switch evType {
	case nsEventTypeLeftMouseDown, nsEventTypeLeftMouseUp, nsEventTypeLeftMouseDragged:
		return wtypes.ButtonLeft
	case nsEventTypeRightMouseDown, nsEventTypeRightMouseUp, nsEventTypeRightMouseDragged:
		return wtypes.ButtonRight
	default:
		switch msgSend0i(event, selButtonNumber) {
		case 2:
			return wtypes.ButtonMiddle
		case 3:
			return wtypes.Button4
		case 4:
			return wtypes.Button5
		default:
			return wtypes.ButtonMiddle
		}
	}
}

func goString(p unsafe.Pointer) string {
	if p == nil {
		return ""
	}
	b := (*[1 << 20]byte)(p)
	n := 0
	for b[n] != 0 {
		n++
	}
	return string(b[:n])
}

// ---------------------------------------------------------------------------
// window.Window interface
// ---------------------------------------------------------------------------

func (w *window) SetTitle(title string) {
	msgSend1pVoid(w.nswin, selSetTitle, nsString(title))
}

func (w *window) SetMinSize(mw, mh uint32) {
	msgSend2fVoid(w.nswin, selSetMinSize, float64(mw), float64(mh))
}

func (w *window) SetSize(sw, sh uint32) {
	msgSend2fVoid(w.nswin, selSetContentSize, float64(sw), float64(sh))
	w.width = sw
	w.height = sh
}

func (w *window) SetCursor(_ wtypes.CursorShape) {}

func (w *window) BeginDrag() {}

func (w *window) Destroy() {
	if w.closed.Swap(true) {
		return
	}
	unregisterDelegate(w.delegate)
	msgSend0(w.nswin, selClose)
	msgSend0(w.nswin, selRelease)
	w.nswin = nil
}

func (w *window) OnResize(fn func(uint32, uint32))                            { w.onResize = fn }
func (w *window) OnLiveResize(fn func())                                      { w.onLiveResize = fn }
func (w *window) OnClose(fn func())                                           { w.onClose = fn }
func (w *window) OnFocus(fn func(bool))                                       { w.onFocus = fn }
func (w *window) OnKey(fn func(wtypes.Key, wtypes.Action, wtypes.Mod))        { w.onKey = fn }
func (w *window) OnChar(fn func(rune))                                        { w.onChar = fn }
func (w *window) OnMouseButton(fn func(wtypes.Button, wtypes.Action, wtypes.Mod)) { w.onMouseButton = fn }
func (w *window) OnMouseMove(fn func(float64, float64))                       { w.onMouseMove = fn }
func (w *window) OnScroll(fn func(float64, float64))                          { w.onScroll = fn }
func (w *window) OnDragBegin(fn func(float64, float64))                       { w.onDragBegin = fn }
func (w *window) OnDragMove(fn func(float64, float64))                        { w.onDragMove = fn }
func (w *window) OnDragEnd(fn func(float64, float64))                         { w.onDragEnd = fn }
func (w *window) OnDrop(fn func([]string))                                    { w.onDrop = fn }

// ---------------------------------------------------------------------------
// CocoaWindow interface methods
// ---------------------------------------------------------------------------

type TitlebarStyle = uint8

const (
	TitlebarDefault     TitlebarStyle = 0
	TitlebarHidden      TitlebarStyle = 1
	TitlebarTransparent TitlebarStyle = 2
)

func (w *window) SetTitlebarStyle(style TitlebarStyle) {
	switch style {
	case TitlebarHidden:
		newStyle := w.styleMask | nsWindowStyleMaskFullSizeContentView
		msgSend1iVoid(w.nswin, selSetStyleMask, int64(newStyle))
		msgSend1bVoid(w.nswin, selSetTitlebarAppearsTransparent, 1)
		msgSend1iVoid(w.nswin, selSetTitleVisibility, 1) // NSWindowTitleHidden = 1
	case TitlebarTransparent:
		newStyle := w.styleMask | nsWindowStyleMaskFullSizeContentView
		msgSend1iVoid(w.nswin, selSetStyleMask, int64(newStyle))
		msgSend1bVoid(w.nswin, selSetTitlebarAppearsTransparent, 1)
	default:
		msgSend1bVoid(w.nswin, selSetTitlebarAppearsTransparent, 0)
		msgSend1iVoid(w.nswin, selSetTitleVisibility, 0)
	}
}

func (w *window) SetTitleVisible(visible bool) {
	v := int64(0) // NSWindowTitleVisible = 0
	if !visible {
		v = 1 // NSWindowTitleHidden = 1
	}
	msgSend1iVoid(w.nswin, selSetTitleVisibility, v)
}

func (w *window) SetTrafficLightsOffset(x, y float32) {
	for _, btn := range []int64{nsWindowCloseButton, nsWindowMiniaturizeButton, nsWindowZoomButton} {
		b := msgSend1p(w.nswin, selStandardWindowButton, unsafe.Pointer(&btn))
		if b == nil {
			continue
		}
		fx := float64(x) + float64(btn)*20
		fy := float64(y)
		msgSend2fVoid(b, selSetFrameOrigin, fx, fy)
	}
}

func (w *window) SetMenuBar(_ []any) {}

// ---------------------------------------------------------------------------
// Vulkan surface
// ---------------------------------------------------------------------------

func (w *window) NewSurface(instance vk.Instance) (*vk.SurfaceKHR, error) {
	layer := msgSend0(w.view, selLayer)
	if layer == nil {
		return nil, fmt.Errorf("quikwin/cocoa: no CAMetalLayer on contentView")
	}
	info := vk.MacOSSurfaceCreateInfoMVK{
		View: layer,
	}
	return instance.CreateMacOSSurfaceMVK(&info, nil)
}
