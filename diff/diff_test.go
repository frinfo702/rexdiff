package diff_test

import (
	"fmt"
	"image/color"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/frinfo702/rexdiff/diff"
)

const (
	oldSrc = "a\nb\nc\n"
	newSrc = "a\nB\nc\nd\n"
)

// snippet returns the diff lines of a comment on rows, as
// [diff.Comments.Review] writes them.
func snippet(rows []diff.Row, old, new string) []string {
	var cs diff.Comments
	cs.Add(diff.NewComment("x", diff.SideNew, rows, old, new))
	lines := strings.Split(cs.Review(""), "\n")
	return lines[1 : len(lines)-1] // not the location and the empty text
}

// isEqual reports whether r is a [diff.KindEqual] row.
func isEqual(r diff.Row) bool { return r.Kind == diff.KindEqual }

// kinds writes the rows as one character each, for example "@=-+=+".
func kinds(rows []diff.Row) string {
	var b strings.Builder
	for _, r := range rows {
		b.WriteByte("@=+-~"[r.Kind])
	}
	return b.String()
}

func TestRenderRowsFollowLines(t *testing.T) {
	const width = 80
	themes := []diff.Theme{
		{},
		{Background: color.White, Foreground: color.Black},
		{Background: color.Black, Foreground: color.White},
	}
	tests := []struct {
		split bool
		kinds string
		rows  []diff.Row
	}{
		{
			split: false,
			kinds: "@=-+=+",
			rows: []diff.Row{
				{Kind: diff.KindHunk},
				{Kind: diff.KindEqual, Old: 1, New: 1},
				{Kind: diff.KindDelete, Old: 2},
				{Kind: diff.KindInsert, New: 2},
				{Kind: diff.KindEqual, Old: 3, New: 3},
				{Kind: diff.KindInsert, New: 4},
			},
		},
		{
			split: true,
			kinds: "@=~=+",
			rows: []diff.Row{
				{Kind: diff.KindHunk},
				{Kind: diff.KindEqual, Old: 1, New: 1},
				{Kind: diff.KindChange, Old: 2, New: 2},
				{Kind: diff.KindEqual, Old: 3, New: 3},
				{Kind: diff.KindInsert, New: 4},
			},
		},
	}
	for ti, th := range themes {
		for _, tt := range tests {
			name := fmt.Sprintf("split %v, theme %d", tt.split, ti)
			o := diff.Options{Split: tt.split, Width: width, Theme: th}
			d := diff.Render("x.go", oldSrc, newSrc, o)
			if got := kinds(d.Rows); got != tt.kinds {
				t.Errorf("%s: kinds = %q, want %q", name, got, tt.kinds)
			}
			for i, r := range tt.rows {
				if i < len(d.Rows) && d.Rows[i] != r {
					t.Errorf("%s: row %d = %+v, want %+v", name, i, d.Rows[i], r)
				}
			}
			if len(d.Lines) != len(d.Rows) {
				t.Fatalf("%s: %d lines for %d rows", name, len(d.Lines), len(d.Rows))
			}
			for i, l := range d.Lines {
				if w := ansi.StringWidth(l); w > width {
					t.Errorf("%s: line %d is %d cells wide", name, i, w)
				}
			}
		}
	}
}

func TestRenderSameContentIsEmpty(t *testing.T) {
	d := diff.Render("x.go", oldSrc, oldSrc, diff.Options{Width: 80})
	if len(d.Lines) != 0 || len(d.Rows) != 0 {
		t.Fatalf("got %d lines and %d rows, want none", len(d.Lines), len(d.Rows))
	}
}

func TestRenderTabsAndOffset(t *testing.T) {
	d := diff.Render("x.go", "", "\tx := 1\n", diff.Options{Width: 80})
	if got := ansi.Strip(d.Lines[1]); !strings.Contains(got, "▌     x := 1") {
		t.Errorf("tab: line = %q", got)
	}
	d = diff.Render("x.go", "", "\tx := 1\n", diff.Options{Width: 80, Offset: 6})
	got := ansi.Strip(d.Lines[1])
	if strings.Contains(got, "x :=") || !strings.Contains(got, "= 1") {
		t.Errorf("offset 6: line = %q", got)
	}
}

