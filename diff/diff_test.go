package diff_test

import (
	"fmt"
	"image/color"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/rednafi/rexdiff/diff"
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
		{Background: color.Black, Syntax: "monokai"},
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
	if got := ansi.Strip(d.Lines[1]); !strings.Contains(got, "+     x := 1") {
		t.Errorf("tab: line = %q", got)
	}
	d = diff.Render("x.go", "", "\tx := 1\n", diff.Options{Width: 80, Offset: 6})
	got := ansi.Strip(d.Lines[1])
	if strings.Contains(got, "x :=") || !strings.Contains(got, "= 1") {
		t.Errorf("offset 6: line = %q", got)
	}
}

func TestSideAt(t *testing.T) {
	// Unified: " 1 " old numbers, " 1 " new numbers, then the code.
	u := diff.Render("x.go", oldSrc, newSrc, diff.Options{Width: 80})
	// Split: old numbers, old code, new numbers, new code.
	split := diff.Options{Split: true, Width: 80}
	s := diff.Render("x.go", oldSrc, newSrc, split)
	// File: line numbers, then the code.
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
