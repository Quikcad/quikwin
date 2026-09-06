//go:build darwin

package cocoa

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/Quikcad/quikwin/internal/platform/cocoa/objc"
	"github.com/Quikcad/quikwin/internal/platform/wtypes"
	vk "github.com/lukem570/vulkan-go/pkg/raw"
)

// init pins the main goroutine to the OS thread that started the runtime.
// On darwin that's the process's main thread (thread 0) — the only thread
// AppKit will accept NSWindow/NSApplication calls on. Doing this from a later
// hook (e.g. inside New) is too late: by then Go may have migrated the main
// goroutine onto a worker thread and AppKit raises
// NSInternalInconsistencyException.
func init() {
	runtime.LockOSThread()
}

// ---------------------------------------------------------------------------
// Selectors (registered once on first use)
// ---------------------------------------------------------------------------

var (
	selInit                          unsafe.Pointer
	selAlloc                         unsafe.Pointer
	selRelease                       unsafe.Pointer
	selSharedApplication             unsafe.Pointer
	selSetActivationPolicy           unsafe.Pointer
	selActivateIgnoringOtherApps     unsafe.Pointer
	selNextEventMatchingMask         unsafe.Pointer
	selSendEvent                     unsafe.Pointer
	selUpdateWindows                 unsafe.Pointer
	selSetDelegate                   unsafe.Pointer
	selDistantPast                   unsafe.Pointer
	selDistantFuture                 unsafe.Pointer
	selDateWithTimeIntervalSinceNow  unsafe.Pointer
	selRetain                        unsafe.Pointer
	selPostEvent                     unsafe.Pointer
	selOtherEventWithType            unsafe.Pointer
	selStringWithUTF8String          unsafe.Pointer
	selInitWithContentRect           unsafe.Pointer
	selMakeKeyAndOrderFront          unsafe.Pointer
	selSetTitle                      unsafe.Pointer
	selSetReleasedWhenClosed         unsafe.Pointer
	selSetMinSize                    unsafe.Pointer
	selSetContentSize                unsafe.Pointer
	selSetStyleMask                  unsafe.Pointer
	selClose                         unsafe.Pointer
	selContentView                   unsafe.Pointer
	selSetWantsLayer                 unsafe.Pointer
	selLayer                         unsafe.Pointer
	selBackingScaleFactor            unsafe.Pointer
	selMainScreen                    unsafe.Pointer
	selType                          unsafe.Pointer
	selKeyCode                       unsafe.Pointer
	selModifierFlags                 unsafe.Pointer
	selIsARepeat                     unsafe.Pointer
	selCharacters                    unsafe.Pointer
	selUTF8String                    unsafe.Pointer
	selButtonNumber                  unsafe.Pointer
	selDeltaX                        unsafe.Pointer
	selDeltaY                        unsafe.Pointer
	selScrollingDeltaX               unsafe.Pointer
	selScrollingDeltaY               unsafe.Pointer
	selSetTitlebarAppearsTransparent unsafe.Pointer
	selSetTitleVisibility            unsafe.Pointer
	selStandardWindowButton          unsafe.Pointer
	selSetFrameOrigin                unsafe.Pointer
	selSetLayer                      unsafe.Pointer
	selSetContentsScale              unsafe.Pointer
	selHide                          unsafe.Pointer
	selUnhide                        unsafe.Pointer
	selPerformWindowDragWithEvent    unsafe.Pointer
	selCurrentEvent                  unsafe.Pointer
	selLocationInWindow              unsafe.Pointer
	selSetAcceptsMouseMovedEvents    unsafe.Pointer
	selFinishLaunching               unsafe.Pointer
	selIsKeyWindow                   unsafe.Pointer
	selFrame                         unsafe.Pointer
	selMiniaturize                   unsafe.Pointer
	selZoom                          unsafe.Pointer
	selIsZoomed                      unsafe.Pointer
	selToggleFullScreen              unsafe.Pointer
	selStyleMask                     unsafe.Pointer
	selSetCollectionBehavior         unsafe.Pointer
	selSetOpaque                     unsafe.Pointer
	selSetBackgroundColor            unsafe.Pointer
	selClearColor                    unsafe.Pointer
	selSetCornerRadius               unsafe.Pointer
	selSetMasksToBounds              unsafe.Pointer
	selCenter                        unsafe.Pointer
	selHasPreciseScrollingDeltas     unsafe.Pointer
	selMagnification                 unsafe.Pointer

	selsOnce sync.Once
)

