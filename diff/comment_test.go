package diff_test

import (
	"slices"
	"testing"

	"github.com/rednafi/rexdiff/diff"
)

// comment returns a comment with text on all rows of the diff of old and
// new.
func comment(path string, s diff.Side, old, new, text string) diff.Comment {
	rows := diff.Render(path, old, new, diff.Options{}).Rows
	c := diff.NewComment(path, s, rows, old, new)
	c.Text = text
	return c
}

// commentOn returns a comment on lines start to end of src, the file at
// path. The rows come from [diff.RenderFile].
func commentOn(path string, s diff.Side, src string, start, end int) diff.Comment {
	rows := diff.RenderFile(path, src, diff.Options{}).Rows
	return diff.NewComment(path, s, rows[start-1:end], src, src)
}

// review returns the text of [diff.Comments.Review] for c only.
func review(c diff.Comment) string {
	var cs diff.Comments
	cs.Add(c)
	return cs.Review("")
}

func TestComments(t *testing.T) {
	var cs diff.Comments
	a := cs.Add(diff.Comment{Path: "a.go", Side: diff.SideNew, Start: 2, End: 4})
	b := cs.Add(diff.Comment{Path: "a.go", Side: diff.SideOld, Start: 4, End: 4})
	cs.Add(diff.Comment{Path: "b.go", Side: diff.SideNew, Start: 1, End: 1})
	if cs.Len() != 3 || cs.Count("a.go") != 2 || a.ID == b.ID {
		t.Fatalf("Len = %d, Count(a.go) = %d, IDs %d %d",
			cs.Len(), cs.Count("a.go"), a.ID, b.ID)
	}
	if c := cs.At("a.go", diff.SideNew, 3); c != a {
		t.Errorf("At(a.go, New, 3) = %v", c)
	}
	if c := cs.At("a.go", diff.SideNew, 5); c != nil {
		t.Errorf("At(a.go, New, 5) = %v, want nil", c)
	}
	if got := cs.EndingAt("a.go", diff.SideOld, 4); len(got) != 1 || got[0] != b {
		t.Errorf("EndingAt(a.go, Old, 4) = %v", got)
	}
	if cs.Find(b.ID) != b {
		t.Error("Find did not find b")
	}
	cs.Remove(b.ID)
	if cs.Find(b.ID) != nil || cs.Len() != 2 {
		t.Error("Remove did not remove b")
	}
	if c := cs.Add(diff.Comment{}); c.ID == a.ID || c.ID == b.ID {
		t.Error("Add reused an ID")
	}
}

// TestNewComment makes comments from the rows of a change, in the two
// layouts, on each side.
func TestNewComment(t *testing.T) {
	old := "a\nb\nc\nd\n"
	new := "a\nB\nC\nd\n"
	moved := "x\n" // a line above the comment moves it down by one
	for _, split := range []bool{false, true} {
		rows := diff.Render("x.txt", old, new, diff.Options{Split: split}).Rows
		// The rows from "a" to "d".
		lo := slices.IndexFunc(rows, isEqual)
		for _, s := range []diff.Side{diff.SideNew, diff.SideOld} {
			c := diff.NewComment("x.txt", s, rows[lo:], old, new)
			if c.Path != "x.txt" || c.Side != s || c.Start != 1 || c.End != 4 {
				t.Errorf("split %v, side %d: %+v, want lines 1-4", split, s, c)
			}
			want := "x.txt:1-4"
			if s == diff.SideOld {
				want += " (removed)"
			}
			want += "\n a\n-b\n-c\n+B\n+C\n d\n"
			if got := review(c); got != want {
				t.Errorf("split %v, side %d: Review:\n%s\nwant:\n%s",
					split, s, got, want)
			}
			// The comment has the lines of its side: it follows them.
			var cs diff.Comments
			got := cs.Add(c)
			cs.Relocate("x.txt", moved+old, moved+new)
			if got.Start != 2 || got.End != 5 || got.Stale {
				t.Errorf("split %v, side %d: after a move %+v, want lines 2-5",
					split, s, got)
			}
		}
		// Only the inserted lines: the range is on the new side.
		var ins []diff.Row
		for _, r := range rows {
			if r.Kind == diff.KindInsert || r.Kind == diff.KindChange {
				ins = append(ins, r)
			}
		}
		var cs diff.Comments
		got := cs.Add(diff.NewComment("x.txt", diff.SideNew, ins, old, new))
		if got.Start != 2 || got.End != 3 {
			t.Errorf("split %v, the new lines: %+v, want lines 2-3", split, got)
		}
		if cs.Relocate("x.txt", "", "B\nC\n"); got.Start != 1 || got.Stale {
			t.Errorf("split %v, the new lines did not follow B and C: %+v", split, got)
		}
	}
	// A hunk header has no line.
	rows := diff.Render("x.txt", old, new, diff.Options{}).Rows
	hunk := rows[:slices.IndexFunc(rows, isEqual)]
	got := diff.NewComment("x.txt", diff.SideNew, hunk, old, new)
	if len(hunk) == 0 || got.Start != 0 || got.End != 0 {
		t.Errorf("a hunk header %v: %+v, want no lines", hunk, got)
	}
}

func TestRelocateFollowsMovedCode(t *testing.T) {
	var cs diff.Comments
	c := cs.Add(commentOn("a.go", diff.SideNew, "a\nb\nc\n", 2, 3))
	other := cs.Add(commentOn("a.go", diff.SideOld, "a\nb\n", 2, 2))

	// Each comment looks in the version of its side.
	cs.Relocate("a.go", "x\nx\nx\nb\n", "new\nnew\na\nb\nc\n")
	if c.Start != 4 || c.End != 5 || c.Stale {
		t.Errorf("moved: %+v", c)
	}
	if other.Start != 4 || other.Stale {
		t.Errorf("the comment on the old side did not follow the old version: %+v", other)
	}
	cs.Relocate("a.go", "b\n", "a\nB\nc\n")
	if !c.Stale || c.Start != 4 {
		t.Errorf("not stale: %+v", c)
	}
	cs.Relocate("a.go", "b\n", "b\nc\nx\nx\nx\nb\nc\n")
	if c.Stale || c.Start != 6 {
		t.Errorf("did not go to the nearest copy: %+v", c)
	}
	// The end of a file is not an empty line, and CRLF is the same as LF.
	blank := cs.Add(commentOn("b.go", diff.SideNew, "x\n\n", 2, 2))
	if cs.Relocate("b.go", "", "x\n"); !blank.Stale {
		t.Errorf("a comment on a deleted blank line is not stale: %+v", blank)
	}
	if cs.Relocate("b.go", "", "x\r\n\r\n"); blank.Stale || blank.Start != 2 {
		t.Errorf("a comment on a CRLF blank line: %+v", blank)
	}
}
