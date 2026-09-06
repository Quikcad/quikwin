package cocoa

import "strings"

// Shortcut parsing is the one part of the menu bar that is plain string work
// rather than AppKit calls, so it carries no build tag and is tested on every
// platform.

// NSEventModifierFlags.
const (
	modShift   = uint64(1 << 17)
	modControl = uint64(1 << 18)
	modOption  = uint64(1 << 19)
	modCommand = uint64(1 << 20)
)

// namedKeys are the key equivalents that are not the character they are named
// after. The values are the Unicode points AppKit uses for them.
var namedKeys = map[string]string{
	"up": "\uf700", "down": "\uf701", "left": "\uf702", "right": "\uf703",
	"f1": "\uf704", "f2": "\uf705", "f3": "\uf706", "f4": "\uf707",
	"f5": "\uf708", "f6": "\uf709", "f7": "\uf70a", "f8": "\uf70b",
	"f9": "\uf70c", "f10": "\uf70d", "f11": "\uf70e", "f12": "\uf70f",
	"insert": "\uf727", "delete": "\uf728", "home": "\uf729", "end": "\uf72b",
	"pageup": "\uf72c", "pagedown": "\uf72d", "help": "\uf746",
	"backspace": "\b", "tab": "\t", "return": "\r", "enter": "\x03",
	"escape": "\x1b", "esc": "\x1b", "space": " ",
}

// parseShortcut turns a chord into the key equivalent and modifier mask an
// NSMenuItem takes. An empty or unrecognised chord yields no shortcut, so a
// mis-spelt one leaves the entry unbound rather than bound to the wrong key.
func parseShortcut(s string) (key string, mods uint64) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", 0
	}

	parts := strings.Split(s, "+")
	if len(parts) == 1 {
		// A chord written in symbols carries its modifiers as a prefix rather
		// than as separate parts: ⇧⌘Z.
		parts = splitSymbolic(s)
	}

	for _, p := range parts[:len(parts)-1] {
		m, ok := modifierOf(p)
		if !ok {
			return "", 0
		}
		mods |= m
	}

	key, ok := keyOf(parts[len(parts)-1])
	if !ok {
		return "", 0
	}
	if mods == 0 {
		// A menu shortcut without a stated modifier is a Command shortcut;
		// nothing else is offered bare on macOS.
		mods = modCommand
	}
	return key, mods
}

// splitSymbolic peels leading modifier symbols off a chord, leaving the key as
// the final part.
func splitSymbolic(s string) []string {
	var parts []string
	for _, r := range s {
		if _, ok := modifierOf(string(r)); ok {
			parts = append(parts, string(r))
			continue
		}
		break
	}
	return append(parts, s[len(strings.Join(parts, "")):])
}

func modifierOf(s string) (uint64, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "cmd", "command", "⌘":
		return modCommand, true
	case "shift", "⇧":
		return modShift, true
	case "alt", "opt", "option", "⌥":
		return modOption, true
	case "ctrl", "control", "⌃":
		return modControl, true
	}
	return 0, false
}

func keyOf(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", false
	}
	if k, ok := namedKeys[strings.ToLower(s)]; ok {
		return k, true
	}
	// A key equivalent is one character. AppKit matches it case-sensitively
	// and expects the lowercase form, with Shift carried in the mask.
	if r := []rune(s); len(r) == 1 {
		return strings.ToLower(s), true
	}
	return "", false
}
