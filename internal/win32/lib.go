//go:build windows

package win32

import (
	"fmt"
	"sync"
	"unsafe"

	"github.com/go-webgpu/goffi/ffi"
	"github.com/go-webgpu/goffi/types"
)

var (
	loadOnce sync.Once
	loadErr  error

	_user32   unsafe.Pointer
	_kernel32 unsafe.Pointer
	_dwmapi   unsafe.Pointer

	// user32
	_RegisterClassExW   unsafe.Pointer
	_CreateWindowExW    unsafe.Pointer
	_DestroyWindow      unsafe.Pointer
	_ShowWindow         unsafe.Pointer
	_PeekMessageW       unsafe.Pointer
	_TranslateMessage   unsafe.Pointer
	_DispatchMessageW   unsafe.Pointer
	_DefWindowProcW     unsafe.Pointer
	_PostQuitMessage    unsafe.Pointer
	_GetClientRect      unsafe.Pointer
	_SetWindowTextW     unsafe.Pointer
	_SetWindowPos       unsafe.Pointer
	_LoadCursorW        unsafe.Pointer
	_SetCursor          unsafe.Pointer
	_GetDC              unsafe.Pointer
	_ReleaseDC          unsafe.Pointer
	_InvalidateRect     unsafe.Pointer
	_DragAcceptFiles    unsafe.Pointer
	_DragQueryFileW     unsafe.Pointer
	_DragFinish         unsafe.Pointer
	_SetWindowLongPtrW  unsafe.Pointer
	_GetWindowLongPtrW  unsafe.Pointer
	_AdjustWindowRect   unsafe.Pointer
	_MoveWindow         unsafe.Pointer

	// kernel32
	_GetModuleHandleW unsafe.Pointer

	// dwmapi
	_DwmSetWindowAttribute unsafe.Pointer

	// CIFs
	cifRegisterClassExW    types.CallInterface
	cifCreateWindowExW     types.CallInterface
	cifDestroyWindow       types.CallInterface
	cifShowWindow          types.CallInterface
	cifPeekMessageW        types.CallInterface
	cifTranslateMessage    types.CallInterface
	cifDispatchMessageW    types.CallInterface
	cifDefWindowProcW      types.CallInterface
	cifGetClientRect       types.CallInterface
	cifSetWindowTextW      types.CallInterface
	cifSetWindowPos        types.CallInterface
	cifLoadCursorW         types.CallInterface
	cifSetCursor           types.CallInterface
	cifGetModuleHandleW    types.CallInterface
	cifDwmSetWindowAttribute types.CallInterface
	cifMoveWindow          types.CallInterface
	cifSetWindowLongPtrW   types.CallInterface
	cifGetWindowLongPtrW   types.CallInterface
)

var (
	_ptr  = types.PointerTypeDescriptor
	_i32  = types.SInt32TypeDescriptor
	_u32  = types.UInt32TypeDescriptor
	_i64  = types.SInt64TypeDescriptor
	_u64  = types.UInt64TypeDescriptor
	_void = types.VoidTypeDescriptor
)

func ensureLoaded() error {
	loadOnce.Do(func() {
		var err error
		_user32, err = ffi.LoadLibrary("user32.dll")
		if err != nil {
			loadErr = fmt.Errorf("quikwin/win32: %w", err)
			return
		}
		_kernel32, err = ffi.LoadLibrary("kernel32.dll")
		if err != nil {
			loadErr = fmt.Errorf("quikwin/win32: %w", err)
			return
		}
		_dwmapi, _ = ffi.LoadLibrary("dwmapi.dll") // optional

		if loadErr = loadSymbols(); loadErr != nil {
			return
		}
		loadErr = prepareCIFs()
	})
	return loadErr
}

func loadSymbols() error {
	u := func(name string) (unsafe.Pointer, error) { return getSymbol(_user32, name) }
	k := func(name string) (unsafe.Pointer, error) { return getSymbol(_kernel32, name) }

	type pair struct {
		dst  *unsafe.Pointer
		fn   func(string) (unsafe.Pointer, error)
		name string
	}
	for _, p := range []pair{
		{&_RegisterClassExW, u, "RegisterClassExW"},
		{&_CreateWindowExW, u, "CreateWindowExW"},
		{&_DestroyWindow, u, "DestroyWindow"},
		{&_ShowWindow, u, "ShowWindow"},
		{&_PeekMessageW, u, "PeekMessageW"},
		{&_TranslateMessage, u, "TranslateMessage"},
		{&_DispatchMessageW, u, "DispatchMessageW"},
		{&_DefWindowProcW, u, "DefWindowProcW"},
		{&_GetClientRect, u, "GetClientRect"},
		{&_SetWindowTextW, u, "SetWindowTextW"},
		{&_SetWindowPos, u, "SetWindowPos"},
		{&_LoadCursorW, u, "LoadCursorW"},
		{&_SetCursor, u, "SetCursor"},
		{&_SetWindowLongPtrW, u, "SetWindowLongPtrW"},
		{&_GetWindowLongPtrW, u, "GetWindowLongPtrW"},
		{&_MoveWindow, u, "MoveWindow"},
		{&_GetModuleHandleW, k, "GetModuleHandleW"},
	} {
		var err error
		if *p.dst, err = p.fn(p.name); err != nil {
			return err
		}
	}
	if _dwmapi != nil {
		_DwmSetWindowAttribute, _ = ffi.GetSymbol(_dwmapi, "DwmSetWindowAttribute")
	}
	return nil
}

