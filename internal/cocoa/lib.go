//go:build darwin

package cocoa

import (
	"fmt"
	"runtime"
	"sync"
	"unsafe"

	"github.com/go-webgpu/goffi/ffi"
	"github.com/go-webgpu/goffi/types"
)

var (
	loadOnce sync.Once
	loadErr  error

	_objc       unsafe.Pointer
	_appkit     unsafe.Pointer
	_foundation unsafe.Pointer

	// ObjC runtime
	_objcMsgSend                unsafe.Pointer
	_objcMsgSendFpret           unsafe.Pointer // x86_64: scalar float returns
	_selRegisterName            unsafe.Pointer
	_objcGetClass               unsafe.Pointer
	_objcAllocateClassPair      unsafe.Pointer
	_classAddMethod             unsafe.Pointer
	_objcRegisterClassPair      unsafe.Pointer
	_objcSetAssociatedObject    unsafe.Pointer
	_objcGetAssociatedObject    unsafe.Pointer

	// CIFs for fixed-arity objc_msgSend variants

	// (id, SEL) → id
	cifMsg0 types.CallInterface
	// (id, SEL, id) → id
	cifMsg1p types.CallInterface
	// (id, SEL, ptr) → void
	cifMsg1pVoid types.CallInterface
	// (id, SEL, i64) → id
	cifMsg1i types.CallInterface
	// (id, SEL, i64) → void
	cifMsg1iVoid types.CallInterface
	// (id, SEL, i32) → void  (e.g. setReleasedWhenClosed:)
	cifMsg1bVoid types.CallInterface
	// (id, SEL) → i64  (e.g. type, keyCode, buttonNumber, windowNumber)
	cifMsg0i types.CallInterface
	// (id, SEL) → u64  (e.g. modifierFlags, styleMask)
	cifMsg0u types.CallInterface
	// (id, SEL) → f64  (e.g. backingScaleFactor, scrollingDeltaX)
	cifMsg0f types.CallInterface
	// (id, SEL, i32) → i32  (windowShouldClose: returns BOOL)
	cifMsg1bReti types.CallInterface
	// (id, SEL, f64, f64, f64, f64, u64, u64, i32) → id  (initWithContentRect:styleMask:backing:defer:)
	cifMsgInitWindow types.CallInterface
	// (id, SEL, f64) → void  (setContentsScale:)
	cifMsg1fVoid types.CallInterface
	// (id, SEL, f64, f64) → void  (setMinSize:, setMaxSize:, setContentSize:)
	cifMsg2fVoid types.CallInterface
	// objc_setAssociatedObject(id, key, value, policy) → void
	cifSetAssoc types.CallInterface
	// objc_getAssociatedObject(id, key) → id
	cifGetAssoc types.CallInterface
	// sel_registerName(char*) → SEL
	cifSelRegister types.CallInterface
	// objc_getClass(char*) → Class
	cifGetClass types.CallInterface
	// objc_allocateClassPair(Class, char*, size_t) → Class
	cifAllocClassPair types.CallInterface
	// class_addMethod(Class, SEL, IMP, char*) → BOOL (i32)
	cifClassAddMethod types.CallInterface
	// objc_registerClassPair(Class) → void
	cifRegisterClassPair types.CallInterface
	// nextEventMatchingMask:untilDate:inMode:dequeue: (app, sel, mask u64, date id, mode id, dequeue i32) → id
	cifNextEvent types.CallInterface
)

var (
	_ptr  = types.PointerTypeDescriptor
	_i32  = types.SInt32TypeDescriptor
	_i64  = types.SInt64TypeDescriptor
	_u64  = types.UInt64TypeDescriptor
	_void = types.VoidTypeDescriptor
	_f64  = types.DoubleTypeDescriptor
)

func ensureLoaded() error {
	loadOnce.Do(func() {
		var err error
		_objc, err = ffi.LoadLibrary("/usr/lib/libobjc.A.dylib")
		if err != nil {
			loadErr = fmt.Errorf("quikwin/cocoa: load libobjc: %w", err)
			return
		}
		_appkit, err = ffi.LoadLibrary("/System/Library/Frameworks/AppKit.framework/Versions/C/AppKit")
		if err != nil {
			loadErr = fmt.Errorf("quikwin/cocoa: load AppKit: %w", err)
			return
		}
		_foundation, err = ffi.LoadLibrary("/System/Library/Frameworks/Foundation.framework/Versions/C/Foundation")
		if err != nil {
			loadErr = fmt.Errorf("quikwin/cocoa: load Foundation: %w", err)
			return
		}
		if loadErr = loadSymbols(); loadErr != nil {
			return
		}
		loadErr = prepareCIFs()
	})
	return loadErr
}

func sym(lib unsafe.Pointer, name string) (unsafe.Pointer, error) {
	p, err := ffi.GetSymbol(lib, name)
	if err != nil {
		return nil, fmt.Errorf("quikwin/cocoa: symbol %s: %w", name, err)
	}
	return p, nil
}

