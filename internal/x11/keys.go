//go:build linux

package x11

import "github.com/Quikcad/quikwin/internal/wtypes"

// X11 keysym constants (from X11/keysymdef.h)
const (
	xkSpace       = 0x0020
	xkApostrophe  = 0x0027
	xkComma       = 0x002c
	xkMinus       = 0x002d
	xkPeriod      = 0x002e
	xkSlash       = 0x002f
	xk0           = 0x0030
	xk9           = 0x0039
	xkSemicolon   = 0x003b
	xkEqual       = 0x003d
	xkA           = 0x0041
	xkZ           = 0x005a
	xkLeftBracket = 0x005b
	xkBackslash   = 0x005c
	xkRightBracket = 0x005d
	xkGraveAccent = 0x0060
	xkLowerA      = 0x0061
	xkLowerZ      = 0x007a

	xkBackSpace   = 0xff08
	xkTab         = 0xff09
	xkReturn      = 0xff0d
	xkPause       = 0xff13
	xkScrollLock  = 0xff14
	xkEscape      = 0xff1b
	xkHome        = 0xff50
	xkLeft        = 0xff51
	xkUp          = 0xff52
	xkRight       = 0xff53
	xkDown        = 0xff54
	xkPageUp      = 0xff55
	xkPageDown    = 0xff56
	xkEnd         = 0xff57
	xkInsert      = 0xff63
	xkMenu        = 0xff67
	xkNumLock     = 0xff7f
	xkF1          = 0xffbe
	xkF12         = 0xffc9
	xkShiftL      = 0xffe1
	xkShiftR      = 0xffe2
	xkControlL    = 0xffe3
	xkControlR    = 0xffe4
	xkCapsLock    = 0xffe5
	xkAltL        = 0xffe9
	xkAltR        = 0xffea
	xkSuperL      = 0xffeb
	xkSuperR      = 0xffec
	xkPrint       = 0xff61
	xkDelete      = 0xffff
)

// modFromState converts an X11 modifier state mask to our Mod type.
func modFromState(state uint32) wtypes.Mod {
	var m wtypes.Mod
	if state&1 != 0 {
		m |= wtypes.ModShift
	}
	if state&2 != 0 {
		m |= wtypes.ModCapsLock
	}
	if state&4 != 0 {
		m |= wtypes.ModControl
	}
	if state&8 != 0 {
		m |= wtypes.ModAlt
	}
	if state&64 != 0 {
		m |= wtypes.ModSuper
	}
	if state&16 != 0 {
		m |= wtypes.ModNumLock
	}
	return m
}

// keysymToKey maps an X11 KeySym to our Key constant.
func keysymToKey(sym uint64) wtypes.Key {
	switch {
	case sym == xkSpace:
		return wtypes.KeySpace
	case sym == xkApostrophe:
		return wtypes.KeyApostrophe
	case sym == xkComma:
		return wtypes.KeyComma
	case sym == xkMinus:
		return wtypes.KeyMinus
	case sym == xkPeriod:
		return wtypes.KeyPeriod
	case sym == xkSlash:
		return wtypes.KeySlash
	case sym >= xk0 && sym <= xk9:
		return wtypes.Key(wtypes.Key0 + wtypes.Key(sym-xk0))
	case sym == xkSemicolon:
		return wtypes.KeySemicolon
	case sym == xkEqual:
		return wtypes.KeyEqual
	case sym >= xkLowerA && sym <= xkLowerZ:
		return wtypes.Key(wtypes.KeyA + wtypes.Key(sym-xkLowerA))
	case sym >= xkA && sym <= xkZ:
		return wtypes.Key(wtypes.KeyA + wtypes.Key(sym-xkA))
	case sym == xkLeftBracket:
		return wtypes.KeyLeftBracket
	case sym == xkBackslash:
		return wtypes.KeyBackslash
	case sym == xkRightBracket:
		return wtypes.KeyRightBracket
	case sym == xkGraveAccent:
		return wtypes.KeyGraveAccent
	case sym == xkEscape:
		return wtypes.KeyEscape
	case sym == xkReturn:
		return wtypes.KeyEnter
	case sym == xkTab:
		return wtypes.KeyTab
	case sym == xkBackSpace:
		return wtypes.KeyBackspace
	case sym == xkInsert:
		return wtypes.KeyInsert
	case sym == xkDelete:
		return wtypes.KeyDelete
	case sym == xkRight:
		return wtypes.KeyRight
	case sym == xkLeft:
		return wtypes.KeyLeft
	case sym == xkDown:
		return wtypes.KeyDown
	case sym == xkUp:
		return wtypes.KeyUp
	case sym == xkPageUp:
		return wtypes.KeyPageUp
	case sym == xkPageDown:
		return wtypes.KeyPageDown
	case sym == xkHome:
		return wtypes.KeyHome
	case sym == xkEnd:
		return wtypes.KeyEnd
	case sym == xkCapsLock:
		return wtypes.KeyCapsLock
	case sym == xkScrollLock:
		return wtypes.KeyScrollLock
	case sym == xkNumLock:
		return wtypes.KeyNumLock
	case sym == xkPrint:
		return wtypes.KeyPrintScreen
	case sym == xkPause:
		return wtypes.KeyPause
	case sym >= xkF1 && sym <= xkF12:
		return wtypes.Key(wtypes.KeyF1 + wtypes.Key(sym-xkF1))
	case sym == xkShiftL:
		return wtypes.KeyLeftShift
	case sym == xkShiftR:
		return wtypes.KeyRightShift
	case sym == xkControlL:
		return wtypes.KeyLeftControl
	case sym == xkControlR:
		return wtypes.KeyRightControl
	case sym == xkAltL:
		return wtypes.KeyLeftAlt
	case sym == xkAltR:
		return wtypes.KeyRightAlt
	case sym == xkSuperL:
		return wtypes.KeyLeftSuper
	case sym == xkSuperR:
		return wtypes.KeyRightSuper
	case sym == xkMenu:
		return wtypes.KeyMenu
	}
	return wtypes.KeyUnknown
}

// xfontCursorID maps CursorShape to XC_* cursor font IDs.
func xfontCursorID(shape wtypes.CursorShape) uint32 {
	switch shape {
	case wtypes.CursorArrow:
		return 2 // XC_arrow
	case wtypes.CursorIBeam:
		return 152 // XC_xterm
	case wtypes.CursorCrosshair:
		return 34 // XC_crosshair
	case wtypes.CursorHand:
		return 60 // XC_hand2
	case wtypes.CursorHResize:
		return 108 // XC_sb_h_double_arrow
	case wtypes.CursorVResize:
		return 116 // XC_sb_v_double_arrow
	case wtypes.CursorNWSEResize:
		return 120 // XC_sizing (approximate)
	case wtypes.CursorNESWResize:
		return 120
	case wtypes.CursorAllResize:
		return 52 // XC_fleur
	case wtypes.CursorNotAllowed:
		return 0 // XC_X_cursor
	}
	return 2
}
