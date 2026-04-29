//go:build linux

package x11

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

	_lib unsafe.Pointer

	// function pointers
	_XOpenDisplay        unsafe.Pointer
	_XCloseDisplay       unsafe.Pointer
	_XDefaultRootWindow  unsafe.Pointer
	_XCreateSimpleWindow unsafe.Pointer
	_XMapWindow          unsafe.Pointer
	_XDestroyWindow      unsafe.Pointer
	_XSelectInput        unsafe.Pointer
	_XPending            unsafe.Pointer
	_XNextEvent          unsafe.Pointer
	_XStoreName          unsafe.Pointer
	_XResizeWindow       unsafe.Pointer
	_XInternAtom         unsafe.Pointer
	_XSetWMProtocols     unsafe.Pointer
	_XCreateFontCursor   unsafe.Pointer
	_XDefineCursor       unsafe.Pointer
	_XFlush              unsafe.Pointer
	_XGrabPointer        unsafe.Pointer
	_XUngrabPointer      unsafe.Pointer
	_XLookupKeysym       unsafe.Pointer
	_XLookupString       unsafe.Pointer
	_XMoveWindow         unsafe.Pointer
	_XSetWMNormalHints   unsafe.Pointer
	_XGetWindowAttributes unsafe.Pointer
	_XQueryPointer       unsafe.Pointer
	_XFree               unsafe.Pointer
	_XSendEvent          unsafe.Pointer
	_XChangeProperty     unsafe.Pointer
	_XGetWindowProperty  unsafe.Pointer
	_XConvertSelection   unsafe.Pointer
	_XGetDefault         unsafe.Pointer

	// call interfaces
	cifXOpenDisplay        types.CallInterface
	cifXCloseDisplay       types.CallInterface
	cifXDefaultRootWindow  types.CallInterface
	cifXCreateSimpleWindow types.CallInterface
	cifXMapWindow          types.CallInterface
	cifXDestroyWindow      types.CallInterface
	cifXSelectInput        types.CallInterface
	cifXPending            types.CallInterface
	cifXNextEvent          types.CallInterface
	cifXStoreName          types.CallInterface
	cifXResizeWindow       types.CallInterface
	cifXInternAtom         types.CallInterface
	cifXSetWMProtocols     types.CallInterface
	cifXCreateFontCursor   types.CallInterface
	cifXDefineCursor       types.CallInterface
	cifXFlush              types.CallInterface
	cifXGrabPointer        types.CallInterface
	cifXUngrabPointer      types.CallInterface
	cifXLookupKeysym       types.CallInterface
	cifXLookupString       types.CallInterface
	cifXMoveWindow         types.CallInterface
	cifXSetWMNormalHints   types.CallInterface
	cifXGetWindowAttributes types.CallInterface
	cifXQueryPointer       types.CallInterface
	cifXFree               types.CallInterface
	cifXSendEvent          types.CallInterface
	cifXChangeProperty     types.CallInterface
	cifXGetWindowProperty  types.CallInterface
	cifXConvertSelection   types.CallInterface
	cifXGetDefault         types.CallInterface
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
		_lib, err = ffi.LoadLibrary("libX11.so.6")
		if err != nil {
			loadErr = fmt.Errorf("quikwin/x11: load libX11: %w", err)
			return
		}
		if loadErr = loadSymbols(); loadErr != nil {
			return
		}
		loadErr = prepareCIFs()
	})
	return loadErr
}

func sym(name string) (unsafe.Pointer, error) {
	p, err := ffi.GetSymbol(_lib, name)
	if err != nil {
		return nil, fmt.Errorf("quikwin/x11: symbol %s: %w", name, err)
	}
	return p, nil
}

