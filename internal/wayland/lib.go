//go:build linux

package wayland

import (
	"fmt"
	"runtime"
	"sync"
	"unsafe"

	"github.com/go-webgpu/goffi/ffi"
	"github.com/go-webgpu/goffi/types"
)

// The Wayland protocol is driven entirely through wayland-go's high-level
// bindings (pkg/wayland + pkg/wl). The only direct FFI left here is for two
// things that wayland-go does not cover: libxkbcommon keymap/state handling,
// and the four libwayland-client entry points needed to read the display
// socket without blocking (PollEvents).

var (
	loadOnce sync.Once
	loadErr  error

	xkbLib unsafe.Pointer
	wlLib  unsafe.Pointer

	_xkbContextNew            unsafe.Pointer
	_xkbContextUnref          unsafe.Pointer
	_xkbKeymapNewFromString   unsafe.Pointer
	_xkbKeymapUnref           unsafe.Pointer
	_xkbStateNew              unsafe.Pointer
	_xkbStateUnref            unsafe.Pointer
	_xkbStateKeyGetOneSym     unsafe.Pointer
	_xkbStateKeyGetUtf32      unsafe.Pointer
	_xkbStateUpdateMask       unsafe.Pointer
	_xkbKeymapModGetIndex     unsafe.Pointer
	_xkbStateModIndexIsActive unsafe.Pointer

	_wlDisplayGetFd       unsafe.Pointer
	_wlDisplayPrepareRead unsafe.Pointer
	_wlDisplayReadEvents  unsafe.Pointer
	_wlDisplayCancelRead  unsafe.Pointer

	cifXkbContextNew            types.CallInterface
	cifXkbContextUnref          types.CallInterface
	cifXkbKeymapNewFromString   types.CallInterface
	cifXkbKeymapUnref           types.CallInterface
	cifXkbStateNew              types.CallInterface
	cifXkbStateUnref            types.CallInterface
	cifXkbStateKeyGetOneSym     types.CallInterface
	cifXkbStateKeyGetUtf32      types.CallInterface
	cifXkbStateUpdateMask       types.CallInterface
	cifXkbKeymapModGetIndex     types.CallInterface
	cifXkbStateModIndexIsActive types.CallInterface

	cifWlDisplayGetFd       types.CallInterface
	cifWlDisplayPrepareRead types.CallInterface
	cifWlDisplayReadEvents  types.CallInterface
	cifWlDisplayCancelRead  types.CallInterface
)

var (
	_ptr  = types.PointerTypeDescriptor
	_i32  = types.SInt32TypeDescriptor
	_u32  = types.UInt32TypeDescriptor
	_void = types.VoidTypeDescriptor
)

func ensureLoaded() error {
	loadOnce.Do(func() {
		var err error
		if xkbLib, err = ffi.LoadLibrary("libxkbcommon.so.0"); err != nil {
			loadErr = fmt.Errorf("quikwin/wayland: load libxkbcommon: %w", err)
			return
		}
		if wlLib, err = ffi.LoadLibrary("libwayland-client.so.0"); err != nil {
			loadErr = fmt.Errorf("quikwin/wayland: load libwayland-client: %w", err)
			return
		}
		if loadErr = loadSymbols(); loadErr != nil {
			return
		}
		loadErr = prepareCIFs()
	})
	return loadErr
}

