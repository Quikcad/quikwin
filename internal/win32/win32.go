//go:build windows

package win32

import (
	"fmt"
	"runtime"
	"sync"
	"unicode/utf16"
	"unsafe"

	"github.com/Quikcad/quikwin/internal/wtypes"
	"github.com/go-webgpu/goffi/ffi"
	"github.com/go-webgpu/goffi/types"
	vk "github.com/lukem570/vulkan-go/pkg/raw"
)

// MSG structure (on 64-bit Windows)
type winMsg [48]byte

// RECT structure
type winRect struct {
	Left, Top, Right, Bottom int32
}

// WNDCLASSEXW structure (80 bytes on 64-bit)
type wndClassExW struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     unsafe.Pointer
	hIcon         unsafe.Pointer
	hCursor       unsafe.Pointer
	hbrBackground unsafe.Pointer
	lpszMenuName  unsafe.Pointer
	lpszClassName unsafe.Pointer
	hIconSm       unsafe.Pointer
}

const (
	// Window styles
	wsOverlappedWindow = uint32(0x00CF0000)
	wsVisible          = uint32(0x10000000)

	// ShowWindow commands
	swShow = int32(5)

	// PeekMessage remove flag
	pmRemove = uint32(0x0001)

	// WM_* message constants
	wmDestroy      = uint32(0x0002)
	wmSize         = uint32(0x0005)
	wmSetFocus     = uint32(0x0007)
	wmKillFocus    = uint32(0x0008)
	wmClose        = uint32(0x0010)
	wmKeyDown      = uint32(0x0100)
	wmKeyUp        = uint32(0x0101)
	wmChar         = uint32(0x0102)
	wmSysKeyDown   = uint32(0x0104)
	wmSysKeyUp     = uint32(0x0105)
	wmMouseMove    = uint32(0x0200)
	wmLButtonDown  = uint32(0x0201)
	wmLButtonUp    = uint32(0x0202)
	wmRButtonDown  = uint32(0x0204)
	wmRButtonUp    = uint32(0x0205)
	wmMButtonDown  = uint32(0x0207)
	wmMButtonUp    = uint32(0x0208)
	wmMouseWheel   = uint32(0x020A)
	wmMouseHWheel  = uint32(0x020E)
	wmXButtonDown  = uint32(0x020B)
	wmXButtonUp    = uint32(0x020C)
	wmDropFiles    = uint32(0x0233)
	wmNcHitTest    = uint32(0x0084)
	wmSizing       = uint32(0x0214)
	wmMoving       = uint32(0x0216)
	wmEnterSizeMove = uint32(0x0231)
	wmExitSizeMove  = uint32(0x0232)
	wmNcCalcSize    = uint32(0x0083)

	// SetWindowPos flags
	swpNoZOrder     = uint32(0x0004)
	swpNoActivate   = uint32(0x0010)
	swpFrameChanged = uint32(0x0020)

	// Virtual key codes
	vkShift   = uint32(0x10)
	vkControl = uint32(0x11)
	vkMenu    = uint32(0x12) // Alt

	// DWM attribute for dark mode
	dwmwaUseImmersiveDarkMode = uint32(20)
	dwmwaMicaEffect           = uint32(1029)

	// GWLP_USERDATA
	gwlpUserdata = int32(-21)

	// GWL_EXSTYLE
	gwlExStyle = int32(-20)

	// WS_EX_ACCEPTFILES
	wsExAcceptFiles = uint32(0x00000010)
)

// Package-level window table indexed by HWND for WndProc dispatch.
var (
	wndProcMu sync.Mutex
	windows   = map[uintptr]*window{}
	wndProcCB uintptr // C callback created once
)

