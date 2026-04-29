//go:build linux

package wayland

import "unsafe"

// wlInterface mirrors struct wl_interface (40 bytes on 64-bit).
// Field order matches C struct exactly for ABI compatibility.
type wlInterface struct {
	name        *byte
	version     int32
	methodCount int32
	methods     *wlMessage
	eventCount  int32
	_           int32 // padding
	events      *wlMessage
}

// wlMessage mirrors struct wl_message (24 bytes on 64-bit).
type wlMessage struct {
	name      *byte
	signature *byte
	types     **wlInterface
}

// wlArgument is a union of 8 bytes — large enough for any Wayland wire argument.
type wlArgument [8]byte

func wlArgUint(v uint32) wlArgument {
	var a wlArgument
	*(*uint32)(unsafe.Pointer(&a[0])) = v
	return a
}

func wlArgPtr(p unsafe.Pointer) wlArgument {
	var a wlArgument
	*(*uintptr)(unsafe.Pointer(&a[0])) = uintptr(p)
	return a
}

func wlArgStr(s *byte) wlArgument {
	return wlArgPtr(unsafe.Pointer(s))
}

func wlArgNewID() wlArgument { return wlArgument{} }

// bstr returns a *byte pointing at a null-terminated copy of s.
// The backing array is allocated on the heap and never freed (static strings only).
func bstr(s string) *byte {
	b := make([]byte, len(s)+1)
	copy(b, s)
	return &b[0]
}

// Protocol interface definitions. These are the minimal descriptors needed for
// wl_proxy_marshal_array_constructor_versioned to bind globals correctly.
// Event/method arrays carry correct counts so dispatch works; we provide minimal
// wl_message entries with valid signatures so libwayland can parse arguments.

var (
	// Signature strings for wl_message descriptors.
	sigEmpty  = bstr("")
	sigU      = bstr("u")
	sigS      = bstr("s")
	sigN      = bstr("n")
	sigNo     = bstr("no")
	sigUsun   = bstr("usun")
	sigIi     = bstr("ii")
	sigIia    = bstr("iia")
	sigO      = bstr("o")
	sigUo     = bstr("uo")
	sigUoa    = bstr("uoa")
	sigUhu    = bstr("uhu")
	sigUuuu   = bstr("uuuu")
	sigUuuuu  = bstr("uuuuu")
	sigUoff   = bstr("uoff")
	sigUuf    = bstr("uuf")
	sigUsu    = bstr("usu")
	sigUff    = bstr("uff")

	// sharedNullTypes is a valid pointer to an array of null wlInterface pointers.
	// Used as the types field in wl_message when no interface-typed args are present.
	// libwayland dereferences types[i] for 'n'/'o' args; NULL entries mean untyped.
	sharedNullTypes = [16]*wlInterface{}
	nilTypes        = &sharedNullTypes[0]
)

// wl_registry: 1 request, 2 events
var (
	wlRegistryMethods = [1]wlMessage{
		{name: bstr("bind"), signature: sigUsun, types: nilTypes},
	}
	wlRegistryEvents = [2]wlMessage{
		{name: bstr("global"), signature: sigUsu, types: nilTypes},
		{name: bstr("global_remove"), signature: sigU, types: nilTypes},
	}
	ifaceWlRegistry = wlInterface{
		name:        bstr("wl_registry"),
		version:     1,
		methodCount: 1,
		methods:     &wlRegistryMethods[0],
		eventCount:  2,
		events:      &wlRegistryEvents[0],
	}
)

// wl_compositor: 2 requests, 0 events
var (
	wlCompositorMethods = [2]wlMessage{
		{name: bstr("create_surface"), signature: sigN, types: nilTypes},
		{name: bstr("create_region"), signature: sigN, types: nilTypes},
	}
	ifaceWlCompositor = wlInterface{
		name:        bstr("wl_compositor"),
		version:     5,
		methodCount: 2,
		methods:     &wlCompositorMethods[0],
		eventCount:  0,
	}
)