// TestMarks checks the mark before the code of each line, in the two
// layouts. By default a bar marks a changed line, as in herdr-reviewr.
// [diff.Options.Signs] gives + and -. The mark takes two cells in both
// cases, so the columns do not move.
func TestMarks(t *testing.T) {
	const old, new = "a\nb\nc\n", "a\nB\nc\n"
	theme := diff.Theme{
		Background: color.Black,
		Green:      color.RGBA{0, 0xff, 0, 0xff},
		Red:        color.RGBA{0xff, 0, 0, 0xff},
	}
	tests := []struct {
		name  string
		o     diff.Options
		lines []string // the screen lines under the hunk header, without colours
	}{
		{"unified bar", diff.Options{Width: 40}, []string{
			" 1  1   a", " 2    ▌ b", "    2 ▌ B", " 3  3   c",
		}},
		{"unified signs", diff.Options{Width: 40, Signs: true}, []string{
			" 1  1   a", " 2    - b", "    2 + B", " 3  3   c",
		}},
		{"split bar", diff.Options{Width: 40, Split: true}, []string{
			" 1   a               1   a",
			" 2 ▌ b               2 ▌ B",
			" 3   c               3   c",
		}},
		{"split signs", diff.Options{Width: 40, Split: true, Signs: true}, []string{
			" 1   a               1   a",
			" 2 - b               2 + B",
			" 3   c               3   c",
		}},
	}
	for _, tt := range tests {
		tt.o.Theme = theme
		d := diff.Render("x.txt", old, new, tt.o)
		if len(d.Lines) != 1+len(tt.lines) {
			t.Fatalf("%s: %d lines, want %d", tt.name, len(d.Lines), 1+len(tt.lines))
		}
		for i, want := range tt.lines {
			got := strings.TrimRight(ansi.Strip(d.Lines[i+1]), " ")
			if got != want {
				t.Errorf("%s: line %d = %q, want %q", tt.name, i+1, got, want)
			}
		}
		// The mark has the colour of the line: ANSI red for a delete, and
		// ANSI green for an insert.
		for _, c := range []struct {
			line int
			fg   string
		}{{2, "31"}, {3, "32"}} {
			if tt.o.Split && c.line == 3 {
				continue // the split layout has the insert on the same row
			}
			if got := markColour(d.Lines[c.line]); got != c.fg {
				t.Errorf("%s: the mark of line %d has colour %q, want %q", tt.name, c.line, got, c.fg)
			}
		}
	}
	// With a scroll to the right, the bar keeps its colour and shows an
	// ellipsis after it.
	d := diff.Render("x.txt", "", "abcdefgh\n", diff.Options{Width: 40, Offset: 2})
	if got := ansi.Strip(d.Lines[1]); !strings.Contains(got, "▌…cdefgh") {
		t.Errorf("offset: line = %q, want ▌…cdefgh", got)
	}
	// The review text of a comment uses + and - for both kinds of mark.
	for _, signs := range []bool{false, true} {
		d := diff.Render("x.txt", old, new, diff.Options{Width: 40, Signs: signs})
		got := snippet(d.Rows[2:5], old, new)
		if want := []string{"-b", "+B", " c"}; !slices.Equal(got, want) {
			t.Errorf("signs %v: snippet = %q, want %q", signs, got, want)
		}
	}
}

// markColour returns the foreground of the SGR sequence just before the
// first bar, + or - in line, for example "31".
func markColour(line string) string {
	i := strings.IndexAny(line, "▌+-")
	if i < 0 {
		return ""
	}
	sgr := line[strings.LastIndex(line[:i], "\x1b[")+2 : i]
	fg, _, _ := strings.Cut(strings.TrimSuffix(sgr, "m"), ";")
	return fg
}

func TestSideAt(t *testing.T) {
	// The unified layout has " 1 " old numbers, " 1 " new numbers, and the
	// code.
	u := diff.Render("x.go", oldSrc, newSrc, diff.Options{Width: 80})
	// The split layout has old numbers, old code, new numbers, and new code.
	split := diff.Options{Split: true, Width: 80}
	s := diff.Render("x.go", oldSrc, newSrc, split)
	// A file has line numbers, and then the code.
	f := diff.RenderFile("x.go", oldSrc, diff.Options{Width: 80})
	tests := []struct {
		name     string
		d        *diff.Diff
		x        int
		side     diff.Side
		onNumber bool
	}{
		{"unified old number", u, 0, diff.SideOld, true},
		{"unified new number", u, 4, diff.SideNew, true},
		{"unified code", u, 10, diff.SideNew, false},
		{"split old number", s, 1, diff.SideOld, true},
		{"split old code", s, 10, diff.SideOld, false},
		{"split new number", s, 41, diff.SideNew, true},
		{"split new code", s, 60, diff.SideNew, false},
		{"file number", f, 1, diff.SideNew, true},
		{"file code", f, 5, diff.SideNew, false},
	}
	for _, tt := range tests {
		side, on := tt.d.SideAt(tt.x)
		if side != tt.side || on != tt.onNumber {
			t.Errorf("%s: SideAt(%d) = %v, %v; want %v, %v",
				tt.name, tt.x, side, on, tt.side, tt.onNumber)
		}
	}
}