type window struct {
	mu   sync.Mutex
	hwnd unsafe.Pointer

	width, height       uint32
	minWidth, minHeight uint32
	darkMode            bool
	snapLayout          bool
	mica                bool

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

// New creates a Win32 window.
func New(title string, width, height, minWidth, minHeight uint32) (*window, error) {
	if err := ensureLoaded(); err != nil {
		return nil, err
	}

	// Register window class once.
	if err := ensureClassRegistered(); err != nil {
		return nil, err
	}

	hInst := getModuleHandle(nil)
	titleW := utf16Ptr(title)
	classW := utf16Ptr("QuikwinWindow")

	var pin runtime.Pinner
	pin.Pin(titleW)
	pin.Pin(classW)
	defer pin.Unpin()

	w := int32(width)
	h := int32(height)
	var hwnd unsafe.Pointer
	ffi.CallFunction(&cifCreateWindowExW, _CreateWindowExW, unsafe.Pointer(&hwnd),
		[]unsafe.Pointer{
			pU32(0),                        // exStyle
			unsafe.Pointer(classW),         // className
			unsafe.Pointer(titleW),         // title
			pU32(wsOverlappedWindow),        // style
			pI32(-0x80000000),              // x = CW_USEDEFAULT
			pI32(-0x80000000),              // y
			unsafe.Pointer(&w),
			unsafe.Pointer(&h),
			nil,                            // parent
			nil,                            // menu
			unsafe.Pointer(&hInst),
			nil,                            // param
		})
	if hwnd == nil {
		return nil, fmt.Errorf("quikwin/win32: CreateWindowExW failed")
	}

	win := &window{
		hwnd:      hwnd,
		width:     width,
		height:    height,
		minWidth:  minWidth,
		minHeight: minHeight,
	}

	wndProcMu.Lock()
	windows[uintptr(hwnd)] = win
	wndProcMu.Unlock()

	showCmd := swShow
	var showResult int32
	ffi.CallFunction(&cifShowWindow, _ShowWindow, unsafe.Pointer(&showResult),
		[]unsafe.Pointer{unsafe.Pointer(&hwnd), unsafe.Pointer(&showCmd)})

	return win, nil
}

var classRegisteredOnce sync.Once
var classRegisteredErr error

func ensureClassRegistered() error {
	classRegisteredOnce.Do(func() {
		// Create the WndProc callback once.
		wndProcCB = ffi.NewCallback(globalWndProc)

		hInst := getModuleHandle(nil)
		className := utf16Ptr("QuikwinWindow")
		var pin runtime.Pinner
		pin.Pin(className)
		defer pin.Unpin()

		cls := wndClassExW{
			cbSize:        uint32(unsafe.Sizeof(wndClassExW{})),
			style:         0x0003, // CS_HREDRAW | CS_VREDRAW
			lpfnWndProc:   wndProcCB,
			hInstance:     unsafe.Pointer(&hInst),
			lpszClassName: unsafe.Pointer(className),
		}
		var pin2 runtime.Pinner
		pin2.Pin(&cls)
		defer pin2.Unpin()
		var atom uint32
		ffi.CallFunction(&cifRegisterClassExW, _RegisterClassExW, unsafe.Pointer(&atom),
			[]unsafe.Pointer{unsafe.Pointer(&cls)})
		if atom == 0 {
			classRegisteredErr = fmt.Errorf("quikwin/win32: RegisterClassExW failed")
		}
	})
	return classRegisteredErr
}

// globalWndProc is the single C WndProc callback dispatching to per-window handlers.
func globalWndProc(hwnd unsafe.Pointer, msg uint32, wParam uint64, lParam int64) int64 {
	wndProcMu.Lock()
	win := windows[uintptr(hwnd)]
	wndProcMu.Unlock()
	if win == nil {
		return defWindowProc(hwnd, msg, wParam, lParam)
	}
	result, handled := win.handleMessage(msg, wParam, lParam)
	if handled {
		return result
	}
	return defWindowProc(hwnd, msg, wParam, lParam)
}

func (w *window) handleMessage(msg uint32, wParam uint64, lParam int64) (int64, bool) {
	switch msg {
	case wmClose:
		w.mu.Lock()
		w.shouldClose = true
		w.mu.Unlock()
		if fn := w.onClose; fn != nil {
			fn()
		}
		return 0, true

	case wmDestroy:
		wndProcMu.Lock()
		delete(windows, uintptr(w.hwnd))
		wndProcMu.Unlock()
		return 0, true

	case wmSize:
		var rect winRect
		var pin runtime.Pinner
		pin.Pin(&rect)
		var ok int32
		ffi.CallFunction(&cifGetClientRect, _GetClientRect, unsafe.Pointer(&ok),
			[]unsafe.Pointer{unsafe.Pointer(&w.hwnd), unsafe.Pointer(&rect)})
		pin.Unpin()
		nw := uint32(rect.Right - rect.Left)
		nh := uint32(rect.Bottom - rect.Top)
		w.mu.Lock()
		changed := nw != w.width || nh != w.height
		w.width, w.height = nw, nh
		w.mu.Unlock()
		if changed {
			if fn := w.onResize; fn != nil {
				fn(nw, nh)
			}
		}
		return 0, true

	case wmSetFocus:
		if fn := w.onFocus; fn != nil {
			fn(true)
		}
		return 0, true

	case wmKillFocus:
		if fn := w.onFocus; fn != nil {
			fn(false)
		}
		return 0, true

	case wmKeyDown, wmSysKeyDown:
		key := vkToKey(uint32(wParam))
		mods := currentMods()
		repeat := (lParam>>30)&1 == 1
		action := wtypes.Press
		if repeat {
			action = wtypes.Repeat
		}
		if fn := w.onKey; fn != nil {
			fn(key, action, mods)
		}
		return 0, true

	case wmKeyUp, wmSysKeyUp:
		key := vkToKey(uint32(wParam))
		mods := currentMods()
		if fn := w.onKey; fn != nil {
			fn(key, wtypes.Release, mods)
		}
		return 0, true

	case wmChar:
		r := rune(wParam)
		if r >= 32 && r != 127 {
			if fn := w.onChar; fn != nil {
				fn(r)
			}
		}
		return 0, true

	case wmMouseMove:
		x := float64(int16(lParam & 0xffff))
		y := float64(int16((lParam >> 16) & 0xffff))
		if fn := w.onMouseMove; fn != nil {
			fn(x, y)
		}
		return 0, true

	case wmLButtonDown:
		mods := currentMods()
		if fn := w.onMouseButton; fn != nil {
			fn(wtypes.ButtonLeft, wtypes.Press, mods)
		}
		return 0, true
	case wmLButtonUp:
		mods := currentMods()
		if fn := w.onMouseButton; fn != nil {
			fn(wtypes.ButtonLeft, wtypes.Release, mods)
		}
		return 0, true
	case wmRButtonDown:
		mods := currentMods()
		if fn := w.onMouseButton; fn != nil {
			fn(wtypes.ButtonRight, wtypes.Press, mods)
		}
		return 0, true
	case wmRButtonUp:
		mods := currentMods()
		if fn := w.onMouseButton; fn != nil {
			fn(wtypes.ButtonRight, wtypes.Release, mods)
		}
		return 0, true
	case wmMButtonDown:
		mods := currentMods()
		if fn := w.onMouseButton; fn != nil {
			fn(wtypes.ButtonMiddle, wtypes.Press, mods)
		}
		return 0, true
	case wmMButtonUp:
		mods := currentMods()
		if fn := w.onMouseButton; fn != nil {
			fn(wtypes.ButtonMiddle, wtypes.Release, mods)
		}
		return 0, true

	case wmMouseWheel:
		delta := float64(int16((wParam >> 16) & 0xffff)) / 120.0
		if fn := w.onScroll; fn != nil {
			fn(0, delta)
		}
		return 0, true
	case wmMouseHWheel:
		delta := float64(int16((wParam >> 16) & 0xffff)) / 120.0
		if fn := w.onScroll; fn != nil {
			fn(delta, 0)
		}
		return 0, true

	case wmDropFiles:
		hDrop := unsafe.Pointer(uintptr(wParam))
		paths := queryDroppedFiles(hDrop)
		if fn := w.onDrop; fn != nil && len(paths) > 0 {
			fn(paths)
		}
		return 0, true
	}
	return 0, false
}

// ─── window.Window interface ──────────────────────────────────────────────────

func (w *window) Size() (uint32, uint32) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.width, w.height
}