func loadSymbols() error {
	o := func(name string) (unsafe.Pointer, error) { return sym(_objc, name) }

	type pair struct {
		dst  *unsafe.Pointer
		fn   func(string) (unsafe.Pointer, error)
		name string
	}
	for _, p := range []pair{
		{&_objcMsgSend, o, "objc_msgSend"},
		{&_selRegisterName, o, "sel_registerName"},
		{&_objcGetClass, o, "objc_getClass"},
		{&_objcAllocateClassPair, o, "objc_allocateClassPair"},
		{&_classAddMethod, o, "class_addMethod"},
		{&_objcRegisterClassPair, o, "objc_registerClassPair"},
		{&_objcSetAssociatedObject, o, "objc_setAssociatedObject"},
		{&_objcGetAssociatedObject, o, "objc_getAssociatedObject"},
	} {
		var err error
		if *p.dst, err = p.fn(p.name); err != nil {
			return err
		}
	}
	// objc_msgSend_fpret exists only on x86_64; arm64 routes all return
	// types including float through objc_msgSend. Alias so call sites that
	// reference _objcMsgSendFpret work on both architectures.
	if runtime.GOARCH == "arm64" {
		_objcMsgSendFpret = _objcMsgSend
	} else {
		var err error
		if _objcMsgSendFpret, err = o("objc_msgSend_fpret"); err != nil {
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
	// macOS uses System V AMD64 on x86_64 and AAPCS64 on arm64.
	// goffi's DefaultCall maps to the platform default (SysV on darwin/amd64, AAPCS64 on darwin/arm64).
	cc := types.DefaultCall
	prep := func(cif *types.CallInterface, ret *types.TypeDescriptor, args []*types.TypeDescriptor) error {
		return ffi.PrepareCallInterface(cif, cc, ret, args)
	}
	for _, e := range []entry{
		// (id, SEL) → id
		{&cifMsg0, _ptr, []*types.TypeDescriptor{_ptr, _ptr}},
		// (id, SEL, id) → id
		{&cifMsg1p, _ptr, []*types.TypeDescriptor{_ptr, _ptr, _ptr}},
		// (id, SEL, ptr) → void
		{&cifMsg1pVoid, _void, []*types.TypeDescriptor{_ptr, _ptr, _ptr}},
		// (id, SEL, i64) → id
		{&cifMsg1i, _ptr, []*types.TypeDescriptor{_ptr, _ptr, _i64}},
		// (id, SEL, i64) → void
		{&cifMsg1iVoid, _void, []*types.TypeDescriptor{_ptr, _ptr, _i64}},
		// (id, SEL, i32) → void
		{&cifMsg1bVoid, _void, []*types.TypeDescriptor{_ptr, _ptr, _i32}},
		// (id, SEL) → i64
		{&cifMsg0i, _i64, []*types.TypeDescriptor{_ptr, _ptr}},
		// (id, SEL) → u64
		{&cifMsg0u, _u64, []*types.TypeDescriptor{_ptr, _ptr}},
		// (id, SEL) → f64  (uses fpret on x86_64; plain msgSend on arm64)
		{&cifMsg0f, _f64, []*types.TypeDescriptor{_ptr, _ptr}},
		// (id, SEL, i32) → i32
		{&cifMsg1bReti, _i32, []*types.TypeDescriptor{_ptr, _ptr, _i32}},
		// initWithContentRect:styleMask:backing:defer:
		// CGRect = (x f64, y f64, w f64, h f64), styleMask u64, backing u64, defer i32 → id
		{&cifMsgInitWindow, _ptr, []*types.TypeDescriptor{_ptr, _ptr, _f64, _f64, _f64, _f64, _u64, _u64, _i32}},
		// setContentsScale: takes CGFloat (f64)
		{&cifMsg1fVoid, _void, []*types.TypeDescriptor{_ptr, _ptr, _f64}},
		// setMinSize:/setMaxSize: take NSSize = (w f64, h f64)
		{&cifMsg2fVoid, _void, []*types.TypeDescriptor{_ptr, _ptr, _f64, _f64}},
		// objc_setAssociatedObject(id, key*, value, policy u64) → void
		{&cifSetAssoc, _void, []*types.TypeDescriptor{_ptr, _ptr, _ptr, _u64}},
		// objc_getAssociatedObject(id, key*) → id
		{&cifGetAssoc, _ptr, []*types.TypeDescriptor{_ptr, _ptr}},
		// sel_registerName(char*) → SEL (ptr)
		{&cifSelRegister, _ptr, []*types.TypeDescriptor{_ptr}},
		// objc_getClass(char*) → Class (ptr)
		{&cifGetClass, _ptr, []*types.TypeDescriptor{_ptr}},
		// objc_allocateClassPair(superclass, name, extraBytes) → Class
		{&cifAllocClassPair, _ptr, []*types.TypeDescriptor{_ptr, _ptr, _u64}},
		// class_addMethod(cls, sel, imp, types) → BOOL (i32)
		{&cifClassAddMethod, _i32, []*types.TypeDescriptor{_ptr, _ptr, _ptr, _ptr}},
		// objc_registerClassPair(cls) → void
		{&cifRegisterClassPair, _void, []*types.TypeDescriptor{_ptr}},
		// nextEventMatchingMask:untilDate:inMode:dequeue: (app, sel, mask u64, date id, mode id, dequeue i32) → id
		{&cifNextEvent, _ptr, []*types.TypeDescriptor{_ptr, _ptr, _u64, _ptr, _ptr, _i32}},
	} {
		if err := prep(e.cif, e.ret, e.args); err != nil {
			return fmt.Errorf("quikwin/cocoa: PrepareCallInterface: %w", err)
		}
	}
	return nil
}
