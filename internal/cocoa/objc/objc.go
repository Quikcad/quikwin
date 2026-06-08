//go:build darwin

// Package objc wraps the raw goffi-driven Objective-C runtime calls used by
// the cocoa backend. It owns the dylib loading, libffi CIF setup and the
// typed msgSend helpers; everything above (selectors, NSApplication, the
// window struct) lives in the parent internal/cocoa package.
package objc

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

	libObjC       unsafe.Pointer
	libAppKit     unsafe.Pointer
	libFoundation unsafe.Pointer

	fpObjCMsgSend             unsafe.Pointer
	fpObjCMsgSendFpret        unsafe.Pointer // x86_64: scalar float returns
	fpSelRegisterName         unsafe.Pointer
	fpObjCGetClass            unsafe.Pointer
	fpObjCAllocateClassPair   unsafe.Pointer
	fpClassAddMethod          unsafe.Pointer
	fpObjCRegisterClassPair   unsafe.Pointer
	fpObjCSetAssociatedObject unsafe.Pointer
	fpObjCGetAssociatedObject unsafe.Pointer

	cifMsg0          types.CallInterface // (id, SEL) → id
	cifMsg1p         types.CallInterface // (id, SEL, id) → id
	cifMsg1pVoid     types.CallInterface // (id, SEL, ptr) → void
	cifMsg1i         types.CallInterface // (id, SEL, i64) → id
	cifMsg1iVoid     types.CallInterface // (id, SEL, i64) → void
	cifMsg1bVoid     types.CallInterface // (id, SEL, i32) → void
	cifMsg0i         types.CallInterface // (id, SEL) → i64
	cifMsg0u         types.CallInterface // (id, SEL) → u64
	cifMsg0f         types.CallInterface // (id, SEL) → f64
	cifMsg1bReti     types.CallInterface // (id, SEL, i32) → i32
	cifMsgInitWindow types.CallInterface // initWithContentRect:styleMask:backing:defer:
	cifMsg1fVoid     types.CallInterface // (id, SEL, f64) → void
	cifMsg2fVoid     types.CallInterface // (id, SEL, f64, f64) → void
	cifSetAssoc      types.CallInterface
	cifGetAssoc      types.CallInterface
	cifSelRegister   types.CallInterface
	cifGetClass      types.CallInterface
	cifAllocClass    types.CallInterface
	cifAddMethod     types.CallInterface
	cifRegisterClass types.CallInterface
	cifNextEvent     types.CallInterface // nextEventMatchingMask:untilDate:inMode:dequeue:
	cifMsg0Point     types.CallInterface // (id, SEL) → NSPoint (struct{f64, f64})
	cifMsg0Rect      types.CallInterface // (id, SEL) → NSRect  (struct{f64, f64, f64, f64})
)

// Point mirrors NSPoint/CGPoint: two CGFloats laid out as { X, Y }. Used to
// receive small struct returns from selectors like locationInWindow.
type Point struct {
	X float64
	Y float64
}

// Rect mirrors NSRect/CGRect: origin + size laid out as { X, Y, W, H }.
type Rect struct {
	X float64
	Y float64
	W float64
	H float64
}

var tNSPoint = &types.TypeDescriptor{
	Size:      16,
	Alignment: 8,
	Kind:      types.StructType,
	Members:   []*types.TypeDescriptor{tF64, tF64},
}

var tNSRect = &types.TypeDescriptor{
	Size:      32,
	Alignment: 8,
	Kind:      types.StructType,
	Members:   []*types.TypeDescriptor{tF64, tF64, tF64, tF64},
}

var (
	tPtr  = types.PointerTypeDescriptor
	tI32  = types.SInt32TypeDescriptor
	tI64  = types.SInt64TypeDescriptor
	tU64  = types.UInt64TypeDescriptor
	tVoid = types.VoidTypeDescriptor
	tF64  = types.DoubleTypeDescriptor
)

