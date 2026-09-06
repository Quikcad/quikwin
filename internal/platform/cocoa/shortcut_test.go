package cocoa

import "testing"

func TestParseShortcut(t *testing.T) {
	cases := []struct {
		in   string
		key  string
		mods uint64
	}{
		{"", "", 0},
		{"   ", "", 0},

		// A bare key is a Command shortcut: nothing else is offered bare in a
		// macOS menu.
		{"S", "s", modCommand},
		{"s", "s", modCommand},

		{"Cmd+S", "s", modCommand},
		{"Command+s", "s", modCommand},
		{"Shift+Cmd+Z", "z", modShift | modCommand},
		{"Ctrl+Alt+Delete", "", modControl | modOption},
		{"Opt+Cmd+I", "i", modOption | modCommand},

		// Symbols carry their modifiers as a prefix rather than as parts.
		{"⌘S", "s", modCommand},
		{"⇧⌘Z", "z", modShift | modCommand},
		{"⌥⌘I", "i", modOption | modCommand},

		{"Cmd+Left", "", modCommand},
		{"Cmd+F5", "", modCommand},
		{"Cmd+Return", "\r", modCommand},
		{"Cmd+Space", " ", modCommand},

		// An unrecognised chord binds nothing rather than the wrong key.
		{"Meta+S", "", 0},
		{"Cmd+Frobnicate", "", 0},
		{"Cmd+", "", 0},
	}

	for _, c := range cases {
		key, mods := parseShortcut(c.in)
		if key != c.key || mods != c.mods {
			t.Errorf("parseShortcut(%q) = (%q, %d), want (%q, %d)", c.in, key, mods, c.key, c.mods)
		}
	}
}

func TestShortcutKeyIsLowercasedWithShiftInTheMask(t *testing.T) {
	// AppKit matches the key equivalent case-sensitively, so an uppercase key
	// with Shift in the mask would never fire.
	key, mods := parseShortcut("Shift+Cmd+S")
	if key != "s" {
		t.Errorf("key = %q, want %q", key, "s")
	}
	if mods&modShift == 0 {
		t.Error("Shift did not reach the modifier mask")
	}
}

func TestZeroMenuItemIsALiveEntry(t *testing.T) {
	// Disabled rather than Enabled: menus built before the field existed must
	// keep working.
	var it MenuItem
	if it.Disabled {
		t.Error("zero MenuItem is disabled")
	}
	if it.Checked {
		t.Error("zero MenuItem is checked")
	}
}
