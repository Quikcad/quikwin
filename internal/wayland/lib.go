//go:build linux

package wayland

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

	_wlLib  unsafe.Pointer
	_xkbLib unsafe.Pointer

	// wl_display
	_wlDisplayConnect         unsafe.Pointer
	_wlDisplayDisconnect      unsafe.Pointer
	_wlDisplayDispatch        unsafe.Pointer
	_wlDisplayDispatchPending unsafe.Pointer
	_wlDisplayFlush           unsafe.Pointer
	_wlDisplayRoundtrip       unsafe.Pointer
	_wlDisplayGetFd           unsafe.Pointer
	_wlDisplayPrepareRead     unsafe.Pointer
	_wlDisplayReadEvents      unsafe.Pointer
	_wlDisplayCancelRead      unsafe.Pointer

	// wl_proxy
	_wlProxyAddListener                      unsafe.Pointer
	_wlProxyDestroy                          unsafe.Pointer
	_wlProxyMarshalConstructor               unsafe.Pointer
	_wlProxyMarshalArrayConstructorVersioned unsafe.Pointer
	_wlProxyMarshalArray                     unsafe.Pointer

	// xkbcommon
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

	// CIFs
	cifWlDisplayConnect                        types.CallInterface
	cifWlDisplayDisconnect                     types.CallInterface
	cifWlDisplayDispatch                       types.CallInterface
	cifWlDisplayDispatchPending                types.CallInterface
	cifWlDisplayFlush                          types.CallInterface
	cifWlDisplayRoundtrip                      types.CallInterface
	cifWlDisplayGetFd                          types.CallInterface
	cifWlDisplayPrepareRead                    types.CallInterface
	cifWlDisplayReadEvents                     types.CallInterface
	cifWlDisplayCancelRead                     types.CallInterface
	cifWlProxyAddListener                      types.CallInterface
	cifWlProxyDestroy                          types.CallInterface
	cifWlProxyMarshalConstructor               types.CallInterface
	cifWlProxyMarshalArrayConstructorVersioned types.CallInterface
	cifWlProxyMarshalArray                     types.CallInterface

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
		_wlLib, err = ffi.LoadLibrary("libwayland-client.so.0")
		if err != nil {
			loadErr = fmt.Errorf("quikwin/wayland: load libwayland-client: %w", err)
			return
		}
		_xkbLib, err = ffi.LoadLibrary("libxkbcommon.so.0")
		if err != nil {
			loadErr = fmt.Errorf("quikwin/wayland: load libxkbcommon: %w", err)
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
		return nil, fmt.Errorf("quikwin/wayland: symbol %s: %w", name, err)
	}
	return p, nil
}

func loadSymbols() error {
	wlSym := func(name string) (unsafe.Pointer, error) { return sym(_wlLib, name) }
	xk := func(name string) (unsafe.Pointer, error) { return sym(_xkbLib, name) }

	type pair struct {
		dst  *unsafe.Pointer
		fn   func(string) (unsafe.Pointer, error)
		name string
	}
	for _, p := range []pair{
		{&_wlDisplayConnect, wlSym, "wl_display_connect"},
		{&_wlDisplayDisconnect, wlSym, "wl_display_disconnect"},
		{&_wlDisplayDispatch, wlSym, "wl_display_dispatch"},
		{&_wlDisplayDispatchPending, wlSym, "wl_display_dispatch_pending"},
		{&_wlDisplayFlush, wlSym, "wl_display_flush"},
		{&_wlDisplayRoundtrip, wlSym, "wl_display_roundtrip"},
		{&_wlDisplayGetFd, wlSym, "wl_display_get_fd"},
		{&_wlDisplayPrepareRead, wlSym, "wl_display_prepare_read"},
		{&_wlDisplayReadEvents, wlSym, "wl_display_read_events"},
		{&_wlDisplayCancelRead, wlSym, "wl_display_cancel_read"},
		{&_wlProxyAddListener, wlSym, "wl_proxy_add_listener"},
		{&_wlProxyDestroy, wlSym, "wl_proxy_destroy"},
		{&_wlProxyMarshalConstructor, wlSym, "wl_proxy_marshal_constructor"},
		{&_wlProxyMarshalArrayConstructorVersioned, wlSym, "wl_proxy_marshal_array_constructor_versioned"},
		{&_wlProxyMarshalArray, wlSym, "wl_proxy_marshal_array"},
		{&_xkbContextNew, xk, "xkb_context_new"},
		{&_xkbContextUnref, xk, "xkb_context_unref"},
		{&_xkbKeymapNewFromString, xk, "xkb_keymap_new_from_string"},
		{&_xkbKeymapUnref, xk, "xkb_keymap_unref"},
		{&_xkbStateNew, xk, "xkb_state_new"},
		{&_xkbStateUnref, xk, "xkb_state_unref"},
		{&_xkbStateKeyGetOneSym, xk, "xkb_state_key_get_one_sym"},
		{&_xkbStateKeyGetUtf32, xk, "xkb_state_key_get_utf32"},
		{&_xkbStateUpdateMask, xk, "xkb_state_update_mask"},
		{&_xkbKeymapModGetIndex, xk, "xkb_keymap_mod_get_index"},
		{&_xkbStateModIndexIsActive, xk, "xkb_state_mod_index_is_active"},
	} {
		var err error
		if *p.dst, err = p.fn(p.name); err != nil {
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
		{&cifWlDisplayConnect, _ptr, []*types.TypeDescriptor{_ptr}},
		{&cifWlDisplayDisconnect, _void, []*types.TypeDescriptor{_ptr}},
		{&cifWlDisplayDispatch, _i32, []*types.TypeDescriptor{_ptr}},
		{&cifWlDisplayDispatchPending, _i32, []*types.TypeDescriptor{_ptr}},
		{&cifWlDisplayFlush, _i32, []*types.TypeDescriptor{_ptr}},
		{&cifWlDisplayRoundtrip, _i32, []*types.TypeDescriptor{_ptr}},
		{&cifWlDisplayGetFd, _i32, []*types.TypeDescriptor{_ptr}},
		{&cifWlDisplayPrepareRead, _i32, []*types.TypeDescriptor{_ptr}},
		{&cifWlDisplayReadEvents, _i32, []*types.TypeDescriptor{_ptr}},
		{&cifWlDisplayCancelRead, _void, []*types.TypeDescriptor{_ptr}},
		{&cifWlProxyMarshalConstructor, _ptr, []*types.TypeDescriptor{_ptr, _u32, _ptr, _ptr}},
		{&cifWlProxyAddListener, _i32, []*types.TypeDescriptor{_ptr, _ptr, _ptr}},
		{&cifWlProxyDestroy, _void, []*types.TypeDescriptor{_ptr}},
		{&cifWlProxyMarshalArrayConstructorVersioned, _ptr, []*types.TypeDescriptor{_ptr, _u32, _ptr, _ptr, _u32}},
		{&cifWlProxyMarshalArray, _void, []*types.TypeDescriptor{_ptr, _u32, _ptr}},
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
	} {
		if err := ffi.PrepareCallInterface(e.cif, types.DefaultCall, e.ret, e.args); err != nil {
			return fmt.Errorf("quikwin/wayland: PrepareCallInterface: %w", err)
		}
	}
	return nil
}