func getSymbol(lib unsafe.Pointer, name string) (unsafe.Pointer, error) {
	p, err := ffi.GetSymbol(lib, name)
	if err != nil {
		return nil, fmt.Errorf("quikwin/win32: symbol %s: %w", name, err)
	}
	return p, nil
}

func prepareCIFs() error {
	type entry struct {
		cif  *types.CallInterface
		ret  *types.TypeDescriptor
		args []*types.TypeDescriptor
	}
	win := types.WindowsCallingConvention
	prep := func(cif *types.CallInterface, ret *types.TypeDescriptor, args []*types.TypeDescriptor) error {
		return ffi.PrepareCallInterface(cif, win, ret, args)
	}
	for _, e := range []entry{
		// RegisterClassExW(*WNDCLASSEX) → ATOM (u16 as u32)
		{&cifRegisterClassExW, _u32, []*types.TypeDescriptor{_ptr}},
		// CreateWindowExW(exStyle, className, title, style, x, y, w, h, parent, menu, hInstance, param) → HWND
		{&cifCreateWindowExW, _ptr, []*types.TypeDescriptor{_u32, _ptr, _ptr, _u32, _i32, _i32, _i32, _i32, _ptr, _ptr, _ptr, _ptr}},
		// DestroyWindow(HWND) → BOOL
		{&cifDestroyWindow, _i32, []*types.TypeDescriptor{_ptr}},
		// ShowWindow(HWND, nCmdShow) → BOOL
		{&cifShowWindow, _i32, []*types.TypeDescriptor{_ptr, _i32}},
		// PeekMessageW(*MSG, HWND, min, max, remove) → BOOL
		{&cifPeekMessageW, _i32, []*types.TypeDescriptor{_ptr, _ptr, _u32, _u32, _u32}},
		// TranslateMessage(*MSG) → BOOL
		{&cifTranslateMessage, _i32, []*types.TypeDescriptor{_ptr}},
		// DispatchMessageW(*MSG) → LRESULT
		{&cifDispatchMessageW, _i64, []*types.TypeDescriptor{_ptr}},
		// DefWindowProcW(HWND, msg, wParam, lParam) → LRESULT
		{&cifDefWindowProcW, _i64, []*types.TypeDescriptor{_ptr, _u32, _u64, _i64}},
		// GetClientRect(HWND, *RECT) → BOOL
		{&cifGetClientRect, _i32, []*types.TypeDescriptor{_ptr, _ptr}},
		// SetWindowTextW(HWND, *wchar) → BOOL
		{&cifSetWindowTextW, _i32, []*types.TypeDescriptor{_ptr, _ptr}},
		// SetWindowPos(HWND, hWndInsertAfter, x, y, cx, cy, flags) → BOOL
		{&cifSetWindowPos, _i32, []*types.TypeDescriptor{_ptr, _ptr, _i32, _i32, _i32, _i32, _u32}},
		// LoadCursorW(hInstance, name) → HCURSOR
		{&cifLoadCursorW, _ptr, []*types.TypeDescriptor{_ptr, _ptr}},
		// SetCursor(HCURSOR) → HCURSOR
		{&cifSetCursor, _ptr, []*types.TypeDescriptor{_ptr}},
		// GetModuleHandleW(*wchar) → HMODULE
		{&cifGetModuleHandleW, _ptr, []*types.TypeDescriptor{_ptr}},
		// DwmSetWindowAttribute(HWND, attr, *pvAttr, cbAttr) → HRESULT
		{&cifDwmSetWindowAttribute, _i32, []*types.TypeDescriptor{_ptr, _u32, _ptr, _u32}},
		// MoveWindow(HWND, x, y, w, h, repaint) → BOOL
		{&cifMoveWindow, _i32, []*types.TypeDescriptor{_ptr, _i32, _i32, _i32, _i32, _i32}},
		// SetWindowLongPtrW(HWND, index, newVal) → LONG_PTR
		{&cifSetWindowLongPtrW, _i64, []*types.TypeDescriptor{_ptr, _i32, _i64}},
		// GetWindowLongPtrW(HWND, index) → LONG_PTR
		{&cifGetWindowLongPtrW, _i64, []*types.TypeDescriptor{_ptr, _i32}},
	} {
		if err := prep(e.cif, e.ret, e.args); err != nil {
			return fmt.Errorf("quikwin/win32: PrepareCallInterface: %w", err)
		}
	}
	return nil
}