func loadSymbols() error {
	var err error
	type pair struct {
		dst  *unsafe.Pointer
		name string
	}
	for _, p := range []pair{
		{&_XOpenDisplay, "XOpenDisplay"},
		{&_XCloseDisplay, "XCloseDisplay"},
		{&_XDefaultRootWindow, "XDefaultRootWindow"},
		{&_XCreateSimpleWindow, "XCreateSimpleWindow"},
		{&_XMapWindow, "XMapWindow"},
		{&_XDestroyWindow, "XDestroyWindow"},
		{&_XSelectInput, "XSelectInput"},
		{&_XPending, "XPending"},
		{&_XNextEvent, "XNextEvent"},
		{&_XStoreName, "XStoreName"},
		{&_XResizeWindow, "XResizeWindow"},
		{&_XInternAtom, "XInternAtom"},
		{&_XSetWMProtocols, "XSetWMProtocols"},
		{&_XCreateFontCursor, "XCreateFontCursor"},
		{&_XDefineCursor, "XDefineCursor"},
		{&_XFlush, "XFlush"},
		{&_XGrabPointer, "XGrabPointer"},
		{&_XUngrabPointer, "XUngrabPointer"},
		{&_XLookupKeysym, "XLookupKeysym"},
		{&_XLookupString, "XLookupString"},
		{&_XMoveWindow, "XMoveWindow"},
		{&_XSetWMNormalHints, "XSetWMNormalHints"},
		{&_XGetWindowAttributes, "XGetWindowAttributes"},
		{&_XQueryPointer, "XQueryPointer"},
		{&_XFree, "XFree"},
		{&_XSendEvent, "XSendEvent"},
		{&_XChangeProperty, "XChangeProperty"},
		{&_XGetWindowProperty, "XGetWindowProperty"},
		{&_XConvertSelection, "XConvertSelection"},
		{&_XGetDefault, "XGetDefault"},
	} {
		if *p.dst, err = sym(p.name); err != nil {
			return err
		}
	}
	return nil
}

