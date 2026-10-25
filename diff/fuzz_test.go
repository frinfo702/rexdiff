package diff_test

import (
	"image/color"
	"regexp"
	"slices"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"

	"github.com/rednafi/rexdiff/diff"
)

// paths select the lexers that the fuzz tests use.
var paths = []string{"x.go", "x.py", "x.txt", "x.md", "x.html", "x.json", "Makefile", "x"}

// hostile is file content that can attack a terminal or break the layout.
var hostile = []string{
	"",
	"\n",
	"\n\n\n",
	"a\r\nb\r\n",
	"a\rb\n",
	"\tx := 1\n\t\ty\n",
	"a\x00b\n",
	"\x1b[31mred\x1b[0m\n",
	"\x1b]52;c;aGVsbG8=\x07\n",
	"\x1b]7501;state=idle\x1b\\\n",
	"\u009b31m\u009d0;title\u009c\n",
	"\x9b31m\xff\xfe\n",
	"日本語のテキスト😀👨‍👩‍👧\n",
	"a\u0301\u200d\u202e\n",
	strings.Repeat("long ", 200) + "\n",
	"no newline at end",
}

// sgr matches the colour sequences that the renderer writes.
var sgr = regexp.MustCompile(`\x1b\[[0-9;:]*m`)

// isUnsafeControl reports whether r is a control character other than a
// tab or a newline.
func isUnsafeControl(r rune) bool {
	return r != '\t' && r != '\n' && unicode.IsControl(r)
}

// fuzzOptions makes Options from the fuzz values.
func fuzzOptions(layout, width, offset uint8, colored bool) diff.Options {
	o := diff.Options{
		Split:  layout&1 != 0,
		Wrap:   layout&2 != 0,
		Width:  int(width % 60),
		Offset: int(offset % 20),
	}
	if colored {
		o.Theme = diff.Theme{
			Background: color.Black,
			Foreground: color.White,
		}
	}
	return o
}

// checkLines checks the invariants that all rendered output must keep.
func checkLines(t *testing.T, d *diff.Diff, o diff.Options) {
	t.Helper()
	if len(d.Lines) != len(d.Rows) {
		t.Fatalf("%d lines for %d rows", len(d.Lines), len(d.Rows))
	}
	for i, l := range d.Lines {
		if w := ansi.StringWidth(l); o.Width > 0 && w > o.Width {
			t.Errorf("line %d is %d cells wide, width is %d: %q", i, w, o.Width, l)
		}
		rest := sgr.ReplaceAllString(l, "")
		if strings.ContainsFunc(rest, unicode.IsControl) {
			t.Errorf("line %d has a control character: %q", i, l)
		}
	}
}

// checkRows checks that each row points at real lines, in order.
func checkRows(t *testing.T, rows []diff.Row, old, new string) {
	t.Helper()
	nOld, nNew := len(diff.Lines(old)), len(diff.Lines(new))
	lastOld, lastNew := 0, 0
	for i, r := range rows {
		if r.Old < 0 || r.Old > nOld || r.New < 0 || r.New > nNew {
			t.Fatalf("row %d = %+v, the files have %d and %d lines", i, r, nOld, nNew)
		}
		if r.Continued {
			if i == 0 || rows[i-1].Old != r.Old || rows[i-1].New != r.New {
				t.Fatalf("continued row %d = %+v does not continue the row above", i, r)
			}
			continue
		}
		oldBack := r.Old != 0 && r.Old <= lastOld
		newBack := r.New != 0 && r.New <= lastNew
		if oldBack || newBack {
			t.Fatalf("row %d = %+v is not after old line %d and new line %d",
				i, r, lastOld, lastNew)
		}
		lastOld, lastNew = max(lastOld, r.Old), max(lastNew, r.New)
	}
}

// FuzzRender checks the invariants of Render, and that Count agrees with the
// snippet.
func FuzzRender(f *testing.F) {
	for i, s := range hostile {
		f.Add(uint8(i), "a\nb\n", s, uint8(i), uint8(i*7), uint8(i%3), i%2 == 0)
		f.Add(uint8(i), s, "a\n"+s+"b\n", uint8(i+1), uint8(i*5), uint8(0), i%2 == 1)
	}
	f.Add(uint8(0), oldSrc, newSrc, uint8(3), uint8(1), uint8(0), false)
	f.Add(uint8(0), "\tx\n", "    x\n", uint8(0), uint8(0), uint8(0), false)
	f.Fuzz(func(
		t *testing.T,
		p uint8,
		old, new string,
		layout, width, offset uint8,
		colored bool,
	) {
		o := fuzzOptions(layout, width, offset, colored)
		d := diff.Render(paths[int(p)%len(paths)], old, new, o)
		checkLines(t, d, o)
		checkRows(t, d.Rows, old, new)
		if o.Width == 0 && o.Offset == 0 && !strings.Contains(old+new, "…") {
			// With no width limit, no line is cut.
			for i, l := range d.Lines {
				cut := strings.Contains(ansi.Strip(l), "…")
				if cut && d.Rows[i].Kind != diff.KindHunk {
					t.Fatalf("line %d is cut with no width limit: %q", i, l)
				}
			}
		}

		added, deleted := diff.Count(old, new)
		if (added == 0 && deleted == 0) != (len(d.Lines) == 0) {
			t.Fatalf("Count = %d, %d, but Render has %d lines",
				added, deleted, len(d.Lines))
		}
		var plus, minus int
		for _, l := range snippet(d.Rows, old, new) {
			switch l[0] {
			case '+':
				plus++
			case '-':
				minus++
			}
		}
		if plus != added || minus != deleted {
			t.Fatalf("the snippet has +%d -%d, Count = +%d -%d",
				plus, minus, added, deleted)
		}
	})
}