func loadSymbols() error {
	type pair struct {
		dst  *unsafe.Pointer
		lib  unsafe.Pointer
		name string
	}
	for _, p := range []pair{
		{&_xkbContextNew, xkbLib, "xkb_context_new"},
		{&_xkbContextUnref, xkbLib, "xkb_context_unref"},
		{&_xkbKeymapNewFromString, xkbLib, "xkb_keymap_new_from_string"},
		{&_xkbKeymapUnref, xkbLib, "xkb_keymap_unref"},
		{&_xkbStateNew, xkbLib, "xkb_state_new"},
		{&_xkbStateUnref, xkbLib, "xkb_state_unref"},
		{&_xkbStateKeyGetOneSym, xkbLib, "xkb_state_key_get_one_sym"},
		{&_xkbStateKeyGetUtf32, xkbLib, "xkb_state_key_get_utf32"},
		{&_xkbStateUpdateMask, xkbLib, "xkb_state_update_mask"},
		{&_xkbKeymapModGetIndex, xkbLib, "xkb_keymap_mod_get_index"},
		{&_xkbStateModIndexIsActive, xkbLib, "xkb_state_mod_index_is_active"},
		{&_wlDisplayGetFd, wlLib, "wl_display_get_fd"},
		{&_wlDisplayPrepareRead, wlLib, "wl_display_prepare_read"},
		{&_wlDisplayReadEvents, wlLib, "wl_display_read_events"},
		{&_wlDisplayCancelRead, wlLib, "wl_display_cancel_read"},
	} {
		s, err := ffi.GetSymbol(p.lib, p.name)
		if err != nil {
			return fmt.Errorf("quikwin/wayland: symbol %s: %w", p.name, err)
		}
		*p.dst = s
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
		{&cifXkbContextNew, _ptr, []*types.TypeDescriptor{_u32}},
		{&cifXkbContextUnref, _void, []*types.TypeDescriptor{_ptr}},
		{&cifXkbKeymapNewFromString, _ptr, []*types.TypeDescriptor{_ptr, _ptr, _u32, _u32}},
		{&cifXkbKeymapUnref, _void, []*types.TypeDescriptor{_ptr}},
		{&cifXkbStateNew, _ptr, []*types.TypeDescriptor{_ptr}},
		{&cifXkbStateUnref, _void, []*types.TypeDescriptor{_ptr}},
		{&cifXkbStateKeyGetOneSym, _u32, []*types.TypeDescriptor{_ptr, _u32}},
		{&cifXkbStateKeyGetUtf32, _u32, []*types.TypeDescriptor{_ptr, _u32}},
		{&cifXkbStateUpdateMask, _u32, []*types.TypeDescriptor{_ptr, _u32, _u32, _u32, _u32, _u32, _u32}},
		{&cifXkbKeymapModGetIndex, _u32, []*types.TypeDescriptor{_ptr, _ptr}},
		{&cifXkbStateModIndexIsActive, _i32, []*types.TypeDescriptor{_ptr, _u32, _u32}},
		{&cifWlDisplayGetFd, _i32, []*types.TypeDescriptor{_ptr}},
		{&cifWlDisplayPrepareRead, _i32, []*types.TypeDescriptor{_ptr}},
		{&cifWlDisplayReadEvents, _i32, []*types.TypeDescriptor{_ptr}},
		{&cifWlDisplayCancelRead, _void, []*types.TypeDescriptor{_ptr}},
	} {
		if err := ffi.PrepareCallInterface(e.cif, types.DefaultCall, e.ret, e.args); err != nil {
			return fmt.Errorf("quikwin/wayland: PrepareCallInterface: %w", err)
		}
	}
	return nil
}

// --- xkbcommon wrappers ---

func xkbContextNew() unsafe.Pointer {
	flags := uint32(0)
	var ctx unsafe.Pointer
	ffi.CallFunction(&cifXkbContextNew, _xkbContextNew, unsafe.Pointer(&ctx),
		[]unsafe.Pointer{unsafe.Pointer(&flags)})
	return ctx
}

func xkbContextUnref(ctx unsafe.Pointer) {
	ffi.CallFunction(&cifXkbContextUnref, _xkbContextUnref, nil, []unsafe.Pointer{unsafe.Pointer(&ctx)})
}

// xkbKeymapNewFromString compiles a text-format keymap. keymap must be
// NUL-terminated (the Wayland keymap fd payload includes the terminator).
func xkbKeymapNewFromString(ctx unsafe.Pointer, keymap []byte) unsafe.Pointer {
	var pin runtime.Pinner
	pin.Pin(&keymap[0])
	defer pin.Unpin()
	strPtr := unsafe.Pointer(&keymap[0])
	format := uint32(1) // XKB_KEYMAP_FORMAT_TEXT_V1
	flags := uint32(0)
	var km unsafe.Pointer
	ffi.CallFunction(&cifXkbKeymapNewFromString, _xkbKeymapNewFromString, unsafe.Pointer(&km),
		[]unsafe.Pointer{unsafe.Pointer(&ctx), unsafe.Pointer(&strPtr), unsafe.Pointer(&format), unsafe.Pointer(&flags)})
	return km
}

func xkbKeymapUnref(km unsafe.Pointer) {
	ffi.CallFunction(&cifXkbKeymapUnref, _xkbKeymapUnref, nil, []unsafe.Pointer{unsafe.Pointer(&km)})
}