func TestCount(t *testing.T) {
	tests := []struct {
		old, new       string
		added, deleted int
	}{
		{oldSrc, newSrc, 2, 1},
		{"", "a\nb\n", 2, 0},
		{"a\r\nb\r\n", "a\nb\n", 0, 0},
		{oldSrc, oldSrc, 0, 0},
	}
	for _, tt := range tests {
		a, d := diff.Count(tt.old, tt.new)
		if a != tt.added || d != tt.deleted {
			t.Errorf("Count(%q, %q) = %d, %d; want %d, %d",
				tt.old, tt.new, a, d, tt.added, tt.deleted)
		}
	}
}

func TestRenderFile(t *testing.T) {
	src := "package x\n\nfunc f() {\n\treturn\n}\n"
	d := diff.RenderFile("x.go", src, diff.Options{Width: 14})
	if len(d.Lines) != 5 || len(d.Rows) != 5 {
		t.Fatalf("got %d lines and %d rows, want 5", len(d.Lines), len(d.Rows))
	}
	if d.Rows[3] != (diff.Row{Kind: diff.KindEqual, Old: 4, New: 4}) {
		t.Errorf("row 3 = %+v", d.Rows[3])
	}
	if got := ansi.Strip(d.Lines[2]); got != " 3  func f() {" {
		t.Errorf("line 2 = %q", got)
	}
	if got := ansi.Strip(d.Lines[0]); got != " 1  package x" {
		t.Errorf("line 0 = %q", got)
	}
	// Control characters must not reach the terminal.
	d = diff.RenderFile("x.txt", "a\x1b[31mb\n", diff.Options{})
	if strings.Contains(ansi.Strip(d.Lines[0]), "\x1b") {
		t.Errorf("line 0 = %q, has an escape", d.Lines[0])
	}
}

// TestWrap wraps a long line. All of its text shows, and each screen line
// fits the width. The continued rows point at the same source lines, and
// the snippet of a comment does not change.
func TestWrap(t *testing.T) {
	long := "x := []string{" + strings.Repeat(`"word", `, 12) + "}"
	old, new := "a\nb\n", "a\n"+long+"\nb\n"
	const width = 40
	renders := []struct {
		name   string
		render func(o diff.Options) *diff.Diff
	}{
		{"unified", func(o diff.Options) *diff.Diff {
			return diff.Render("x.go", old, new, o)
		}},
		{"split", func(o diff.Options) *diff.Diff {
			o.Split = true
			return diff.Render("x.go", old, new, o)
		}},
		{"file", func(o diff.Options) *diff.Diff {
			return diff.RenderFile("x.go", new, o)
		}},
	}
	for _, tt := range renders {
		name, render := tt.name, tt.render
		cut := render(diff.Options{Width: width})
		d := render(diff.Options{Width: width, Wrap: true})
		if len(d.Lines) != len(d.Rows) || len(d.Lines) <= len(cut.Lines) {
			t.Fatalf("%s: %d lines and %d rows, want more than the %d lines without wrap",
				name, len(d.Lines), len(d.Rows), len(cut.Lines))
		}
		var text strings.Builder
		for i, l := range d.Lines {
			if w := ansi.StringWidth(l); w > width {
				t.Errorf("%s: line %d is %d cells wide", name, i, w)
			}
			r := d.Rows[i]
			if r.New == 2 {
				text.WriteString(ansi.Strip(l))
			}
			prev := d.Rows[max(i-1, 0)]
			sameLines := prev.Old == r.Old && prev.New == r.New
			if r.Continued && !sameLines {
				t.Errorf("%s: continued row %d = %+v, the row above = %+v",
					name, i, r, prev)
			}
		}
		noSpace := func(s string) string { return strings.Join(strings.Fields(s), "") }
		if !strings.Contains(noSpace(text.String()), noSpace(long)) {
			t.Errorf("%s: the wrapped lines do not show all of the line:\n%s",
				name, text.String())
		}
		got, want := snippet(d.Rows, old, new), snippet(cut.Rows, old, new)
		if !slices.Equal(got, want) {
			t.Errorf("%s: snippet with wrap %q, without %q", name, got, want)
		}
	}
}
