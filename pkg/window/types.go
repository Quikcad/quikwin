package window

type Key uint32

const (
	KeyUnknown Key = iota
	KeySpace
	KeyApostrophe
	KeyComma
	KeyMinus
	KeyPeriod
	KeySlash
	Key0
	Key1
	Key2
	Key3
	Key4
	Key5
	Key6
	Key7
	Key8
	Key9
	KeySemicolon
	KeyEqual
	KeyA
	KeyB
	KeyC
	KeyD
	KeyE
	KeyF
	KeyG
	KeyH
	KeyI
	KeyJ
	KeyK
	KeyL
	KeyM
	KeyN
	KeyO
	KeyP
	KeyQ
	KeyR
	KeyS
	KeyT
	KeyU
	KeyV
	KeyW
	KeyX
	KeyY
	KeyZ
	KeyLeftBracket
	KeyBackslash
	KeyRightBracket
	KeyGraveAccent
	KeyEscape
	KeyEnter
	KeyTab
	KeyBackspace
	KeyInsert
	KeyDelete
	KeyRight
	KeyLeft
	KeyDown
	KeyUp
	KeyPageUp
	KeyPageDown
	KeyHome
	KeyEnd
	KeyCapsLock
	KeyScrollLock
	KeyNumLock
	KeyPrintScreen
	KeyPause
	KeyF1
	KeyF2
	KeyF3
	KeyF4
	KeyF5
	KeyF6
	KeyF7
	KeyF8
	KeyF9
	KeyF10
	KeyF11
	KeyF12
	KeyLeftShift
	KeyLeftControl
	KeyLeftAlt
	KeyLeftSuper
	KeyRightShift
	KeyRightControl
	KeyRightAlt
	KeyRightSuper
	KeyMenu
)

type Action uint8

const (
	Release Action = iota
	Press
	Repeat
)

type Mod uint8

const (
	ModShift   Mod = 1 << iota
	ModControl
	ModAlt
	ModSuper
	ModCapsLock
	ModNumLock
)

type Button uint8

const (
	ButtonLeft Button = iota
	ButtonRight
	ButtonMiddle
	Button4
	Button5
)

type CursorShape uint8

const (
	CursorArrow CursorShape = iota
	CursorIBeam
	CursorCrosshair
	CursorHand
	CursorHResize
	CursorVResize
	CursorNWSEResize
	CursorNESWResize
	CursorAllResize
	CursorNotAllowed
)
