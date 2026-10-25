package diff

import (
	"cmp"
	"encoding/json/v2"
	"fmt"
	"slices"
	"strings"

	"github.com/rednafi/rexdiff/internal/termtext"
)

// Comment is a review note on a range of lines on one side of a file. Use
// [NewComment] to make one. A comment keeps the text of its lines, for
// [Comments.Relocate], and its diff rows, for [Comments.Review].
type Comment struct {
	ID   int // [Comments.Add] sets it
	Path string
	Side Side
	// Start and End are the first and the last line of the range. They
	// start at 1.
	Start, End int
	Text       string
	// Stale is true if Relocate cannot find the lines of the comment.
	Stale bool

	code    []string // the text of the lines Start to End
	snippet []string // the rows as unified diff lines, or empty to use code
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
// A comment on the old side has " (removed)" at the end. [Quote] quotes
// the path.
func (c *Comment) Location() string {
	loc := fmt.Sprintf("%s:%d", termtext.Quote(c.Path), c.Start)
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
func (cs *Comments) Len() int { return len(cs.items) }

// Add adds c with a new ID, and returns the comment in the set.
func (cs *Comments) Add(c Comment) *Comment {
	cs.last++
	c.ID = cs.last
	cs.items = append(cs.items, &c)
	return &c
}

// Remove removes the comment with the given ID, if there is one.
func (cs *Comments) Remove(id int) {
	cs.items = slices.DeleteFunc(cs.items, func(c *Comment) bool { return c.ID == id })
}

// All returns the comments, sorted by path and then by start line.
func (cs *Comments) All() []*Comment {
	all := slices.Clone(cs.items)
	slices.SortStableFunc(all, func(a, b *Comment) int {
		return cmp.Or(strings.Compare(a.Path, b.Path), cmp.Compare(a.Start, b.Start))
	})
	return all
}

// Find returns the comment with the given ID, or nil.
func (cs *Comments) Find(id int) *Comment {
	for _, c := range cs.items {
		if c.ID == id {
			return c
		}
	}
	return nil
}

// At returns the first comment whose range includes line, or nil.
func (cs *Comments) At(path string, side Side, line int) *Comment {
	for _, c := range cs.items {
		if c.Path == path && c.Side == side && line >= c.Start && line <= c.End {
			return c
		}
	}
	return nil
}

// EndingAt returns the comments whose range ends on line.
func (cs *Comments) EndingAt(path string, side Side, line int) []*Comment {
	var ending []*Comment
	for _, c := range cs.items {
		if c.Path == path && c.Side == side && c.End == line {
			ending = append(ending, c)
		}
	}
	return ending
}

// Count returns the number of comments on the file at path.
func (cs *Comments) Count(path string) int {
	n := 0
	for _, c := range cs.items {
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
func (cs *Comments) Relocate(path, old, new string) {
	// lines has the lines of each side. Relocate splits a side only if a
	// comment needs that side.
	var lines [2][]string
	for _, c := range cs.items {
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

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// savedComment is a [Comment] with its private fields, for MarshalJSON.
type savedComment struct {
	ID      int      `json:"id"`
	Path    string   `json:"path"`
	Side    Side     `json:"side"`
	Start   int      `json:"start"`
	End     int      `json:"end"`
	Text    string   `json:"text"`
	Stale   bool     `json:"stale"`
	Code    []string `json:"code"`
	Snippet []string `json:"snippet"`
}

// savedComments is a [Comments] with its private fields.
type savedComments struct {
	Last  int            `json:"last"`
	Items []savedComment `json:"items"`
}

// MarshalJSON encodes the comments with their code and their diff rows,
// so that UnmarshalJSON restores the same comments, for example after a
// restart.
func (cs *Comments) MarshalJSON() ([]byte, error) {
	saved := savedComments{Last: cs.last}
	for _, c := range cs.items {
		saved.Items = append(saved.Items, savedComment{
			ID: c.ID, Path: c.Path, Side: c.Side, Start: c.Start, End: c.End,
			Text: c.Text, Stale: c.Stale, Code: c.code, Snippet: c.snippet,
		})
	}
	return json.Marshal(saved)
}

// UnmarshalJSON reads comments that MarshalJSON wrote.
func (cs *Comments) UnmarshalJSON(b []byte) error {
	var saved savedComments
	if err := json.Unmarshal(b, &saved); err != nil {
		return err
	}
	*cs = Comments{last: saved.Last}
	for _, c := range saved.Items {
		cs.items = append(cs.items, &Comment{
			ID: c.ID, Path: c.Path, Side: c.Side, Start: c.Start, End: c.End,
			Text: c.Text, Stale: c.Stale, code: c.Code, snippet: c.Snippet,
		})
	}
	return nil
}