// wl_surface: 11 requests, 2 events (only opcodes 0-6 need valid entries for our usage)
var (
	wlSurfaceMethods = [7]wlMessage{
		{name: bstr("destroy"), signature: sigEmpty, types: nilTypes},
		{name: bstr("attach"), signature: bstr("?oii"), types: nilTypes},
		{name: bstr("damage"), signature: bstr("iiii"), types: nilTypes},
		{name: bstr("frame"), signature: sigN, types: nilTypes},
		{name: bstr("set_opaque_region"), signature: bstr("?o"), types: nilTypes},
		{name: bstr("set_input_region"), signature: bstr("?o"), types: nilTypes},
		{name: bstr("commit"), signature: sigEmpty, types: nilTypes},
	}
	wlSurfaceEvents = [2]wlMessage{
		{name: bstr("enter"), signature: sigO, types: nilTypes},
		{name: bstr("leave"), signature: sigO, types: nilTypes},
	}
	ifaceWlSurface = wlInterface{
		name:        bstr("wl_surface"),
		version:     6,
		methodCount: 7,
		methods:     &wlSurfaceMethods[0],
		eventCount:  2,
		events:      &wlSurfaceEvents[0],
	}
)

// xdg_wm_base: 4 requests, 1 event (ping)
var (
	xdgWmBaseMethods = [4]wlMessage{
		{name: bstr("destroy"), signature: sigEmpty, types: nilTypes},
		{name: bstr("create_positioner"), signature: sigN, types: nilTypes},
		{name: bstr("get_xdg_surface"), signature: sigNo, types: nilTypes},
		{name: bstr("pong"), signature: sigU, types: nilTypes},
	}
	xdgWmBaseEvents = [1]wlMessage{
		{name: bstr("ping"), signature: sigU, types: nilTypes},
	}
	ifaceXdgWmBase = wlInterface{
		name:        bstr("xdg_wm_base"),
		version:     6,
		methodCount: 4,
		methods:     &xdgWmBaseMethods[0],
		eventCount:  1,
		events:      &xdgWmBaseEvents[0],
	}
)

// xdg_surface: 5 requests, 1 event (configure)
var (
	xdgSurfaceMethods = [5]wlMessage{
		{name: bstr("destroy"), signature: sigEmpty, types: nilTypes},
		{name: bstr("get_toplevel"), signature: sigN, types: nilTypes},
		{name: bstr("get_popup"), signature: bstr("noo"), types: nilTypes},
		{name: bstr("set_window_geometry"), signature: bstr("iiii"), types: nilTypes},
		{name: bstr("ack_configure"), signature: sigU, types: nilTypes},
	}
	xdgSurfaceEvents = [1]wlMessage{
		{name: bstr("configure"), signature: sigU, types: nilTypes},
	}
	ifaceXdgSurface = wlInterface{
		name:        bstr("xdg_surface"),
		version:     6,
		methodCount: 5,
		methods:     &xdgSurfaceMethods[0],
		eventCount:  1,
		events:      &xdgSurfaceEvents[0],
	}
)

// xdg_toplevel: 14 requests, 4 events (entries 0-8 needed for set_min_size at opcode 8)
var (
	xdgToplevelMethods = [9]wlMessage{
		{name: bstr("destroy"), signature: sigEmpty, types: nilTypes},
		{name: bstr("set_parent"), signature: bstr("?o"), types: nilTypes},
		{name: bstr("set_title"), signature: sigS, types: nilTypes},
		{name: bstr("set_app_id"), signature: sigS, types: nilTypes},
		{name: bstr("show_window_menu"), signature: bstr("ouii"), types: nilTypes},
		{name: bstr("move"), signature: bstr("ou"), types: nilTypes},
		{name: bstr("resize"), signature: bstr("ouu"), types: nilTypes},
		{name: bstr("set_max_size"), signature: sigIi, types: nilTypes},
		{name: bstr("set_min_size"), signature: sigIi, types: nilTypes},
	}
	xdgToplevelEvents = [4]wlMessage{
		{name: bstr("configure"), signature: sigIia, types: nilTypes},
		{name: bstr("close"), signature: sigEmpty, types: nilTypes},
		{name: bstr("configure_bounds"), signature: sigIi, types: nilTypes},
		{name: bstr("wm_capabilities"), signature: sigU, types: nilTypes},
	}
	ifaceXdgToplevel = wlInterface{
		name:        bstr("xdg_toplevel"),
		version:     6,
		methodCount: 9,
		methods:     &xdgToplevelMethods[0],
		eventCount:  4,
		events:      &xdgToplevelEvents[0],
	}
)

