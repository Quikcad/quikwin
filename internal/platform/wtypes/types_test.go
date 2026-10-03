package wtypes

import "testing"

func TestDetectEdge(t *testing.T) {
	const w, h, b = 100.0, 80.0, 5.0
	tests := []struct {
		x, y float64
		want ResizeEdge
	}{
		{50, 40, EdgeNone},        // center
		{50, 1, EdgeTop},          // top center
		{50, 79, EdgeBottom},      // bottom center
		{1, 40, EdgeLeft},         // left center
		{99, 40, EdgeRight},       // right center
		{1, 1, EdgeTopLeft},       // top-left corner
		{99, 1, EdgeTopRight},     // top-right corner
		{1, 79, EdgeBottomLeft},   // bottom-left corner
		{99, 79, EdgeBottomRight}, // bottom-right corner
		{5, 40, EdgeNone},         // just inside left edge
		{4.9, 40, EdgeLeft},       // just on left edge
	}
	for _, tt := range tests {
		got := DetectEdge(tt.x, tt.y, w, h, b)
		if got != tt.want {
			t.Errorf("DetectEdge(%v, %v, %v, %v, %v) = %d, want %d", tt.x, tt.y, w, h, b, got, tt.want)
		}
	}
}

// A negative side would claim a window larger than the surface it is painted
// in. xdg_surface rejects a geometry that does not intersect its surface and
// the protocol error takes the connection with it, so caller arithmetic that
// has gone wrong has to stop here rather than on the wire.
func TestANegativeInsetIsNoInset(t *testing.T) {
	got := FrameInset{Left: -4, Top: 19, Right: -0.5, Bottom: 22}.Normalized()
	want := FrameInset{Left: 0, Top: 19, Right: 0, Bottom: 22}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestAnInsetTakesBothSidesOffEachAxis(t *testing.T) {
	in := FrameInset{Left: 4, Top: 2, Right: 8, Bottom: 16}
	if got := in.Horizontal(); got != 12 {
		t.Errorf("horizontal is %v, want 12", got)
	}
	if got := in.Vertical(); got != 18 {
		t.Errorf("vertical is %v, want 18", got)
	}
}

func TestEdgeCursorShape(t *testing.T) {
	tests := []struct {
		edge ResizeEdge
		want CursorShape
	}{
		{EdgeNone, CursorArrow},
		{EdgeTop, CursorVResize},
		{EdgeBottom, CursorVResize},
		{EdgeLeft, CursorHResize},
		{EdgeRight, CursorHResize},
		{EdgeTopLeft, CursorNWSEResize},
		{EdgeBottomRight, CursorNWSEResize},
		{EdgeTopRight, CursorNESWResize},
		{EdgeBottomLeft, CursorNESWResize},
	}
	for _, tt := range tests {
		got := EdgeCursorShape(tt.edge)
		if got != tt.want {
			t.Errorf("EdgeCursorShape(%d) = %d, want %d", tt.edge, got, tt.want)
		}
	}
}