func initSels() {
	selsOnce.Do(func() {
		reg := objc.SelRegister
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
		selDistantFuture = reg("distantFuture")
		selDateWithTimeIntervalSinceNow = reg("dateWithTimeIntervalSinceNow:")
		selRetain = reg("retain")
		selPostEvent = reg("postEvent:atStart:")
		selOtherEventWithType = reg("otherEventWithType:location:modifierFlags:timestamp:windowNumber:context:subtype:data1:data2:")
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
		selSetLayer = reg("setLayer:")
		selSetContentsScale = reg("setContentsScale:")
		selHide = reg("hide")
		selUnhide = reg("unhide")
		selPerformWindowDragWithEvent = reg("performWindowDragWithEvent:")
		selCurrentEvent = reg("currentEvent")
		selLocationInWindow = reg("locationInWindow")
		selSetAcceptsMouseMovedEvents = reg("setAcceptsMouseMovedEvents:")
		selFinishLaunching = reg("finishLaunching")
		selIsKeyWindow = reg("isKeyWindow")
		selFrame = reg("frame")
		selMiniaturize = reg("miniaturize:")
		selZoom = reg("zoom:")
		selIsZoomed = reg("isZoomed")
		selToggleFullScreen = reg("toggleFullScreen:")
		selStyleMask = reg("styleMask")
		selSetCollectionBehavior = reg("setCollectionBehavior:")
		selSetOpaque = reg("setOpaque:")
		selSetBackgroundColor = reg("setBackgroundColor:")
		selClearColor = reg("clearColor")
		selSetCornerRadius = reg("setCornerRadius:")
		selSetMasksToBounds = reg("setMasksToBounds:")
		selCenter = reg("center")
		selHasPreciseScrollingDeltas = reg("hasPreciseScrollingDeltas")
		selMagnification = reg("magnification")
	})
}

func nsString(s string) unsafe.Pointer {
	p := objc.Bstr(s)
	cls := objc.GetClass("NSString")
	return objc.MsgSend1p(cls, selStringWithUTF8String, unsafe.Pointer(p))
}

// ---------------------------------------------------------------------------
// NSApplication setup (once per process)
// ---------------------------------------------------------------------------

var (
	nsApp     unsafe.Pointer
	nsAppOnce sync.Once
)

// Objects the event loop needs on every pass. Resolving them per call meant an
// NSString allocation and two runtime lookups for each poll, which a caller
// running at a few hundred hertz pays for. Each is retained: they outlive any
// autorelease pool the loop pushes.
var (
	nsEventClass    unsafe.Pointer
	nsDateClass     unsafe.Pointer
	nsDefaultMode   unsafe.Pointer // @"kCFRunLoopDefaultMode"
	nsDistantPast   unsafe.Pointer
	nsDistantFuture unsafe.Pointer
)

func initApp() {
	nsAppOnce.Do(func() {
		nsEventClass = objc.GetClass("NSEvent")
		nsDateClass = objc.GetClass("NSDate")
		retain := func(p unsafe.Pointer) unsafe.Pointer { return objc.MsgSend0(p, selRetain) }
		nsDefaultMode = retain(nsString("kCFRunLoopDefaultMode"))
		nsDistantPast = retain(objc.MsgSend0(nsDateClass, selDistantPast))
		nsDistantFuture = retain(objc.MsgSend0(nsDateClass, selDistantFuture))

		cls := objc.GetClass("NSApplication")
		nsApp = objc.MsgSend0(cls, selSharedApplication)
		objc.MsgSend1iVoid(nsApp, selSetActivationPolicy, 0) // NSApplicationActivationPolicyRegular = 0
		// finishLaunching does what -[NSApplication run] would do up to the
		// first runloop iteration: posts NSApplicationDidFinishLaunching, wires
		// up the delegate auto-observer machinery (windowDidBecomeKey: etc.)
		// and prepares the app for event delivery. Skipping it leaves AppKit
		// in a partially-launched state where window-key notifications never
		// reach delegate methods.
		objc.MsgSend0(nsApp, selFinishLaunching)
		objc.MsgSend1bVoid(nsApp, selActivateIgnoringOtherApps, 1)
	})
}