func (w *window) Scale() float32 {
	// GetDpiForWindow requires Windows 10; fall back to 1.0 for simplicity.
	return 1.0
}

func (w *window) ShouldClose() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.shouldClose
}

func (w *window) PollEvents() {
	var msg winMsg
	var pin runtime.Pinner
	pin.Pin(&msg)
	defer pin.Unpin()
	for {
		var ok int32
		ffi.CallFunction(&cifPeekMessageW, _PeekMessageW, unsafe.Pointer(&ok),
			[]unsafe.Pointer{
				unsafe.Pointer(&msg), unsafe.Pointer(&w.hwnd),
				pU32(0), pU32(0), pU32(pmRemove),
			})
		if ok == 0 {
			break
		}
		ffi.CallFunction(&cifTranslateMessage, _TranslateMessage, nil,
			[]unsafe.Pointer{unsafe.Pointer(&msg)})
		var discard int64
		ffi.CallFunction(&cifDispatchMessageW, _DispatchMessageW, unsafe.Pointer(&discard),
			[]unsafe.Pointer{unsafe.Pointer(&msg)})
	}
}

func (w *window) SetTitle(title string) {
	tw := utf16Ptr(title)
	var pin runtime.Pinner
	pin.Pin(tw)
	defer pin.Unpin()
	var result int32
	ffi.CallFunction(&cifSetWindowTextW, _SetWindowTextW, unsafe.Pointer(&result),
		[]unsafe.Pointer{unsafe.Pointer(&w.hwnd), unsafe.Pointer(tw)})
}

