package diff

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
)

// Comment is a review note on a range of lines on one side of a file. Use
// [NewComment] to make one. A comment keeps the text of its lines, for
// [Comments.Relocate], and its diff rows, for [Comments.Review].
type Comment struct {
	ID   int // set by [Comments.Add]
	Path string
	Side Side
	// Start and End are the first and the last line of the range. They
	// start at 1.
	Start, End int
	Text       string
	// Stale is true if Relocate cannot find the lines of the comment.
	Stale bool

	code    []string // the text of the lines Start to End
	snippet []string // the rows as unified diff lines; if empty, Review uses code
}

// NewComment returns a comment with no text on side s of the file at path.
// Rows are rows of the [Diff] of old and new. Start and End are the first and
// the last line of the rows on side s. They are 0 if no row has a line on s.
//
// In the diff rows of the comment, equal, deleted, and inserted lines start
// with " ", "-", and "+". The text comes from old and new, so tabs stay. In
// each block of changes, deleted lines come before inserted lines, also in
// the split layout. Continued rows add nothing.
func NewComment(path string, s Side, rows []Row, old, new string) Comment {
	c := Comment{Path: path, Side: s, snippet: snippet(rows, old, new)}
	for _, r := range rows {
		n := r.Line(s)
		if n == 0 {
			continue
		}
		if c.Start == 0 || n < c.Start {
			c.Start = n
		}
		c.End = max(c.End, n)
	}
	if c.Start == 0 {
		return c
	}
	lines := Lines(version(s, old, new))
	c.code = slices.Clone(lines[min(c.Start-1, len(lines)):min(c.End, len(lines))])
	return c
}

// Location returns the path and the lines of c, for example "a.go:3-4".
// A comment on the old side has " (removed)" at the end. The path is
// quoted with [Quote].
func (c *Comment) Location() string {
	loc := fmt.Sprintf("%s:%d", Quote(c.Path), c.Start)
	if c.End != c.Start {
		loc += fmt.Sprintf("-%d", c.End)
	}
	if c.Side == SideOld {
		loc += " (removed)"
	}
	return loc
}

// Comments is the set of comments of a review. The zero value is an empty
// set that is ready to use.
type Comments struct {
	last  int
	items []*Comment
}

// Len returns the number of comments.
func (s *Comments) Len() int { return len(s.items) }

// Add adds a copy of c with a new ID, and returns the copy.
func (s *Comments) Add(c Comment) *Comment {
	s.last++
	c.ID = s.last
	s.items = append(s.items, &c)
	return &c
}

// Remove removes the comment with the given ID, if there is one.
func (s *Comments) Remove(id int) {
	s.items = slices.DeleteFunc(s.items, func(c *Comment) bool { return c.ID == id })
}

// All returns the comments, sorted by path and then by start line.
func (s *Comments) All() []*Comment {
	cs := slices.Clone(s.items)
	slices.SortStableFunc(cs, func(a, b *Comment) int {
		return cmp.Or(strings.Compare(a.Path, b.Path), cmp.Compare(a.Start, b.Start))
	})
	return cs
}

// Find returns the comment with the given ID, or nil.
func (s *Comments) Find(id int) *Comment {
	for _, c := range s.items {
		if c.ID == id {
			return c
		}
	}
	return nil
}

// At returns the first comment whose range includes line, or nil.
func (s *Comments) At(path string, side Side, line int) *Comment {
	for _, c := range s.items {
		if c.Path == path && c.Side == side && line >= c.Start && line <= c.End {
			return c
		}
	}
	return nil
}

// EndingAt returns the comments whose range ends on line.
func (s *Comments) EndingAt(path string, side Side, line int) []*Comment {
	var cs []*Comment
	for _, c := range s.items {
		if c.Path == path && c.Side == side && c.End == line {
			cs = append(cs, c)
		}
	}
	return cs
}

// Count returns the number of comments on the file at path.
func (s *Comments) Count(path string) int {
	n := 0
	for _, c := range s.items {
		if c.Path == path {
			n++
		}
	}
	return n
}

// Relocate moves each comment on path to the lines that have its text.
// Old and new are the two versions of the file. A comment on the [SideOld]
// side looks in old, and a comment on the [SideNew] side looks in new. If
// the text is at more than one location, the comment moves to the nearest
// one. If the version does not have the text, the comment stays and
// becomes stale.
func (s *Comments) Relocate(path, old, new string) {
	var lines [2][]string // the lines of each side; split when a comment needs them
	for _, c := range s.items {
		if c.Path != path || len(c.code) == 0 {
			continue
		}
		if lines[c.Side] == nil {
			lines[c.Side] = Lines(version(c.Side, old, new))
		}
		c.relocate(lines[c.Side])
	}
}

// version returns old for the [SideOld] side, and new for the [SideNew] side.
func version(s Side, old, new string) string {
	if s == SideOld {
		return old
	}
	return new
}

// relocate moves c to the location of its code in lines that is nearest to
// Start. If lines does not have the code, c becomes stale.
func (c *Comment) relocate(lines []string) {
	if matchAt(lines, c.code, c.Start-1) {
		c.Stale = false
		return
	}
	best, bestDist := -1, 0
	for i := range len(lines) - len(c.code) + 1 {
		d := abs(i - (c.Start - 1))
		nearer := best < 0 || d < bestDist
		if nearer && matchAt(lines, c.code, i) {
			best, bestDist = i, d
		}
	}
	if best < 0 {
		c.Stale = true
		return
	}
	n := c.End - c.Start
	c.Start, c.End, c.Stale = best+1, best+1+n, false
}

// matchAt reports whether lines has want at index at.
func matchAt(lines, want []string, at int) bool {
	end := at + len(want)
	return at >= 0 && end <= len(lines) && slices.Equal(lines[at:end], want)
}

// abs returns the absolute value of x.
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