// ---------------------------------------------------------------------------
// NSWindow subclass
// ---------------------------------------------------------------------------

// quikwinWindowClass is an NSWindow subclass that forces canBecomeKeyWindow
// and canBecomeMainWindow to YES. NSWindow's defaults return NO for
// borderless windows (no NSWindowStyleMaskTitled), so a custom-chrome window
// would otherwise never become key — meaning no windowDidBecomeKey:/
// windowDidResignKey: notifications and no keyboard focus.
var (
	quikwinWindowClass     unsafe.Pointer
	quikwinWindowClassOnce sync.Once
)

func initWindowClass() {
	quikwinWindowClassOnce.Do(func() {
		super := objc.GetClass("NSWindow")
		quikwinWindowClass = objc.AllocateClassPair(super, "QuikwinWindow")
		// BOOL methods: encoding "c@:" = signed char return, id self, SEL _cmd.
		objc.AddMethod(quikwinWindowClass, "canBecomeKeyWindow", "c@:",
			func(_self, _cmd uintptr) bool { return true })
		objc.AddMethod(quikwinWindowClass, "canBecomeMainWindow", "c@:",
			func(_self, _cmd uintptr) bool { return true })
		objc.RegisterClassPair(quikwinWindowClass)
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
		superCls := objc.GetClass("NSObject")
		delegateClass = objc.AllocateClassPair(superCls, "QuikwinWindowDelegate")

		addMethod("windowWillClose:", "v@:@", func(self, _cmd, notif uintptr) {
			if w := lookupDelegate(unsafe.Pointer(self)); w != nil {
				w.closed.Store(true)
				if fn := w.onClose; fn != nil {
					fn()
				}
			}
		})
		addMethod("windowDidResize:", "v@:@", func(self, _cmd, notif uintptr) {
			w := lookupDelegate(unsafe.Pointer(self))
			if w == nil {
				return
			}
			view := objc.MsgSend0(w.nswin, selContentView)
			frame := objc.MsgSend0Rect(view, selFrame)
			w.width = uint32(frame.W)
			w.height = uint32(frame.H)
			if fn := w.onResize; fn != nil {
				fn(w.width, w.height)
			}
			if fn := w.onLiveResize; fn != nil {
				fn()
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

		objc.RegisterClassPair(delegateClass)
	})
}

func addMethod(selName, typeStr string, fn any) {
	objc.AddMethod(delegateClass, selName, typeStr, fn)
}

// ---------------------------------------------------------------------------
// NSEvent type constants
// ---------------------------------------------------------------------------

const (
	nsEventTypeLeftMouseDown      int64 = 1
	nsEventTypeLeftMouseUp        int64 = 2
	nsEventTypeRightMouseDown     int64 = 3
	nsEventTypeRightMouseUp       int64 = 4
	nsEventTypeMouseMoved         int64 = 5
	nsEventTypeLeftMouseDragged   int64 = 6
	nsEventTypeRightMouseDragged  int64 = 7
	nsEventTypeKeyDown            int64 = 10
	nsEventTypeKeyUp              int64 = 11
	nsEventTypeFlagsChanged       int64 = 12
	nsEventTypeScrollWheel        int64 = 22
	nsEventTypeOtherMouseDown     int64 = 25
	nsEventTypeOtherMouseUp       int64 = 26
	nsEventTypeOtherMouseDragged  int64 = 27
	nsEventTypeMagnify            int64 = 30
	nsEventTypeApplicationDefined int64 = 15
)

const nsEventMaskAny uint64 = ^uint64(0) // NSEventMaskAny

// NSWindow style masks
const (
	nsWindowStyleMaskTitled              uint64 = 1 << 0
	nsWindowStyleMaskClosable            uint64 = 1 << 1
	nsWindowStyleMaskMiniaturizable      uint64 = 1 << 2
	nsWindowStyleMaskResizable           uint64 = 1 << 3
	nsWindowStyleMaskFullSizeContentView uint64 = 1 << 15
	nsWindowStyleMaskFullScreen          uint64 = 1 << 14

	nsWindowCollectionBehaviorFullScreenPrimary uint64 = 1 << 7
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

var _ wtypes.Window = (*window)(nil)

type window struct {
	nswin      unsafe.Pointer
	delegate   unsafe.Pointer
	view       unsafe.Pointer
	metalLayer unsafe.Pointer // CAMetalLayer backing the view; set in New

	width, height  uint32
	scale          float32
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
	onScroll      func(float64, float64, bool)
	onPinch       func(float64)
	onDragBegin   func(float64, float64)
	onDragMove    func(float64, float64)
	onDragEnd     func(float64, float64)
	onDrop        func([]string)
	onHitTest     func(float64, float64) wtypes.HitTestResult
	onCursorEnter func(float64, float64)

	cursorHidden  bool
	mouseInWindow bool

	clipText string // in-process clipboard fallback (see clipboard.go)
}

func New(cfg *wtypes.Config) (*window, error) {
	if err := objc.Load(); err != nil {
		return nil, err
	}
	initSels()
	initApp()
	initWindowClass()
	initDelegateClass()

	width, height := cfg.Width, cfg.Height
	minW, minH := cfg.MinWidth, cfg.MinHeight

	style := nsWindowStyleMaskClosable | nsWindowStyleMaskMiniaturizable
	if cfg.Titlebar {
		style |= nsWindowStyleMaskTitled
	}
	if cfg.Resizable {
		style |= nsWindowStyleMaskResizable
	}

	raw := objc.MsgSend0(quikwinWindowClass, selAlloc)
	nswin := objc.MsgSendInitWindow(raw, selInitWithContentRect,
		0, 0, float64(width), float64(height),
		style, nsBackingStoreBuffered, 0)
	if nswin == nil {
		return nil, fmt.Errorf("quikwin/cocoa: NSWindow init failed")
	}

	objc.MsgSend1bVoid(nswin, selSetReleasedWhenClosed, 0)
	objc.MsgSend1bVoid(nswin, selSetAcceptsMouseMovedEvents, 1)
	// Enable native macOS full-screen (green button enters a new Space rather
	// than just zooming within the current desktop).
	objc.MsgSend1iVoid(nswin, selSetCollectionBehavior, int64(nsWindowCollectionBehaviorFullScreenPrimary))
	objc.MsgSend1pVoid(nswin, selSetTitle, nsString(cfg.Title))

	if minW > 0 || minH > 0 {
		objc.MsgSend2fVoid(nswin, selSetMinSize, float64(minW), float64(minH))
	}

	view := objc.MsgSend0(nswin, selContentView)
	// Create a CAMetalLayer and attach it as the view's backing layer before
	// calling setWantsLayer:YES. This gives MoltenVK a concrete Metal layer to
	// use when creating a VkSurfaceKHR via VK_EXT_metal_surface. If we only
	// call setWantsLayer:YES the view gets a plain CALayer which MoltenVK
	// cannot use and vkGetPhysicalDeviceSurfaceCapabilitiesKHR returns
	// VK_ERROR_SURFACE_LOST_KHR.
	metalLayerCls := objc.GetClass("CAMetalLayer")
	metalLayer := objc.MsgSend0(metalLayerCls, selLayer) // [CAMetalLayer layer]
	if metalLayer == nil {
		// Fallback: alloc+init if the class method is unavailable.
		metalLayer = objc.MsgSend0(objc.MsgSend0(metalLayerCls, selAlloc), selInit)
	}
	if metalLayer != nil {
		objc.MsgSend1pVoid(view, selSetLayer, metalLayer) // [view setLayer:metalLayer]
	}
	objc.MsgSend1bVoid(view, selSetWantsLayer, 1)

	screen := objc.MsgSend0(objc.GetClass("NSScreen"), selMainScreen)
	scale := float32(1.0)
	if screen != nil {
		scale = float32(objc.MsgSend0f(screen, selBackingScaleFactor))
	}
	// Set contentsScale on the Metal layer so the drawable size matches
	// physical pixels. Without this it defaults to 1.0 and every drawable
	// pixel maps to backingScaleFactor display pixels, making all content
	// look scale-factor× too large on Retina displays.
	if metalLayer != nil && scale > 0 {
		objc.MsgSend1fVoid(metalLayer, selSetContentsScale, float64(scale))
	}

	w := &window{
		nswin:      nswin,
		view:       view,
		metalLayer: metalLayer,
		width:      width,
		height:     height,
		scale:      scale,
		styleMask:  style,
	}

	del := objc.MsgSend0(objc.MsgSend0(delegateClass, selAlloc), selInit)
	registerDelegate(del, w)
	w.delegate = del
	objc.MsgSend1pVoid(nswin, selSetDelegate, del)

	if cfg.Centered {
		// NSWindow's -center positions the window slightly above the visual
		// centre of the screen containing the cursor — Cocoa's idiomatic
		// "centred" placement for newly-opened windows.
		objc.MsgSend0(nswin, selCenter)
	}
	objc.MsgSend1pVoid(nswin, selMakeKeyAndOrderFront, nil)
	return w, nil
}

func (w *window) Size() (uint32, uint32) { return w.width, w.height }
func (w *window) Scale() float32         { return w.scale }
func (w *window) ShouldClose() bool      { return w.closed.Load() }

func (w *window) PollEvents() { w.dispatch(0) }

func (w *window) WaitEvents() { w.dispatch(-1) }

func (w *window) WaitEventsTimeout(d time.Duration) {
	if d <= 0 {
		w.dispatch(0)
		return
	}
	w.dispatch(d)
}

// Post wakes a blocked wait with an application-defined event. AppKit retains
// a posted event, so the pool this pushes can take the autoreleased original
// back at once — and the pool is needed because Post runs off the UI goroutine,
// where nothing else drains one.
func (w *window) Post() {
	if w.closed.Load() {
		return
	}
	pool := objc.PoolPush()
	defer objc.PoolPop(pool)

	event := objc.OtherEvent(nsEventClass, selOtherEventWithType,
		uint64(nsEventTypeApplicationDefined), 0, 0, 0, 0, 0, nil, 0, 0, 0)
	if event == nil {
		return
	}
	objc.MsgSend1p1bVoid(nsApp, selPostEvent, event, 1)
}

// dispatch drains the AppKit queue. A negative timeout blocks until an event or
// a Post arrives, zero returns at once, and a positive one blocks for at most
// that long.
func (w *window) dispatch(timeout time.Duration) {
	if w.closed.Load() {
		return
	}
	// AppKit autoreleases freely while events are handled, and this loop is the
	// only thing resembling a run-loop iteration, so it owns the pool.
	pool := objc.PoolPush()
	defer objc.PoolPop(pool)

	until := nsDistantPast
	switch {
	case timeout < 0:
		until = nsDistantFuture
	case timeout > 0:
		until = objc.MsgSend1f(nsDateClass, selDateWithTimeIntervalSinceNow, timeout.Seconds())
	}

	handled := false
	for {
		event := objc.NextEvent(nsApp, selNextEventMatchingMask, nsEventMaskAny, until, nsDefaultMode, 1)
		if event == nil {
			break
		}
		w.handleEvent(event)
		objc.MsgSend1pVoid(nsApp, selSendEvent, event)
		handled = true
		// Only the first pass may block; the rest take what is already queued.
		until = nsDistantPast
	}
	// updateWindows can run layout and display work, so it stays off the passes
	// that saw no event at all.
	if handled {
		objc.MsgSend0(nsApp, selUpdateWindows)
	}
}

func (w *window) handleEvent(event unsafe.Pointer) {
	evType := objc.MsgSend0i(event, selType)
	flags := objc.MsgSend0u(event, selModifierFlags)
	mods := modFlagsToMod(flags)

	switch evType {
	case nsEventTypeKeyDown, nsEventTypeKeyUp:
		vkCode := uint64(objc.MsgSend0i(event, selKeyCode))
		key := macKeyToKey(vkCode)
		isRepeat := objc.MsgSend0i(event, selIsARepeat) != 0
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
			if nsChars := objc.MsgSend0(event, selCharacters); nsChars != nil {
				if utf8ptr := objc.MsgSend0(nsChars, selUTF8String); utf8ptr != nil {
					for _, r := range goString(utf8ptr) {
						if fn := w.onChar; fn != nil {
							fn(r)
						}
					}
				}
			}
		}

	case nsEventTypeFlagsChanged:
		vkCode := uint64(objc.MsgSend0i(event, selKeyCode))
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
		if evType == nsEventTypeLeftMouseDown {
			if fn := w.onHitTest; fn != nil {
				if fn(w.mouseX, w.mouseY) == wtypes.HitTestDrag {
					objc.MsgSend1pVoid(w.nswin, selPerformWindowDragWithEvent, event)
					return
				}
			}
		}
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
		pt := objc.MsgSend0Point(event, selLocationInWindow)
		// Cocoa window coords: origin bottom-left, in points. Flip Y to match
		// the top-left convention exposed by Window.OnMouseMove.
		w.mouseX = pt.X
		w.mouseY = float64(w.height) - pt.Y
		inside := w.mouseX >= 0 && w.mouseX < float64(w.width) &&
			w.mouseY >= 0 && w.mouseY < float64(w.height)
		if inside && !w.mouseInWindow {
			w.mouseInWindow = true
			if fn := w.onCursorEnter; fn != nil {
				fn(w.mouseX, w.mouseY)
			}
		} else if !inside {
			w.mouseInWindow = false
		}
		// Cocoa keeps delivering mouseMoved/dragged events for points outside
		// the content rect (titlebar, window shadow, drag-out). Only forward
		// when the cursor is actually inside the content area.
		if inside {
			if fn := w.onMouseMove; fn != nil {
				fn(w.mouseX, w.mouseY)
			}
		}

	case nsEventTypeScrollWheel:
		dx := objc.MsgSend0f(event, selScrollingDeltaX)
		dy := objc.MsgSend0f(event, selScrollingDeltaY)
		// BOOL is signed char in ObjC; truncate to low byte and compare.
		precise := int8(objc.MsgSend0i(event, selHasPreciseScrollingDeltas)) != 0
		if fn := w.onScroll; fn != nil {
			fn(dx, dy, precise)
		}

	case nsEventTypeMagnify:
		mag := objc.MsgSend0f(event, selMagnification)
		if fn := w.onPinch; fn != nil {
			fn(mag)
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
		switch objc.MsgSend0i(event, selButtonNumber) {
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
	objc.MsgSend1pVoid(w.nswin, selSetTitle, nsString(title))
}

func (w *window) SetMinSize(mw, mh uint32) {
	objc.MsgSend2fVoid(w.nswin, selSetMinSize, float64(mw), float64(mh))
}

func (w *window) SetSize(sw, sh uint32) {
	objc.MsgSend2fVoid(w.nswin, selSetContentSize, float64(sw), float64(sh))
	w.width = sw
	w.height = sh
}

func (w *window) SetCursor(_ wtypes.CursorShape) {}

func (w *window) HideCursor() {
	if w.cursorHidden {
		return
	}
	w.cursorHidden = true
	cls := objc.GetClass("NSCursor")
	objc.MsgSend0(cls, selHide)
}

func (w *window) ShowCursor() {
	if !w.cursorHidden {
		return
	}
	w.cursorHidden = false
	cls := objc.GetClass("NSCursor")
	objc.MsgSend0(cls, selUnhide)
}

func (w *window) BeginDrag() {
	event := objc.MsgSend0(nsApp, selCurrentEvent)
	if event != nil {
		objc.MsgSend1pVoid(w.nswin, selPerformWindowDragWithEvent, event)
	}
}

func (w *window) Destroy() {
	if w.closed.Swap(true) {
		return
	}
	unregisterDelegate(w.delegate)
	objc.MsgSend0(w.nswin, selClose)
	objc.MsgSend0(w.nswin, selRelease)
	w.nswin = nil
}

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

// ---------------------------------------------------------------------------
// CocoaWindow interface methods
// ---------------------------------------------------------------------------

func (w *window) SetTitlebarStyle(style TitlebarStyle) {
	switch style {
	case TitlebarHidden:
		newStyle := w.styleMask | nsWindowStyleMaskFullSizeContentView
		objc.MsgSend1iVoid(w.nswin, selSetStyleMask, int64(newStyle))
		objc.MsgSend1bVoid(w.nswin, selSetTitlebarAppearsTransparent, 1)
		objc.MsgSend1iVoid(w.nswin, selSetTitleVisibility, 1) // NSWindowTitleHidden = 1
	case TitlebarTransparent:
		newStyle := w.styleMask | nsWindowStyleMaskFullSizeContentView
		objc.MsgSend1iVoid(w.nswin, selSetStyleMask, int64(newStyle))
		objc.MsgSend1bVoid(w.nswin, selSetTitlebarAppearsTransparent, 1)
	default:
		objc.MsgSend1bVoid(w.nswin, selSetTitlebarAppearsTransparent, 0)
		objc.MsgSend1iVoid(w.nswin, selSetTitleVisibility, 0)
	}
}

func (w *window) SetTitleVisible(visible bool) {
	v := int64(0) // NSWindowTitleVisible = 0
	if !visible {
		v = 1 // NSWindowTitleHidden = 1
	}
	objc.MsgSend1iVoid(w.nswin, selSetTitleVisibility, v)
}

func (w *window) SetTrafficLightsOffset(x, y float32) {
	for _, btn := range []int64{nsWindowCloseButton, nsWindowMiniaturizeButton, nsWindowZoomButton} {
		b := objc.MsgSend1p(w.nswin, selStandardWindowButton, unsafe.Pointer(&btn))
		if b == nil {
			continue
		}
		fx := float64(x) + float64(btn)*20
		fy := float64(y)
		objc.MsgSend2fVoid(b, selSetFrameOrigin, fx, fy)
	}
}

// Minimize sends -[NSWindow miniaturize:] (sender unused).
func (w *window) Minimize() {
	objc.MsgSend1pVoid(w.nswin, selMiniaturize, nil)
}

// ToggleMaximize toggles native macOS full screen, matching the green
// traffic-light button: the window animates into its own Space rather than
// just zooming within the current desktop.
func (w *window) ToggleMaximize() {
	objc.MsgSend1pVoid(w.nswin, selToggleFullScreen, nil)
}

// IsMaximized reports whether the window is currently in full-screen mode
// (NSWindowStyleMaskFullScreen present in the live style mask).
func (w *window) IsMaximized() bool {
	return objc.MsgSend0u(w.nswin, selStyleMask)&nsWindowStyleMaskFullScreen != 0
}

// SetCornerRadius applies a corner radius to the content view's backing layer
// (the CAMetalLayer) and switches the window to a non-opaque/clear background
// so the rounded corners aren't hidden by the system background fill. A radius
// of 0 reverts to square corners.
func (w *window) SetCornerRadius(r float64) {
	if w.metalLayer == nil {
		return
	}
	clear := objc.MsgSend0(objc.GetClass("NSColor"), selClearColor)
	objc.MsgSend1bVoid(w.nswin, selSetOpaque, 0)
	objc.MsgSend1pVoid(w.nswin, selSetBackgroundColor, clear)
	objc.MsgSend1fVoid(w.metalLayer, selSetCornerRadius, r)
	objc.MsgSend1bVoid(w.metalLayer, selSetMasksToBounds, 1)
}

// ---------------------------------------------------------------------------
// Vulkan surface
// ---------------------------------------------------------------------------

func (w *window) NewSurface(instance vk.Instance) (*vk.SurfaceKHR, error) {
	// Prefer VK_EXT_metal_surface (CreateMetalSurfaceEXT) over the deprecated
	// VK_MVK_macos_surface. The EXT variant takes a CAMetalLayer* directly,
	// which we set up in New(); newer MoltenVK versions return
	// ErrorSurfaceLostKHR from GetSurfaceCapabilitiesKHR when given a surface
	// created via the legacy MVK extension.
	if w.metalLayer != nil {
		info := vk.MetalSurfaceCreateInfoEXT{
			Layer: w.metalLayer,
		}
		return instance.CreateMetalSurfaceEXT(&info, nil)
	}
	// Fallback for the rare case where CAMetalLayer setup failed.
	info := vk.MacOSSurfaceCreateInfoMVK{
		View: w.view,
	}
	return instance.CreateMacOSSurfaceMVK(&info, nil)
}