// Load resolves the macOS system frameworks and prepares every libffi
// call-interface used by the cocoa backend. Idempotent; safe to call from
// multiple goroutines.
func Load() error {
	loadOnce.Do(func() {
		var err error
		libObjC, err = ffi.LoadLibrary("/usr/lib/libobjc.A.dylib")
		if err != nil {
			loadErr = fmt.Errorf("quikwin/cocoa/objc: load libobjc: %w", err)
			return
		}
		libAppKit, err = ffi.LoadLibrary("/System/Library/Frameworks/AppKit.framework/Versions/C/AppKit")
		if err != nil {
			loadErr = fmt.Errorf("quikwin/cocoa/objc: load AppKit: %w", err)
			return
		}
		libFoundation, err = ffi.LoadLibrary("/System/Library/Frameworks/Foundation.framework/Versions/C/Foundation")
		if err != nil {
			loadErr = fmt.Errorf("quikwin/cocoa/objc: load Foundation: %w", err)
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
		return nil, fmt.Errorf("quikwin/cocoa/objc: symbol %s: %w", name, err)
	}
	return p, nil
}

func loadSymbols() error {
	o := func(name string) (unsafe.Pointer, error) { return sym(libObjC, name) }

	type pair struct {
		dst  *unsafe.Pointer
		name string
	}
	for _, p := range []pair{
		{&fpObjCMsgSend, "objc_msgSend"},
		{&fpSelRegisterName, "sel_registerName"},
		{&fpObjCGetClass, "objc_getClass"},
		{&fpObjCAllocateClassPair, "objc_allocateClassPair"},
		{&fpClassAddMethod, "class_addMethod"},
		{&fpObjCRegisterClassPair, "objc_registerClassPair"},
		{&fpObjCSetAssociatedObject, "objc_setAssociatedObject"},
		{&fpObjCGetAssociatedObject, "objc_getAssociatedObject"},
	} {
		var err error
		if *p.dst, err = o(p.name); err != nil {
			return err
		}
	}
	// objc_msgSend_fpret exists only on x86_64; arm64 routes all return types
	// including float through objc_msgSend.
	if runtime.GOARCH == "arm64" {
		fpObjCMsgSendFpret = fpObjCMsgSend
	} else {
		var err error
		if fpObjCMsgSendFpret, err = o("objc_msgSend_fpret"); err != nil {
			return err
		}
	}
	return nil
}

func prepareCIFs() error {
	cc := types.DefaultCall
	prep := func(cif *types.CallInterface, ret *types.TypeDescriptor, args []*types.TypeDescriptor) error {
		return ffi.PrepareCallInterface(cif, cc, ret, args)
	}
	type entry struct {
		cif  *types.CallInterface
		ret  *types.TypeDescriptor
		args []*types.TypeDescriptor
	}
	for _, e := range []entry{
		{&cifMsg0, tPtr, []*types.TypeDescriptor{tPtr, tPtr}},
		{&cifMsg1p, tPtr, []*types.TypeDescriptor{tPtr, tPtr, tPtr}},
		{&cifMsg1pVoid, tVoid, []*types.TypeDescriptor{tPtr, tPtr, tPtr}},
		{&cifMsg1i, tPtr, []*types.TypeDescriptor{tPtr, tPtr, tI64}},
		{&cifMsg1iVoid, tVoid, []*types.TypeDescriptor{tPtr, tPtr, tI64}},
		{&cifMsg1bVoid, tVoid, []*types.TypeDescriptor{tPtr, tPtr, tI32}},
		{&cifMsg0i, tI64, []*types.TypeDescriptor{tPtr, tPtr}},
		{&cifMsg0u, tU64, []*types.TypeDescriptor{tPtr, tPtr}},
		{&cifMsg0f, tF64, []*types.TypeDescriptor{tPtr, tPtr}},
		{&cifMsg1bReti, tI32, []*types.TypeDescriptor{tPtr, tPtr, tI32}},
		// CGRect (4 f64), styleMask u64, backing u64, defer i32
		{&cifMsgInitWindow, tPtr, []*types.TypeDescriptor{tPtr, tPtr, tF64, tF64, tF64, tF64, tU64, tU64, tI32}},
		{&cifMsg1fVoid, tVoid, []*types.TypeDescriptor{tPtr, tPtr, tF64}},
		{&cifMsg2fVoid, tVoid, []*types.TypeDescriptor{tPtr, tPtr, tF64, tF64}},
		{&cifSetAssoc, tVoid, []*types.TypeDescriptor{tPtr, tPtr, tPtr, tU64}},
		{&cifGetAssoc, tPtr, []*types.TypeDescriptor{tPtr, tPtr}},
		{&cifSelRegister, tPtr, []*types.TypeDescriptor{tPtr}},
		{&cifGetClass, tPtr, []*types.TypeDescriptor{tPtr}},
		{&cifAllocClass, tPtr, []*types.TypeDescriptor{tPtr, tPtr, tU64}},
		{&cifAddMethod, tI32, []*types.TypeDescriptor{tPtr, tPtr, tPtr, tPtr}},
		{&cifRegisterClass, tVoid, []*types.TypeDescriptor{tPtr}},
		// (app, sel, mask u64, date id, mode id, dequeue i32) → id
		{&cifNextEvent, tPtr, []*types.TypeDescriptor{tPtr, tPtr, tU64, tPtr, tPtr, tI32}},
		// (id, SEL) → NSPoint
		{&cifMsg0Point, tNSPoint, []*types.TypeDescriptor{tPtr, tPtr}},
		// (id, SEL) → NSRect
		{&cifMsg0Rect, tNSRect, []*types.TypeDescriptor{tPtr, tPtr}},
	} {
		if err := prep(e.cif, e.ret, e.args); err != nil {
			return fmt.Errorf("quikwin/cocoa/objc: PrepareCallInterface: %w", err)
		}
	}
	return nil
}

// Bstr returns a pointer to a NUL-terminated copy of s, suitable for passing
// to C string parameters.
func Bstr(s string) *byte {
	b := make([]byte, len(s)+1)
	copy(b, s)
	return &b[0]
}

// SelRegister wraps sel_registerName.
func SelRegister(name string) unsafe.Pointer {
	p := Bstr(name)
	var ret unsafe.Pointer
	ffi.CallFunction(&cifSelRegister, fpSelRegisterName, unsafe.Pointer(&ret),
		[]unsafe.Pointer{unsafe.Pointer(&p)})
	return ret
}

// GetClass wraps objc_getClass.
func GetClass(name string) unsafe.Pointer {
	p := Bstr(name)
	var ret unsafe.Pointer
	ffi.CallFunction(&cifGetClass, fpObjCGetClass, unsafe.Pointer(&ret),
		[]unsafe.Pointer{unsafe.Pointer(&p)})
	return ret
}

// AllocateClassPair wraps objc_allocateClassPair with extraBytes=0.
func AllocateClassPair(super unsafe.Pointer, name string) unsafe.Pointer {
	n := Bstr(name)
	var extra uint64
	var ret unsafe.Pointer
	ffi.CallFunction(&cifAllocClass, fpObjCAllocateClassPair, unsafe.Pointer(&ret),
		[]unsafe.Pointer{unsafe.Pointer(&super), unsafe.Pointer(&n), unsafe.Pointer(&extra)})
	return ret
}

// RegisterClassPair wraps objc_registerClassPair.
func RegisterClassPair(cls unsafe.Pointer) {
	ffi.CallFunction(&cifRegisterClass, fpObjCRegisterClassPair, nil,
		[]unsafe.Pointer{unsafe.Pointer(&cls)})
}

// AddMethod attaches a Go callback as an IMP on cls under the given selector.
// All callback params must be uintptr-sized (ObjC passes id/SEL as pointer
// words). typeStr is the standard ObjC type encoding (e.g. "v@:@"). Returns
// the class_addMethod BOOL result so callers can detect a silent failure
// (selector already present, etc).
func AddMethod(cls unsafe.Pointer, selName, typeStr string, fn any) bool {
	sel := SelRegister(selName)
	imp := unsafe.Pointer(ffi.NewCallback(fn))
	ts := Bstr(typeStr)
	var ret int32
	ffi.CallFunction(&cifAddMethod, fpClassAddMethod, unsafe.Pointer(&ret),
		[]unsafe.Pointer{unsafe.Pointer(&cls), unsafe.Pointer(&sel), unsafe.Pointer(&imp), unsafe.Pointer(&ts)})
	return ret != 0
}

// SetAssoc wraps objc_setAssociatedObject.
func SetAssoc(obj, key, value unsafe.Pointer, policy uint64) {
	ffi.CallFunction(&cifSetAssoc, fpObjCSetAssociatedObject, nil,
		[]unsafe.Pointer{unsafe.Pointer(&obj), unsafe.Pointer(&key), unsafe.Pointer(&value), unsafe.Pointer(&policy)})
}

// GetAssoc wraps objc_getAssociatedObject.
func GetAssoc(obj, key unsafe.Pointer) unsafe.Pointer {
	var ret unsafe.Pointer
	ffi.CallFunction(&cifGetAssoc, fpObjCGetAssociatedObject, unsafe.Pointer(&ret),
		[]unsafe.Pointer{unsafe.Pointer(&obj), unsafe.Pointer(&key)})
	return ret
}

// MsgSend0: (id, SEL) → id
func MsgSend0(recv, sel unsafe.Pointer) unsafe.Pointer {
	var ret unsafe.Pointer
	ffi.CallFunction(&cifMsg0, fpObjCMsgSend, unsafe.Pointer(&ret),
		[]unsafe.Pointer{unsafe.Pointer(&recv), unsafe.Pointer(&sel)})
	return ret
}

// MsgSend1p: (id, SEL, id) → id
func MsgSend1p(recv, sel, arg unsafe.Pointer) unsafe.Pointer {
	var ret unsafe.Pointer
	ffi.CallFunction(&cifMsg1p, fpObjCMsgSend, unsafe.Pointer(&ret),
		[]unsafe.Pointer{unsafe.Pointer(&recv), unsafe.Pointer(&sel), unsafe.Pointer(&arg)})
	return ret
}

// MsgSend1pVoid: (id, SEL, ptr) → void
func MsgSend1pVoid(recv, sel, arg unsafe.Pointer) {
	ffi.CallFunction(&cifMsg1pVoid, fpObjCMsgSend, nil,
		[]unsafe.Pointer{unsafe.Pointer(&recv), unsafe.Pointer(&sel), unsafe.Pointer(&arg)})
}

// MsgSend1iVoid: (id, SEL, i64) → void
func MsgSend1iVoid(recv, sel unsafe.Pointer, v int64) {
	ffi.CallFunction(&cifMsg1iVoid, fpObjCMsgSend, nil,
		[]unsafe.Pointer{unsafe.Pointer(&recv), unsafe.Pointer(&sel), unsafe.Pointer(&v)})
}

// MsgSend1bVoid: (id, SEL, i32) → void
func MsgSend1bVoid(recv, sel unsafe.Pointer, v int32) {
	ffi.CallFunction(&cifMsg1bVoid, fpObjCMsgSend, nil,
		[]unsafe.Pointer{unsafe.Pointer(&recv), unsafe.Pointer(&sel), unsafe.Pointer(&v)})
}

// MsgSend0i: (id, SEL) → i64
func MsgSend0i(recv, sel unsafe.Pointer) int64 {
	var ret int64
	ffi.CallFunction(&cifMsg0i, fpObjCMsgSend, unsafe.Pointer(&ret),
		[]unsafe.Pointer{unsafe.Pointer(&recv), unsafe.Pointer(&sel)})
	return ret
}

// MsgSend0u: (id, SEL) → u64
func MsgSend0u(recv, sel unsafe.Pointer) uint64 {
	var ret uint64
	ffi.CallFunction(&cifMsg0u, fpObjCMsgSend, unsafe.Pointer(&ret),
		[]unsafe.Pointer{unsafe.Pointer(&recv), unsafe.Pointer(&sel)})
	return ret
}

// MsgSend0f: (id, SEL) → f64
func MsgSend0f(recv, sel unsafe.Pointer) float64 {
	var ret float64
	ffi.CallFunction(&cifMsg0f, fpObjCMsgSend, unsafe.Pointer(&ret),
		[]unsafe.Pointer{unsafe.Pointer(&recv), unsafe.Pointer(&sel)})
	return ret
}

// MsgSend1fVoid: (id, SEL, f64) → void
func MsgSend1fVoid(recv, sel unsafe.Pointer, v float64) {
	ffi.CallFunction(&cifMsg1fVoid, fpObjCMsgSend, nil,
		[]unsafe.Pointer{unsafe.Pointer(&recv), unsafe.Pointer(&sel), unsafe.Pointer(&v)})
}

// MsgSend2fVoid: (id, SEL, f64, f64) → void
func MsgSend2fVoid(recv, sel unsafe.Pointer, x, y float64) {
	ffi.CallFunction(&cifMsg2fVoid, fpObjCMsgSend, nil,
		[]unsafe.Pointer{unsafe.Pointer(&recv), unsafe.Pointer(&sel), unsafe.Pointer(&x), unsafe.Pointer(&y)})
}

// MsgSendInitWindow: initWithContentRect:styleMask:backing:defer:
func MsgSendInitWindow(recv, sel unsafe.Pointer, x, y, w, h float64, style, backing uint64, deferred int32) unsafe.Pointer {
	var ret unsafe.Pointer
	ffi.CallFunction(&cifMsgInitWindow, fpObjCMsgSend, unsafe.Pointer(&ret),
		[]unsafe.Pointer{unsafe.Pointer(&recv), unsafe.Pointer(&sel),
			unsafe.Pointer(&x), unsafe.Pointer(&y), unsafe.Pointer(&w), unsafe.Pointer(&h),
			unsafe.Pointer(&style), unsafe.Pointer(&backing), unsafe.Pointer(&deferred)})
	return ret
}

// MsgSend0Point: (id, SEL) → NSPoint. For selectors that return a small
// struct of two CGFloats (e.g. locationInWindow, mouseLocation, frame.origin).
func MsgSend0Point(recv, sel unsafe.Pointer) Point {
	var ret Point
	ffi.CallFunction(&cifMsg0Point, fpObjCMsgSend, unsafe.Pointer(&ret),
		[]unsafe.Pointer{unsafe.Pointer(&recv), unsafe.Pointer(&sel)})
	return ret
}

// MsgSend0Rect: (id, SEL) → NSRect. For frame/bounds-style selectors.
func MsgSend0Rect(recv, sel unsafe.Pointer) Rect {
	var ret Rect
	ffi.CallFunction(&cifMsg0Rect, fpObjCMsgSend, unsafe.Pointer(&ret),
		[]unsafe.Pointer{unsafe.Pointer(&recv), unsafe.Pointer(&sel)})
	return ret
}

// NextEvent wraps nextEventMatchingMask:untilDate:inMode:dequeue:.
func NextEvent(app, sel unsafe.Pointer, mask uint64, untilDate, mode unsafe.Pointer, dequeue int32) unsafe.Pointer {
	var ret unsafe.Pointer
	ffi.CallFunction(&cifNextEvent, fpObjCMsgSend, unsafe.Pointer(&ret),
		[]unsafe.Pointer{unsafe.Pointer(&app), unsafe.Pointer(&sel),
			unsafe.Pointer(&mask), unsafe.Pointer(&untilDate), unsafe.Pointer(&mode), unsafe.Pointer(&dequeue)})
	return ret
}