// FuzzSnippetSameInLayouts checks that the split and the unified layouts give
// the same snippet.
func FuzzSnippetSameInLayouts(f *testing.F) {
	f.Add(oldSrc, newSrc)
	f.Add("a\nb\nc\nd\n", "x\nb\ny\nz\nd\n")
	f.Add("\tx\r\n", "\ty\n")
	f.Fuzz(func(t *testing.T, old, new string) {
		u := diff.Render("x.txt", old, new, diff.Options{})
		o := diff.Options{Split: true, Width: 30, Wrap: true}
		s := diff.Render("x.txt", old, new, o)
		got, want := snippet(s.Rows, old, new), snippet(u.Rows, old, new)
		if !slices.Equal(got, want) {
			t.Fatalf("split snippet %q, unified snippet %q", got, want)
		}
	})
}

// FuzzRenderFile checks that RenderFile gives one row for each source line.
func FuzzRenderFile(f *testing.F) {
	for i, s := range hostile {
		f.Add(uint8(i), s, uint8(i), uint8(i*7), uint8(i%3), i%2 == 0)
	}
	src := "package x\n\nfunc f() {\n\treturn\n}\n"
	f.Add(uint8(0), src, uint8(2), uint8(14), uint8(0), false)
	f.Fuzz(func(
		t *testing.T,
		p uint8,
		src string,
		layout, width, offset uint8,
		colored bool,
	) {
		o := fuzzOptions(layout, width, offset, colored)
		d := diff.RenderFile(paths[int(p)%len(paths)], src, o)
		checkLines(t, d, o)
		checkRows(t, d.Rows, src, src)
		n := 0
		for _, r := range d.Rows {
			if !r.Continued {
				n++
			}
		}
		if want := len(diff.Lines(src)); n != want {
			t.Fatalf("%d rows for a file of %d lines", n, want)
		}
	})
}

// FuzzPlain checks that Plain and Quote remove all unsafe control characters.
func FuzzPlain(f *testing.F) {
	for _, s := range hostile {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		p := diff.Plain(s)
		if strings.ContainsFunc(p, isUnsafeControl) {
			t.Errorf("Plain(%q) = %q, has a control character", s, p)
		}
		if pp := diff.Plain(p); pp != p {
			t.Errorf("Plain is not idempotent: %q, then %q", p, pp)
		}
		q := diff.Quote(s)
		if strings.ContainsFunc(q, unicode.IsControl) || !utf8.ValidString(q) {
			t.Errorf("Quote(%q) = %q, has a control character", s, q)
		}
	})
}

// FuzzCount checks that the counts of Count are in the range of the line
// counts.
func FuzzCount(f *testing.F) {
	for _, s := range hostile {
		f.Add(s, "a\n")
	}
	f.Fuzz(func(t *testing.T, a, b string) {
		if added, deleted := diff.Count(a, a); added != 0 || deleted != 0 {
			t.Errorf("Count(s, s) = %d, %d", added, deleted)
		}
		na, nb := len(diff.Lines(a)), len(diff.Lines(b))
		if added, deleted := diff.Count("", a); added != na || deleted != 0 {
			t.Errorf("Count(\"\", %q) = %d, %d; want %d, 0", a, added, deleted, na)
		}
		if added, deleted := diff.Count(a, ""); added != 0 || deleted != na {
			t.Errorf("Count(%q, \"\") = %d, %d; want 0, %d", a, added, deleted, na)
		}
		added, deleted := diff.Count(a, b)
		if added < 0 || deleted < 0 || added > nb || deleted > na {
			t.Errorf("Count(%q, %q) = %d, %d", a, b, added, deleted)
		}
	})
}

// FuzzComments checks that Relocate keeps a valid range, and that the review
// text has no control characters.
func FuzzComments(f *testing.F) {
	for i, s := range hostile {
		f.Add("a\nb\nc\n", s, "a.go", s, uint8(i), uint8(i%3))
	}
	f.Add(
		"a\nb\nc\n",
		"x\na\nb\nc\n",
		"a\x1b]52;c;aGk=\x07.go",
		"ok\x1b]7501;x\x07",
		uint8(1),
		uint8(1),
	)
	f.Fuzz(func(t *testing.T, before, after, path, text string, start, size uint8) {
		lines := diff.Lines(before)
		if len(lines) == 0 {
			return
		}
		s := int(start) % len(lines)
		e := min(s+int(size), len(lines)-1)
		var cs diff.Comments
		code := lines[s : e+1]
		c := cs.Add(commentOn(path, diff.SideNew, before, s+1, e+1))
		c.Text = text
		cs.Add(comment(path, diff.SideOld, before, after, text))

		cs.Relocate(path, before, after)
		n := len(diff.Lines(after))
		badRange := c.Start < 1 || c.Start > c.End
		if badRange || (!c.Stale && c.End > n) {
			t.Errorf("after Relocate: %+v, the file has %d lines", c, n)
		}
		if !c.Stale && !slices.Equal(diff.Lines(after)[c.Start-1:c.End], code) {
			t.Errorf("after Relocate: lines %d-%d are not the code", c.Start, c.End)
		}
		r := cs.Review(text)
		if strings.ContainsFunc(r, isUnsafeControl) {
			t.Errorf("Review has a control character: %q", r)
		}
		for _, c := range cs.All() {
			if l := c.Location(); strings.ContainsFunc(l, unicode.IsControl) {
				t.Errorf("Location = %q, has a control character", l)
			}
		}
	})
}
