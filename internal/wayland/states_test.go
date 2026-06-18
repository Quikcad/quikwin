//go:build linux

package wayland

import (
	"encoding/binary"
	"testing"

	xdg "github.com/lukem570/wayland-go/pkg/protocols/stable/xdgshell"
)

func packStates(states ...xdg.ToplevelState) []byte {
	b := make([]byte, 4*len(states))
	for i, s := range states {
		binary.LittleEndian.PutUint32(b[4*i:], uint32(s))
	}
	return b
}

func TestTiledStateDetection(t *testing.T) {
	tiled := func(states []byte) bool {
		return statesContainAny(states,
			xdg.ToplevelStateTiledLeft,
			xdg.ToplevelStateTiledRight,
			xdg.ToplevelStateTiledTop,
			xdg.ToplevelStateTiledBottom,
		)
	}

	cases := []struct {
		name      string
		states    []byte
		wantTiled bool
		wantMax   bool
	}{
		{"empty", nil, false, false},
		{"floating-activated", packStates(xdg.ToplevelStateActivated), false, false},
		{"maximized-only", packStates(xdg.ToplevelStateMaximized, xdg.ToplevelStateActivated), false, true},
		{"quick-tile-left", packStates(xdg.ToplevelStateTiledLeft, xdg.ToplevelStateTiledTop, xdg.ToplevelStateTiledBottom, xdg.ToplevelStateActivated), true, false},
		{"quick-tile-right", packStates(xdg.ToplevelStateTiledRight, xdg.ToplevelStateActivated), true, false},
		{"maximized-and-tiled", packStates(xdg.ToplevelStateMaximized, xdg.ToplevelStateTiledLeft), true, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := tiled(c.states); got != c.wantTiled {
				t.Errorf("tiled = %v, want %v", got, c.wantTiled)
			}
			if got := statesContain(c.states, xdg.ToplevelStateMaximized); got != c.wantMax {
				t.Errorf("maximized = %v, want %v", got, c.wantMax)
			}
		})
	}
}

// A truncated trailing word (not a multiple of 4 bytes) must not panic and must
// be ignored — guards the configure-array parser against a short read.
func TestStatesContainTruncated(t *testing.T) {
	b := append(packStates(xdg.ToplevelStateTiledLeft), 0x01, 0x02)
	if !statesContainAny(b, xdg.ToplevelStateTiledLeft) {
		t.Fatal("expected tiled_left in leading word")
	}
}
