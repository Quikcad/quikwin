package window

import "github.com/Quikcad/quikwin/internal/wtypes"

type (
	Key         = wtypes.Key
	Action      = wtypes.Action
	Mod         = wtypes.Mod
	Button      = wtypes.Button
	CursorShape = wtypes.CursorShape
)

const (
	KeyUnknown      = wtypes.KeyUnknown
	KeySpace        = wtypes.KeySpace
	KeyApostrophe   = wtypes.KeyApostrophe
	KeyComma        = wtypes.KeyComma
	KeyMinus        = wtypes.KeyMinus
	KeyPeriod       = wtypes.KeyPeriod
	KeySlash        = wtypes.KeySlash
	Key0            = wtypes.Key0
	Key1            = wtypes.Key1
	Key2            = wtypes.Key2
	Key3            = wtypes.Key3
	Key4            = wtypes.Key4
	Key5            = wtypes.Key5
	Key6            = wtypes.Key6
	Key7            = wtypes.Key7
	Key8            = wtypes.Key8
	Key9            = wtypes.Key9
	KeySemicolon    = wtypes.KeySemicolon
	KeyEqual        = wtypes.KeyEqual
	KeyA            = wtypes.KeyA
	KeyB            = wtypes.KeyB
	KeyC            = wtypes.KeyC
	KeyD            = wtypes.KeyD
	KeyE            = wtypes.KeyE
	KeyF            = wtypes.KeyF
	KeyG            = wtypes.KeyG
	KeyH            = wtypes.KeyH
	KeyI            = wtypes.KeyI
	KeyJ            = wtypes.KeyJ
	KeyK            = wtypes.KeyK
	KeyL            = wtypes.KeyL
	KeyM            = wtypes.KeyM
	KeyN            = wtypes.KeyN
	KeyO            = wtypes.KeyO
	KeyP            = wtypes.KeyP
	KeyQ            = wtypes.KeyQ
	KeyR            = wtypes.KeyR
	KeyS            = wtypes.KeyS
	KeyT            = wtypes.KeyT
	KeyU            = wtypes.KeyU
	KeyV            = wtypes.KeyV
	KeyW            = wtypes.KeyW
	KeyX            = wtypes.KeyX
	KeyY            = wtypes.KeyY
	KeyZ            = wtypes.KeyZ
	KeyLeftBracket  = wtypes.KeyLeftBracket
	KeyBackslash    = wtypes.KeyBackslash
	KeyRightBracket = wtypes.KeyRightBracket
	KeyGraveAccent  = wtypes.KeyGraveAccent
	KeyEscape       = wtypes.KeyEscape
	KeyEnter        = wtypes.KeyEnter
	KeyTab          = wtypes.KeyTab
	KeyBackspace    = wtypes.KeyBackspace
	KeyInsert       = wtypes.KeyInsert
	KeyDelete       = wtypes.KeyDelete
	KeyRight        = wtypes.KeyRight
	KeyLeft         = wtypes.KeyLeft
	KeyDown         = wtypes.KeyDown
	KeyUp           = wtypes.KeyUp
	KeyPageUp       = wtypes.KeyPageUp
	KeyPageDown     = wtypes.KeyPageDown
	KeyHome         = wtypes.KeyHome
	KeyEnd          = wtypes.KeyEnd
	KeyCapsLock     = wtypes.KeyCapsLock
	KeyScrollLock   = wtypes.KeyScrollLock
	KeyNumLock      = wtypes.KeyNumLock
	KeyPrintScreen  = wtypes.KeyPrintScreen
	KeyPause        = wtypes.KeyPause
	KeyF1           = wtypes.KeyF1
	KeyF2           = wtypes.KeyF2
	KeyF3           = wtypes.KeyF3
	KeyF4           = wtypes.KeyF4
	KeyF5           = wtypes.KeyF5
	KeyF6           = wtypes.KeyF6
	KeyF7           = wtypes.KeyF7
	KeyF8           = wtypes.KeyF8
	KeyF9           = wtypes.KeyF9
	KeyF10          = wtypes.KeyF10
	KeyF11          = wtypes.KeyF11
	KeyF12          = wtypes.KeyF12
	KeyLeftShift    = wtypes.KeyLeftShift
	KeyLeftControl  = wtypes.KeyLeftControl
	KeyLeftAlt      = wtypes.KeyLeftAlt
	KeyLeftSuper    = wtypes.KeyLeftSuper
	KeyRightShift   = wtypes.KeyRightShift
	KeyRightControl = wtypes.KeyRightControl
	KeyRightAlt     = wtypes.KeyRightAlt
	KeyRightSuper   = wtypes.KeyRightSuper
	KeyMenu         = wtypes.KeyMenu

	Release = wtypes.Release
	Press   = wtypes.Press
	Repeat  = wtypes.Repeat

	ModShift    = wtypes.ModShift
	ModControl  = wtypes.ModControl
	ModAlt      = wtypes.ModAlt
	ModSuper    = wtypes.ModSuper
	ModCapsLock = wtypes.ModCapsLock
	ModNumLock  = wtypes.ModNumLock

	ButtonLeft   = wtypes.ButtonLeft
	ButtonRight  = wtypes.ButtonRight
	ButtonMiddle = wtypes.ButtonMiddle
	Button4      = wtypes.Button4
	Button5      = wtypes.Button5

	CursorArrow      = wtypes.CursorArrow
	CursorIBeam      = wtypes.CursorIBeam
	CursorCrosshair  = wtypes.CursorCrosshair
	CursorHand       = wtypes.CursorHand
	CursorHResize    = wtypes.CursorHResize
	CursorVResize    = wtypes.CursorVResize
	CursorNWSEResize = wtypes.CursorNWSEResize
	CursorNESWResize = wtypes.CursorNESWResize
	CursorAllResize  = wtypes.CursorAllResize
	CursorNotAllowed = wtypes.CursorNotAllowed
)