func prepareCIFs() error {
	type entry struct {
		cif  *types.CallInterface
		ret  *types.TypeDescriptor
		args []*types.TypeDescriptor
	}
	for _, e := range []entry{
		// XOpenDisplay(char*) → ptr
		{&cifXOpenDisplay, _ptr, []*types.TypeDescriptor{_ptr}},
		// XCloseDisplay(ptr) → i32
		{&cifXCloseDisplay, _i32, []*types.TypeDescriptor{_ptr}},
		// XDefaultRootWindow(ptr) → u64
		{&cifXDefaultRootWindow, _u64, []*types.TypeDescriptor{_ptr}},
		// XCreateSimpleWindow(ptr, u64, i32, i32, u32, u32, u32, u64, u64) → u64
		{&cifXCreateSimpleWindow, _u64, []*types.TypeDescriptor{_ptr, _u64, _i32, _i32, _u32, _u32, _u32, _u64, _u64}},
		// XMapWindow(ptr, u64) → i32
		{&cifXMapWindow, _i32, []*types.TypeDescriptor{_ptr, _u64}},
		// XDestroyWindow(ptr, u64) → i32
		{&cifXDestroyWindow, _i32, []*types.TypeDescriptor{_ptr, _u64}},
		// XSelectInput(ptr, u64, i64) → i32
		{&cifXSelectInput, _i32, []*types.TypeDescriptor{_ptr, _u64, _i64}},
		// XPending(ptr) → i32
		{&cifXPending, _i32, []*types.TypeDescriptor{_ptr}},
		// XNextEvent(ptr, ptr) → i32
		{&cifXNextEvent, _i32, []*types.TypeDescriptor{_ptr, _ptr}},
		// XStoreName(ptr, u64, ptr) → i32
		{&cifXStoreName, _i32, []*types.TypeDescriptor{_ptr, _u64, _ptr}},
		// XResizeWindow(ptr, u64, u32, u32) → i32
		{&cifXResizeWindow, _i32, []*types.TypeDescriptor{_ptr, _u64, _u32, _u32}},
		// XInternAtom(ptr, ptr, i32) → u64
		{&cifXInternAtom, _u64, []*types.TypeDescriptor{_ptr, _ptr, _i32}},
		// XSetWMProtocols(ptr, u64, ptr, i32) → i32
		{&cifXSetWMProtocols, _i32, []*types.TypeDescriptor{_ptr, _u64, _ptr, _i32}},
		// XCreateFontCursor(ptr, u32) → u64
		{&cifXCreateFontCursor, _u64, []*types.TypeDescriptor{_ptr, _u32}},
		// XDefineCursor(ptr, u64, u64) → i32
		{&cifXDefineCursor, _i32, []*types.TypeDescriptor{_ptr, _u64, _u64}},
		// XFlush(ptr) → i32
		{&cifXFlush, _i32, []*types.TypeDescriptor{_ptr}},
		// XGrabPointer(ptr, u64, i32, u32, i32, i32, u64, u64, u64) → i32
		{&cifXGrabPointer, _i32, []*types.TypeDescriptor{_ptr, _u64, _i32, _u32, _i32, _i32, _u64, _u64, _u64}},
		// XUngrabPointer(ptr, u64) → i32
		{&cifXUngrabPointer, _i32, []*types.TypeDescriptor{_ptr, _u64}},
		// XLookupKeysym(ptr, i32) → u64
		{&cifXLookupKeysym, _u64, []*types.TypeDescriptor{_ptr, _i32}},
		// XLookupString(ptr, ptr, i32, ptr, ptr) → i32
		{&cifXLookupString, _i32, []*types.TypeDescriptor{_ptr, _ptr, _i32, _ptr, _ptr}},
		// XMoveWindow(ptr, u64, i32, i32) → i32
		{&cifXMoveWindow, _i32, []*types.TypeDescriptor{_ptr, _u64, _i32, _i32}},
		// XSetWMNormalHints(ptr, u64, ptr) → void
		{&cifXSetWMNormalHints, _void, []*types.TypeDescriptor{_ptr, _u64, _ptr}},
		// XGetWindowAttributes(ptr, u64, ptr) → i32
		{&cifXGetWindowAttributes, _i32, []*types.TypeDescriptor{_ptr, _u64, _ptr}},
		// XQueryPointer(ptr, u64, ptr, ptr, ptr, ptr, ptr, ptr, ptr) → i32
		{&cifXQueryPointer, _i32, []*types.TypeDescriptor{_ptr, _u64, _ptr, _ptr, _ptr, _ptr, _ptr, _ptr, _ptr}},
		// XFree(ptr) → i32
		{&cifXFree, _i32, []*types.TypeDescriptor{_ptr}},
		// XSendEvent(ptr, u64, i32, i64, ptr) → i32
		{&cifXSendEvent, _i32, []*types.TypeDescriptor{_ptr, _u64, _i32, _i64, _ptr}},
		// XChangeProperty(ptr, u64, u64, u64, i32, i32, ptr, i32) → i32
		{&cifXChangeProperty, _i32, []*types.TypeDescriptor{_ptr, _u64, _u64, _u64, _i32, _i32, _ptr, _i32}},
		// XGetWindowProperty(ptr, u64, u64, i64, i64, i32, u64, ptr, ptr, ptr, ptr, ptr) → i32
		{&cifXGetWindowProperty, _i32, []*types.TypeDescriptor{_ptr, _u64, _u64, _i64, _i64, _i32, _u64, _ptr, _ptr, _ptr, _ptr, _ptr}},
		// XConvertSelection(ptr, u64, u64, u64, u64, u64) → i32
		{&cifXConvertSelection, _i32, []*types.TypeDescriptor{_ptr, _u64, _u64, _u64, _u64, _u64}},
		// XGetDefault(ptr, ptr, ptr) → ptr
		{&cifXGetDefault, _ptr, []*types.TypeDescriptor{_ptr, _ptr, _ptr}},
	} {
		if err := ffi.PrepareCallInterface(e.cif, types.DefaultCall, e.ret, e.args); err != nil {
			return fmt.Errorf("quikwin/x11: PrepareCallInterface: %w", err)
		}
	}
	return nil
}