// wl_seat: 4 requests, 2 events
var (
	wlSeatMethods = [4]wlMessage{
		{name: bstr("get_pointer"), signature: sigN, types: nilTypes},
		{name: bstr("get_keyboard"), signature: sigN, types: nilTypes},
		{name: bstr("get_touch"), signature: sigN, types: nilTypes},
		{name: bstr("release"), signature: sigEmpty, types: nilTypes},
	}
	wlSeatEvents = [2]wlMessage{
		{name: bstr("capabilities"), signature: sigU, types: nilTypes},
		{name: bstr("name"), signature: sigS, types: nilTypes},
	}
	ifaceWlSeat = wlInterface{
		name:        bstr("wl_seat"),
		version:     7,
		methodCount: 4,
		methods:     &wlSeatMethods[0],
		eventCount:  2,
		events:      &wlSeatEvents[0],
	}
)

// wl_keyboard: 1 request (release at opcode 0), 6 events
var (
	wlKeyboardMethods = [1]wlMessage{
		{name: bstr("release"), signature: sigEmpty, types: nilTypes},
	}
	wlKeyboardEvents = [6]wlMessage{
		{name: bstr("keymap"), signature: sigUhu, types: nilTypes},
		{name: bstr("enter"), signature: sigUoa, types: nilTypes},
		{name: bstr("leave"), signature: sigUo, types: nilTypes},
		{name: bstr("key"), signature: sigUuuu, types: nilTypes},
		{name: bstr("modifiers"), signature: sigUuuuu, types: nilTypes},
		{name: bstr("repeat_info"), signature: sigIi, types: nilTypes},
	}
	ifaceWlKeyboard = wlInterface{
		name:        bstr("wl_keyboard"),
		version:     7,
		methodCount: 1,
		methods:     &wlKeyboardMethods[0],
		eventCount:  6,
		events:      &wlKeyboardEvents[0],
	}
)

// wl_pointer: 2 requests, 9 events
var (
	wlPointerMethods = [2]wlMessage{
		{name: bstr("set_cursor"), signature: bstr("u?oii"), types: nilTypes},
		{name: bstr("release"), signature: sigEmpty, types: nilTypes},
	}
	wlPointerEvents = [9]wlMessage{
		{name: bstr("enter"), signature: sigUoff, types: nilTypes},
		{name: bstr("leave"), signature: sigUo, types: nilTypes},
		{name: bstr("motion"), signature: sigUff, types: nilTypes},
		{name: bstr("button"), signature: sigUuuu, types: nilTypes},
		{name: bstr("axis"), signature: sigUuf, types: nilTypes},
		{name: bstr("frame"), signature: sigEmpty, types: nilTypes},
		{name: bstr("axis_source"), signature: sigU, types: nilTypes},
		{name: bstr("axis_stop"), signature: bstr("uu"), types: nilTypes},
		{name: bstr("axis_discrete"), signature: bstr("uu"), types: nilTypes},
	}
	ifaceWlPointer = wlInterface{
		name:        bstr("wl_pointer"),
		version:     7,
		methodCount: 2,
		methods:     &wlPointerMethods[0],
		eventCount:  9,
		events:      &wlPointerEvents[0],
	}
)

// zxdg_decoration_manager_v1: 2 requests, 0 events
var (
	zxdgDecorMgrMethods = [2]wlMessage{
		{name: bstr("destroy"), signature: sigEmpty, types: nilTypes},
		{name: bstr("get_toplevel_decoration"), signature: sigNo, types: nilTypes},
	}
	ifaceZxdgDecorationManagerV1 = wlInterface{
		name:        bstr("zxdg_decoration_manager_v1"),
		version:     1,
		methodCount: 2,
		methods:     &zxdgDecorMgrMethods[0],
	}
)

