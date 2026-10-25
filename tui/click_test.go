package tui

import (
	"os"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// codeCell returns the screen cell of the first needle in the main pane.
func codeCell(t *testing.T, m model, needle string) (x, y int) {
	t.Helper()
	p := m.cur()
	for i, l := range p.lines {
		plain := ansi.Strip(l)
		if before, _, ok := strings.Cut(plain, needle); ok {
			return m.mainX() + gutter + ansi.StringWidth(before), screenY(t, m, i)
		}
	}
	t.Fatalf("no %q in the main pane", needle)
	return 0, 0
}

// clicks sends n clicks at x, y, and runs the command of the last click.
func clicks(t *testing.T, m model, n, x, y int) model {
	t.Helper()
	for range n - 1 {
		m = click(t, m, x, y)
	}
	next, cmd := m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: y})
	if m = *next.(*model); cmd != nil {
		m, _ = run(t, m, cmd)
	}
	return step(t, m, tea.MouseReleaseMsg{Button: tea.MouseLeft, X: x, Y: y})
}

// TestMultiClickCopies checks that a double click copies the word, a
// triple click copies the source line, and a double click on the file list
// copies the path.
func TestMultiClickCopies(t *testing.T) {
	tests := []struct {
		name, needle string
		n            int
		want         string
	}{
		{"double", "return", 2, "return"},
		{"double on one letter", "x := 2", 2, "x"},
		{"triple", "return x", 3, "\treturn x"},
		{"four", "return x", 4, "\treturn x"},
		{"double on a symbol", ":= 2", 2, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := fakeClipboard(t, false)
			m, _ := newTestModel(t)
			x, y := codeCell(t, m, tt.needle)
			m = clicks(t, m, tt.n, x, y)
			got, _ := os.ReadFile(out)
			if string(got) != tt.want {
				t.Errorf("clipboard %q, want %q", got, tt.want)
			}
			if tt.want != "" && m.status != "copied "+plural(len([]rune(tt.want)), "char") {
				t.Errorf("status %q, want the count of chars", m.status)
			}
		})
	}

	t.Run("path", func(t *testing.T) {
		out := fakeClipboard(t, false)
		m, _ := newTestModel(t)
		y := m.listY() + 1 + slices.Index(rowNames(m), "a.go")
		m = clicks(t, m, 2, m.listX()+3, y)
		if got, _ := os.ReadFile(out); string(got) != "a.go" || m.status != "copied 4 chars" {
			t.Errorf("clipboard %q, status %q; want a.go, copied 4 chars", got, m.status)
		}
	})
}

// TestClickChain checks that the clicks count only on the same cell and
// within the window.
func TestClickChain(t *testing.T) {
	m, _ := newTestModel(t)
	clk := setClock(&m)
	at := tea.Mouse{X: 40, Y: 10}
	for i, want := range []int{1, 2, 3, 3} {
		if n := m.countClick(at); n != want {
			t.Errorf("press %d: count %d, want %d", i+1, n, want)
		}
	}
	if n := m.countClick(tea.Mouse{X: 41, Y: 10}); n != 1 {
		t.Errorf("another cell: count %d, want 1", n)
	}
	clk.advance(clickWindow)
	if n := m.countClick(tea.Mouse{X: 41, Y: 10}); n != 1 {
		t.Errorf("after the window: count %d, want 1", n)
	}
	m.countClick(at)
	m = press(t, m, "j")
	if n := m.countClick(at); n != 1 {
		t.Errorf("after a key: count %d, want 1", n)
	}
}