func (w *window) SetCursor(shape wtypes.CursorShape) {
	id := win32CursorID(shape)
	cursor := loadCursor(nil, id)
	var result unsafe.Pointer
	ffi.CallFunction(&cifSetCursor, _SetCursor, unsafe.Pointer(&result),
		[]unsafe.Pointer{unsafe.Pointer(&cursor)})
}

func (w *window) SetMinSize(mw, mh uint32) {
	w.mu.Lock()
	w.minWidth = mw
	w.minHeight = mh
	w.mu.Unlock()
}

func (w *window) SetSize(sw, sh uint32) {
	iw, ih := int32(sw), int32(sh)
	var result int32
	ffi.CallFunction(&cifMoveWindow, _MoveWindow, unsafe.Pointer(&result),
		[]unsafe.Pointer{
			unsafe.Pointer(&w.hwnd),
			pI32(0), pI32(0),
			unsafe.Pointer(&iw), unsafe.Pointer(&ih),
			pI32(1),
		})
}

func (w *window) Destroy() {
	w.mu.Lock()
	if w.destroyed {
		w.mu.Unlock()
		return
	}
	w.destroyed = true
	w.mu.Unlock()
	var result int32
	ffi.CallFunction(&cifDestroyWindow, _DestroyWindow, unsafe.Pointer(&result),
		[]unsafe.Pointer{unsafe.Pointer(&w.hwnd)})
}

