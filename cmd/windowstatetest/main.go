// Command windowstatetest is a hands-on check of the window-state controls.
//
// The states it exercises cannot be asserted from a test: they need a real
// window, a real compositor or window server, and a real keystroke. So it opens
// a window, drives the states from the keyboard, and prints what the platform
// reports back. It stays open until the window is closed — nothing is on a
// timer, so there is time to look.
//
// The case it exists for is Escape. Native full screen binds Escape to leaving
// it, so an application maximised that way never sees the key. A zoomed window
// does not, so the window opens zoomed: press Escape and the line prints while
// the window stays exactly where it is. Press F for full screen and press it
// again there to watch the difference.
package main

import (
	"fmt"
	"log"

	"github.com/Quikcad/quikwin/pkg/window"
)

// stater is the window-state surface this exercises. Only the Cocoa backend
// carries Zoom today, so everywhere else runs the keyboard half and reports
// that the rest is unavailable rather than failing to start.
type stater interface {
	Zoom()
	IsZoomed() bool
	ToggleMaximize()
	IsMaximized() bool
	Minimize()
}

func main() {
	win, err := window.New(
		window.WithTitle("Window State Test"),
		window.WithSize(800, 500),
	)
	if err != nil {
		log.Fatal("create window:", err)
	}
	defer win.Destroy()

	st, ok := win.(stater)
	if !ok {
		fmt.Println("this backend exposes no window-state controls; keys will report only size")
	}

	fmt.Print(usage)

	report := func(tag string) {
		w, h := win.Size()
		if !ok {
			fmt.Printf("%-16s %4dx%-4d\n", tag, w, h)
			return
		}
		fmt.Printf("%-16s %4dx%-4d zoomed=%-5v fullscreen=%v\n", tag, w, h, st.IsZoomed(), st.IsMaximized())
	}

	quit := false
	win.OnClose(func() { quit = true })
	win.OnResize(func(uint32, uint32) { report("resized") })

	win.OnKey(func(key window.Key, action window.Action, _ window.Mod) {
		if action != window.Press {
			return
		}
		switch key {
		case window.KeyEscape:
			// The whole point: this line prints, and nothing else happens.
			report("ESCAPE — window stays as it is")
		case window.KeyZ:
			if !ok {
				return
			}
			st.Zoom()
			report("after Zoom")
		case window.KeyF:
			if !ok {
				return
			}
			st.ToggleMaximize()
			report("after ToggleMaximize")
		case window.KeyM:
			if !ok {
				return
			}
			st.Minimize()
		case window.KeyQ:
			quit = true
		}
	})

	report("initial")

	// Zoomed from the start: that is the state Escape is interesting in, and
	// leaving it to a keypress means the window opens in the one state that
	// proves nothing.
	if ok {
		st.Zoom()
		report("zoomed at start")
	}

	// WaitEvents rather than PollEvents: there is nothing to render, so the
	// process should be asleep between keystrokes rather than spinning.
	for !quit && !win.ShouldClose() {
		win.WaitEvents()
	}

	fmt.Println("closed")
}

const usage = `
  Z    zoom / restore   — fills the screen, stays a window
  F    full screen      — the green button's plain click
  M    minimize
  Esc  should print a line and change nothing
  Q    quit
`