// zxdg_toplevel_decoration_v1: 3 requests, 1 event
var (
	zxdgToplevelDecorMethods = [3]wlMessage{
		{name: bstr("destroy"), signature: sigEmpty, types: nilTypes},
		{name: bstr("set_mode"), signature: sigU, types: nilTypes},
		{name: bstr("unset_mode"), signature: sigEmpty, types: nilTypes},
	}
	zxdgToplevelDecorEvents = [1]wlMessage{
		{name: bstr("configure"), signature: sigU, types: nilTypes},
	}
	ifaceZxdgToplevelDecorationV1 = wlInterface{
		name:        bstr("zxdg_toplevel_decoration_v1"),
		version:     1,
		methodCount: 3,
		methods:     &zxdgToplevelDecorMethods[0],
		eventCount:  1,
		events:      &zxdgToplevelDecorEvents[0],
	}
)

// Wayland request opcodes — vars so they are addressable for ffi.CallFunction args.
var (
	opcWlDisplayGetRegistry      = uint32(1) // wl_display.get_registry (opcode 1)
	opcWlRegistryBind            = uint32(0)
	opcWlCompositorCreateSurface = uint32(0)
	opcXdgWmBaseGetXdgSurface    = uint32(2)
	opcXdgWmBasePong             = uint32(3)
	opcXdgSurfaceGetToplevel     = uint32(1)
	opcXdgSurfaceAckConfigure    = uint32(4)
	opcXdgToplevelSetTitle       = uint32(2)
	opcXdgToplevelSetAppID       = uint32(3)
	opcXdgToplevelSetMinSize     = uint32(8)
	opcXdgToplevelDestroy        = uint32(0)
	opcWlSurfaceCommit           = uint32(6)
	opcWlSeatGetPointer          = uint32(0)
	opcWlSeatGetKeyboard         = uint32(1)
	opcWlPointerRelease                   = uint32(1)
	opcWlKeyboardRelease                  = uint32(0)
	opcZxdgDecorationMgrGetToplevelDecor = uint32(1)
	opcZxdgToplevelDecorSetMode          = uint32(1)

	// XKB format/flag vars (addressable equivalents of the constants below)
	varXkbKeymapFormatTextV1   = xkbKeymapFormatTextV1
	varXkbKeymapCompileNoFlags = xkbKeymapCompileNoFlags
)

// xkb_context_flags
const xkbContextNoFlags = uint32(0)

// xkb_keymap_format
const xkbKeymapFormatTextV1 = uint32(1)

// xkb_keymap_compile_flags
const xkbKeymapCompileNoFlags = uint32(0)

// xkb_state_component — XKB_STATE_MODS_EFFECTIVE = (1 << 2)
const xkbStateModsEffective = uint32(1 << 2)

// wl_pointer button state
const (
	wlPointerButtonStateReleased = uint32(0)
	wlPointerButtonStatePressed  = uint32(1)
)

// wl_keyboard key state
const (
	wlKeyboardKeyStateReleased = uint32(0)
	wlKeyboardKeyStatePressed  = uint32(1)
)

// wl_seat capability bits
const (
	wlSeatCapPointer  = uint32(1)
	wlSeatCapKeyboard = uint32(2)
)

// wl_keyboard keymap format
const wlKeyboardKeymapFormatXkbV1 = uint32(1)

// Linux evdev button codes (used in wl_pointer button events)
const (
	btnLeft   = uint32(0x110)
	btnRight  = uint32(0x111)
	btnMiddle = uint32(0x112)
	btnSide   = uint32(0x113)
	btnExtra  = uint32(0x114)
)

// wl_pointer axis
const (
	wlPointerAxisVerticalScroll   = uint32(0)
	wlPointerAxisHorizontalScroll = uint32(1)
)

// wl_fixed_t to float64 conversion: fixed point with 8 fractional bits.
func wlFixedToFloat(v int32) float64 {
	return float64(v) / 256.0
}
