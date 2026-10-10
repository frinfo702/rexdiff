package diff_test

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/rednafi/rexdiff/diff"
)

// TestHostileContent renders each hostile file in all layouts. No control
// character from the file can reach the terminal, and the rows must point
// at real lines.
func TestHostileContent(t *testing.T) {
	opts := []diff.Options{
		{},
		{Width: 20},
		{Width: 20, Wrap: true},
		{Width: 9, Wrap: true},
		{Split: true, Width: 30},
		{Split: true, Width: 30, Wrap: true},
		{Width: 20, Offset: 3},
	}
	for _, s := range hostile {
		for _, o := range opts {
			d := diff.Render("x.go", "a\n", s, o)
			checkLines(t, d, o)
			checkRows(t, d.Rows, "a\n", s)
			f := diff.RenderFile("x.go", s, o)
			checkLines(t, f, o)
			checkRows(t, f.Rows, s, s)
			if t.Failed() {
				t.Fatalf("content %q, options %+v", s, o)
			}
		}
	}
}

// TestEscapesAreVisible checks that an escape in a file shows as a symbol,
// so that the reader sees it.
func TestEscapesAreVisible(t *testing.T) {
	tests := []struct{ src, want string }{
		{"\x1b]52;c;aGk=\x07\n", "␛]52;c;aGk=␇"},
		{"\x1b]7501;state=idle\x1b\\\n", "␛]7501;state=idle␛\\"},
		{"a\x00b\n", "a␀b"},
		{"\u009b31m\n", "\ufffd31m"},
		{"a\rb\n", "a␍b"},
	}
	for _, tt := range tests {
		d := diff.Render("x.txt", "", tt.src, diff.Options{})
		if got := ansi.Strip(d.Lines[1]); !strings.Contains(got, tt.want) {
			t.Errorf("Render(%q) = %q, want it to show %q", tt.src, got, tt.want)
		}
		f := diff.RenderFile("x.txt", tt.src, diff.Options{})
		if len(f.Lines) != 1 || !strings.Contains(ansi.Strip(f.Lines[0]), tt.want) {
			t.Errorf("RenderFile(%q) = %q, want one line that shows %q",
				tt.src, f.Lines, tt.want)
		}
	}
}

// TestWrapWideText checks that wrapped CJK and emoji text fits the width
// and that no character is lost.
func TestWrapWideText(t *testing.T) {
	line := strings.Repeat("日本語😀", 10)
	for _, o := range []diff.Options{
		{Width: 25, Wrap: true},
		{Split: true, Width: 41, Wrap: true},
	} {
		d := diff.Render("x.txt", "", line+"\n", o)
		checkLines(t, d, o)
		var text strings.Builder
		for i, l := range d.Lines {
			if d.Rows[i].New == 1 {
				text.WriteString(ansi.Strip(l))
			}
		}
		got := strings.Join(strings.Fields(text.String()), "")
		if !strings.Contains(got, line) {
			t.Errorf("split %v: the wrapped lines lose text: %q", o.Split, got)
		}
	}
}

// TestTabToSpacesIsAChange checks that a diff shows a change of a tab to
// spaces, as Count and git do.
func TestTabToSpacesIsAChange(t *testing.T) {
	d := diff.Render("x.go", "\tx\n", "    x\n", diff.Options{})
	if got := kinds(d.Rows); got != "@-+" {
		t.Errorf("kinds = %q, want %q", got, "@-+")
	}
}

func TestRenderFileLineCount(t *testing.T) {
	tests := []struct {
		src  string
		rows int
	}{
		{"", 0},
		{"\n", 1},
		{"\n\n\n", 3},
		{"a", 1},
		{"a\r\nb\r\n", 2},
		{"a\rb\rc", 1},
	}
	for _, tt := range tests {
		for _, path := range []string{"x.txt", "Makefile", "x.go"} {
			if d := diff.RenderFile(path, tt.src, diff.Options{}); len(d.Rows) != tt.rows {
				t.Errorf("RenderFile(%q, %q) has %d rows, want %d",
					path, tt.src, len(d.Rows), tt.rows)
			}
		}
	}
}

func TestRenderFileNarrow(t *testing.T) {
	for w := 1; w < 8; w++ {
		o := diff.Options{Width: w, Wrap: true}
		checkLines(t, diff.RenderFile("x.txt", "日本\n 0000\n", o), o)
	}
}

func TestQuote(t *testing.T) {
	tests := []struct{ in, want string }{
		{"a.go", "a.go"},
		{"日本.go", "日本.go"},
		{"a\x1b]0;x\x07.go", `"a\x1b]0;x\a.go"`},
		{"a\n.go", `"a\n.go"`},
		{"a\x9b31m.go", `"a\x9b31m.go"`},
	}
	for _, tt := range tests {
		if got := diff.Quote(tt.in); got != tt.want {
			t.Errorf("Quote(%q) = %s, want %s", tt.in, got, tt.want)
		}
	}
}