func xkbStateNew(km unsafe.Pointer) unsafe.Pointer {
	var st unsafe.Pointer
	ffi.CallFunction(&cifXkbStateNew, _xkbStateNew, unsafe.Pointer(&st), []unsafe.Pointer{unsafe.Pointer(&km)})
	return st
}

func xkbStateUnref(st unsafe.Pointer) {
	ffi.CallFunction(&cifXkbStateUnref, _xkbStateUnref, nil, []unsafe.Pointer{unsafe.Pointer(&st)})
}

func xkbStateKeyGetOneSym(st unsafe.Pointer, keycode uint32) uint32 {
	var sym uint32
	ffi.CallFunction(&cifXkbStateKeyGetOneSym, _xkbStateKeyGetOneSym, unsafe.Pointer(&sym),
		[]unsafe.Pointer{unsafe.Pointer(&st), unsafe.Pointer(&keycode)})
	return sym
}

func xkbStateKeyGetUtf32(st unsafe.Pointer, keycode uint32) uint32 {
	var cp uint32
	ffi.CallFunction(&cifXkbStateKeyGetUtf32, _xkbStateKeyGetUtf32, unsafe.Pointer(&cp),
		[]unsafe.Pointer{unsafe.Pointer(&st), unsafe.Pointer(&keycode)})
	return cp
}

func xkbStateUpdateMask(st unsafe.Pointer, depressed, latched, locked, group uint32) {
	var comp uint32
	ffi.CallFunction(&cifXkbStateUpdateMask, _xkbStateUpdateMask, unsafe.Pointer(&comp),
		[]unsafe.Pointer{
			unsafe.Pointer(&st),
			unsafe.Pointer(&depressed), unsafe.Pointer(&latched), unsafe.Pointer(&locked),
			unsafe.Pointer(&group), unsafe.Pointer(&group), unsafe.Pointer(&group),
		})
}

func xkbKeymapModGetIndex(km unsafe.Pointer, name string) uint32 {
	b := append([]byte(name), 0)
	var pin runtime.Pinner
	pin.Pin(&b[0])
	defer pin.Unpin()
	namePtr := unsafe.Pointer(&b[0])
	var idx uint32
	ffi.CallFunction(&cifXkbKeymapModGetIndex, _xkbKeymapModGetIndex, unsafe.Pointer(&idx),
		[]unsafe.Pointer{unsafe.Pointer(&km), unsafe.Pointer(&namePtr)})
	return idx
}

func xkbStateModIndexIsActive(st unsafe.Pointer, idx uint32) bool {
	component := uint32(1 << 2) // XKB_STATE_MODS_EFFECTIVE
	var r int32
	ffi.CallFunction(&cifXkbStateModIndexIsActive, _xkbStateModIndexIsActive, unsafe.Pointer(&r),
		[]unsafe.Pointer{unsafe.Pointer(&st), unsafe.Pointer(&idx), unsafe.Pointer(&component)})
	return r > 0
}

// --- libwayland event-loop pump ---
//
// wayland-go exposes blocking Dispatch plus non-reading DispatchPending/Flush,
// but not the prepare_read/read_events handshake PollEvents needs to drain the
// socket without blocking. These four entry points fill that gap; they operate
// on the same default queue the generated dispatchers are attached to.

func displayGetFd(display unsafe.Pointer) int32 {
	var fd int32
	ffi.CallFunction(&cifWlDisplayGetFd, _wlDisplayGetFd, unsafe.Pointer(&fd),
		[]unsafe.Pointer{unsafe.Pointer(&display)})
	return fd
}

func displayPrepareRead(display unsafe.Pointer) int32 {
	var r int32
	ffi.CallFunction(&cifWlDisplayPrepareRead, _wlDisplayPrepareRead, unsafe.Pointer(&r),
		[]unsafe.Pointer{unsafe.Pointer(&display)})
	return r
}

func displayReadEvents(display unsafe.Pointer) {
	var r int32
	ffi.CallFunction(&cifWlDisplayReadEvents, _wlDisplayReadEvents, unsafe.Pointer(&r),
		[]unsafe.Pointer{unsafe.Pointer(&display)})
}

func displayCancelRead(display unsafe.Pointer) {
	ffi.CallFunction(&cifWlDisplayCancelRead, _wlDisplayCancelRead, nil,
		[]unsafe.Pointer{unsafe.Pointer(&display)})
}