func (w *window) BeginDrag() {
	// ReleaseCapture + SendMessage(WM_NCLBUTTONDOWN, HTCAPTION) is the standard
	// way to trigger native title-bar dragging without a real title bar.
	// Omitted here as it requires additional symbols; app can implement via
	// returning HTCAPTION from WM_NCHITTEST via a hook.
	if fn := w.onDragBegin; fn != nil {
		fn(0, 0)
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

// ─── Win32Window interface ────────────────────────────────────────────────────

func (w *window) HWND() (uintptr, error) { return uintptr(w.hwnd), nil }

func (w *window) SetDarkMode(enabled bool) error {
	if _DwmSetWindowAttribute == nil {
		return nil
	}
	val := uint32(0)
	if enabled {
		val = 1
	}
	sz := uint32(4)
	var result int32
	ffi.CallFunction(&cifDwmSetWindowAttribute, _DwmSetWindowAttribute, unsafe.Pointer(&result),
		[]unsafe.Pointer{
			unsafe.Pointer(&w.hwnd),
			pU32(dwmwaUseImmersiveDarkMode),
			unsafe.Pointer(&val),
			unsafe.Pointer(&sz),
		})
	return nil
}

func (w *window) SetSnapLayoutEnabled(_ bool) error {
	// Snap layout is always enabled on Win11 unless you override WM_NCHITTEST.
	return nil
}

func (w *window) SetMicaBackground(enabled bool) error {
	if _DwmSetWindowAttribute == nil {
		return nil
	}
	val := uint32(0)
	if enabled {
		val = 2 // DWMSBT_MAINWINDOW
	}
	sz := uint32(4)
	var result int32
	ffi.CallFunction(&cifDwmSetWindowAttribute, _DwmSetWindowAttribute, unsafe.Pointer(&result),
		[]unsafe.Pointer{
			unsafe.Pointer(&w.hwnd),
			pU32(dwmwaMicaEffect),
			unsafe.Pointer(&val),
			unsafe.Pointer(&sz),
		})
	return nil
}

// ─── vkwin.Window interface ───────────────────────────────────────────────────

func (w *window) NewSurface(instance vk.Instance) (*vk.SurfaceKHR, error) {
	return instance.CreateWin32SurfaceKHR(&vk.Win32SurfaceCreateInfoKHR{
		Hwnd: w.hwnd,
	}, nil)
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func utf16Ptr(s string) *uint16 {
	encoded := utf16.Encode([]rune(s + "\x00"))
	return &encoded[0]
}

func getModuleHandle(name *uint16) unsafe.Pointer {
	var result unsafe.Pointer
	var namePtr unsafe.Pointer
	if name != nil {
		namePtr = unsafe.Pointer(name)
	}
	ffi.CallFunction(&cifGetModuleHandleW, _GetModuleHandleW, unsafe.Pointer(&result),
		[]unsafe.Pointer{unsafe.Pointer(&namePtr)})
	return result
}

func loadCursor(hInst unsafe.Pointer, name uintptr) unsafe.Pointer {
	var result unsafe.Pointer
	ffi.CallFunction(&cifLoadCursorW, _LoadCursorW, unsafe.Pointer(&result),
		[]unsafe.Pointer{unsafe.Pointer(&hInst), unsafe.Pointer(&name)})
	return result
}

func defWindowProc(hwnd unsafe.Pointer, msg uint32, wParam uint64, lParam int64) int64 {
	var result int64
	ffi.CallFunction(&cifDefWindowProcW, _DefWindowProcW, unsafe.Pointer(&result),
		[]unsafe.Pointer{
			unsafe.Pointer(&hwnd), unsafe.Pointer(&msg),
			unsafe.Pointer(&wParam), unsafe.Pointer(&lParam),
		})
	return result
}

func currentMods() wtypes.Mod {
	// GetKeyState requires user32 which is already loaded.
	// Simplified: return 0 (full modifier tracking requires per-key state queries).
	return 0
}

func queryDroppedFiles(hDrop unsafe.Pointer) []string {
	// DragQueryFileW requires shell32.dll — load lazily.
	shell32, err := ffi.LoadLibrary("shell32.dll")
	if err != nil {
		return nil
	}
	qfPtr, err := ffi.GetSymbol(shell32, "DragQueryFileW")
	if err != nil {
		return nil
	}
	finishPtr, _ := ffi.GetSymbol(shell32, "DragFinish")

	var qfCIF types.CallInterface
	if err := ffi.PrepareCallInterface(&qfCIF, types.WindowsCallingConvention,
		_u32, []*types.TypeDescriptor{_ptr, _u32, _ptr, _u32}); err != nil {
		return nil
	}

	// Get count
	countIdx := ^uint32(0) // 0xFFFFFFFF
	var count uint32
	ffi.CallFunction(&qfCIF, qfPtr, unsafe.Pointer(&count),
		[]unsafe.Pointer{unsafe.Pointer(&hDrop), unsafe.Pointer(&countIdx), nil, pU32(0)})

	paths := make([]string, 0, count)
	buf := make([]uint16, 260)
	var pin runtime.Pinner
	pin.Pin(&buf[0])
	for i := uint32(0); i < count; i++ {
		bufLen := uint32(len(buf))
		ffi.CallFunction(&qfCIF, qfPtr, nil,
			[]unsafe.Pointer{unsafe.Pointer(&hDrop), unsafe.Pointer(&i), unsafe.Pointer(&buf[0]), unsafe.Pointer(&bufLen)})
		paths = append(paths, string(utf16.Decode(trimNullW(buf))))
	}
	pin.Unpin()

	if finishPtr != nil {
		var finCIF types.CallInterface
		if err := ffi.PrepareCallInterface(&finCIF, types.WindowsCallingConvention,
			_void, []*types.TypeDescriptor{_ptr}); err == nil {
			ffi.CallFunction(&finCIF, finishPtr, nil, []unsafe.Pointer{unsafe.Pointer(&hDrop)})
		}
	}
	return paths
}

func trimNullW(b []uint16) []uint16 {
	for i, v := range b {
		if v == 0 {
			return b[:i]
		}
	}
	return b
}

func pU32(v uint32) unsafe.Pointer { return unsafe.Pointer(&v) }
func pI32(v int32) unsafe.Pointer  { return unsafe.Pointer(&v) }

func win32CursorID(shape wtypes.CursorShape) uintptr {
	// Standard Win32 IDC_* cursor IDs (as uintptr for LoadCursorW MAKEINTRESOURCE)
	switch shape {
	case wtypes.CursorArrow:
		return 32512 // IDC_ARROW
	case wtypes.CursorIBeam:
		return 32513 // IDC_IBEAM
	case wtypes.CursorCrosshair:
		return 32515 // IDC_CROSS
	case wtypes.CursorHand:
		return 32649 // IDC_HAND
	case wtypes.CursorHResize:
		return 32644 // IDC_SIZEWE
	case wtypes.CursorVResize:
		return 32645 // IDC_SIZENS
	case wtypes.CursorNWSEResize:
		return 32642 // IDC_SIZENWSE
	case wtypes.CursorNESWResize:
		return 32643 // IDC_SIZENESW
	case wtypes.CursorAllResize:
		return 32646 // IDC_SIZEALL
	case wtypes.CursorNotAllowed:
		return 32648 // IDC_NO
	}
	return 32512
}

func vkToKey(vk uint32) wtypes.Key {
	switch {
	case vk >= 0x41 && vk <= 0x5A:
		return wtypes.Key(wtypes.KeyA + wtypes.Key(vk-0x41))
	case vk >= 0x30 && vk <= 0x39:
		return wtypes.Key(wtypes.Key0 + wtypes.Key(vk-0x30))
	case vk >= 0x70 && vk <= 0x7B:
		return wtypes.Key(wtypes.KeyF1 + wtypes.Key(vk-0x70))
	}
	switch vk {
	case 0x20:
		return wtypes.KeySpace
	case 0x1B:
		return wtypes.KeyEscape
	case 0x0D:
		return wtypes.KeyEnter
	case 0x09:
		return wtypes.KeyTab
	case 0x08:
		return wtypes.KeyBackspace
	case 0x2D:
		return wtypes.KeyInsert
	case 0x2E:
		return wtypes.KeyDelete
	case 0x27:
		return wtypes.KeyRight
	case 0x25:
		return wtypes.KeyLeft
	case 0x28:
		return wtypes.KeyDown
	case 0x26:
		return wtypes.KeyUp
	case 0x22:
		return wtypes.KeyPageDown
	case 0x21:
		return wtypes.KeyPageUp
	case 0x24:
		return wtypes.KeyHome
	case 0x23:
		return wtypes.KeyEnd
	case 0x14:
		return wtypes.KeyCapsLock
	case 0x91:
		return wtypes.KeyScrollLock
	case 0x90:
		return wtypes.KeyNumLock
	case 0x2C:
		return wtypes.KeyPrintScreen
	case 0x13:
		return wtypes.KeyPause
	case 0xA0:
		return wtypes.KeyLeftShift
	case 0xA1:
		return wtypes.KeyRightShift
	case 0xA2:
		return wtypes.KeyLeftControl
	case 0xA3:
		return wtypes.KeyRightControl
	case 0xA4:
		return wtypes.KeyLeftAlt
	case 0xA5:
		return wtypes.KeyRightAlt
	case 0x5B:
		return wtypes.KeyLeftSuper
	case 0x5C:
		return wtypes.KeyRightSuper
	case 0x5D:
		return wtypes.KeyMenu
	case 0xBC:
		return wtypes.KeyComma
	case 0xBE:
		return wtypes.KeyPeriod
	case 0xBF:
		return wtypes.KeySlash
	case 0xBA:
		return wtypes.KeySemicolon
	case 0xBB:
		return wtypes.KeyEqual
	case 0xBD:
		return wtypes.KeyMinus
	case 0xDB:
		return wtypes.KeyLeftBracket
	case 0xDC:
		return wtypes.KeyBackslash
	case 0xDD:
		return wtypes.KeyRightBracket
	case 0xDE:
		return wtypes.KeyApostrophe
	case 0xC0:
		return wtypes.KeyGraveAccent
	}
	return wtypes.KeyUnknown
}
