package wtypes

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
	ModShift Mod = 1 << iota
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

// Config holds window creation parameters shared between the common
// package and platform backends.
type Config struct {
	Title               string
	Width, Height       uint32
	MinWidth, MinHeight uint32
	Resizable           bool
	Border              bool
	Titlebar            bool
	// Centered asks the backend to position the window on the centre of the
	// primary display at creation. Backends that can't query a display (e.g.
	// Wayland in client-side mode) silently ignore the hint.
	Centered bool
}

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

type HitTestResult uint8

const (
	HitTestClient HitTestResult = iota
	HitTestDrag
)

// ResizeEdge identifies which edge(s) the cursor is near. Values match
// the xdg_toplevel resize_edge enum so Wayland can cast directly.
type ResizeEdge uint32

const (
	EdgeNone        ResizeEdge = 0
	EdgeTop         ResizeEdge = 1
	EdgeBottom      ResizeEdge = 2
	EdgeLeft        ResizeEdge = 4
	EdgeTopLeft     ResizeEdge = EdgeTop | EdgeLeft    // 5
	EdgeBottomLeft  ResizeEdge = EdgeBottom | EdgeLeft // 6
	EdgeRight       ResizeEdge = 8
	EdgeTopRight    ResizeEdge = EdgeTop | EdgeRight    // 9
	EdgeBottomRight ResizeEdge = EdgeBottom | EdgeRight // 10
)

// BorderWidth is the pixel width of the invisible resize border on
// undecorated windows.
const BorderWidth = 5.0

// DetectEdge returns the resize edge the point (x, y) falls on within
// a window of dimensions (w, h) and the given border width.
func DetectEdge(x, y, w, h, border float64) ResizeEdge {
	var edge ResizeEdge
	if y < border {
		edge |= EdgeTop
	} else if y > h-border {
		edge |= EdgeBottom
	}
	if x < border {
		edge |= EdgeLeft
	} else if x > w-border {
		edge |= EdgeRight
	}
	return edge
}

// EdgeCursorShape returns the cursor shape appropriate for the given
// resize edge.
func EdgeCursorShape(edge ResizeEdge) CursorShape {
	switch edge {
	case EdgeTop, EdgeBottom:
		return CursorVResize
	case EdgeLeft, EdgeRight:
		return CursorHResize
	case EdgeTopLeft, EdgeBottomRight:
		return CursorNWSEResize
	case EdgeTopRight, EdgeBottomLeft:
		return CursorNESWResize
	default:
		return CursorArrow
	}
}
