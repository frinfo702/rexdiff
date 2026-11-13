package tui

import "testing"

func TestScrollTop(t *testing.T) {
	tests := []struct {
		name              string
		top, sel, rows, n int
		want              int
	}{
		{"inside the window: keep", 30, 40, 20, 100, 30},
		{"move up inside the window: keep", 31, 49, 20, 100, 31},
		{"below the window: scroll down", 0, 25, 20, 100, 6},
		{"above the window: scroll up", 30, 10, 20, 100, 10},
		{"the list got shorter", 80, 15, 20, 30, 10},
		{"shorter than the window", 5, 3, 20, 10, 0},
		{"no room", 0, 4, 0, 10, 4},
	}
	for _, tt := range tests {
		if got := scrollTop(tt.top, tt.sel, tt.rows, tt.n); got != tt.want {
			t.Errorf("%s: scrollTop(%d, %d, %d, %d) = %d, want %d",
				tt.name, tt.top, tt.sel, tt.rows, tt.n, got, tt.want)
		}
	}
}
