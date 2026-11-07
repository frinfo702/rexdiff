// Package diffview draws the difference between two versions of a file as
// terminal lines, in a unified or a split layout, with syntax colours.
//
// [Render] returns the screen lines, one [Row] for each line, and the
// [Geometry] of the columns.
package diffview

import (
	"cmp"
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/aymanbagabas/go-udiff"
	"github.com/charmbracelet/x/ansi"
)

const (
	markWidth = 2   // the cells of the mark before the code, such as "▌ "
	ellipsis  = "…" // shows where a line is cut
)

// The marks before the code of a line.
const (
	markNone   = "  "
	markBar    = "▌ "
	markInsert = "+ "
	markDelete = "- "
)

// Options control how [Render] draws a diff.
type Options struct {
	Split    bool // the old lines on the left, the new lines on the right
	Width    int  // the maximum width of a line, in cells; 0 means no limit
	Offset   int  // the number of cells to scroll the code to the right
	Wrap     bool // continue a long line on the next lines; Offset does not apply
	Signs    bool // mark a changed line with + or -, not with a bar
	TabWidth int  // the number of spaces that replace a tab; 0 means 8
	Style    Style
}

// RowKind is the type of a rendered row.
type RowKind int

const (
	RowHunk   RowKind = iota // a hunk header or its scope line
	RowEqual                 // a line that is the same in the two versions
	RowInsert                // an inserted line
	RowDelete                // a deleted line
	RowChange                // in the split layout, a deleted line beside an inserted line
)

// Row tells which source lines one screen line shows. Before and After are
// line numbers from 1, or 0 if the row has no line on that side. Continued
// is true for the second and the next screen lines of a wrapped line.
type Row struct {
	Kind          RowKind
	Before, After int
	Continued     bool
}

// Geometry gives the columns of the line numbers and of the code, from 0.
type Geometry struct {
	BeforeNum     int // the width of the old line-number column
	AfterNum      int // the width of the new line-number column
	AfterNumStart int // the column where the new line numbers start
	BeforeCode    int // the column where the old code starts
	AfterCode     int // the column where the new code starts
}

// line is one source line of a hunk. old and new are its line numbers, 0 on
// the side that does not have it.
type line struct {
	kind     udiff.OpKind
	old, new int
	text     string
}

// hunk is a group of changed lines with their context.
type hunk struct {
	header string // tells how many unchanged lines come before the hunk
	scope  string // the line that encloses the hunk, or ""
	lines  []line
}

// renderer keeps the state of one call to Render.
type renderer struct {
	o         Options
	hl        highlighter
	oldDigits int
	newDigits int
	code      int // the width of the code column (the left one in split)
	codeRight int // the width of the right code column in split
	lines     []string
	rows      []Row
}

// Render draws the diff from before to after, the two versions of the file
// at path. The path selects the syntax colours. Render returns no lines if
// the versions are the same.
func Render(path, before, after string, o Options) ([]string, []Row, Geometry) {
	before = strings.ReplaceAll(before, "\r\n", "\n")
	after = strings.ReplaceAll(after, "\r\n", "\n")
	hunks := diffHunks(before, after, cmp.Or(o.TabWidth, 8))
	r := &renderer{o: o, hl: newHighlighter(path, cmp.Or(before, after))}
	r.measure(hunks)
	for _, h := range hunks {
		r.header(h)
		if o.Split {
			for _, p := range pairs(h.lines) {
				r.splitRow(p[0], p[1])
			}
			continue
		}
		for _, l := range h.lines {
			r.unifiedRow(l)
		}
	}
	return r.lines, r.rows, r.geometry()
}

// diffHunks computes the hunks of the diff, with the tabs of the lines
// replaced after the diff, so that a change from a tab to spaces shows.
func diffHunks(before, after string, tabWidth int) []hunk {
	u, err := udiff.ToUnifiedDiff("", "", before, udiff.Lines(before, after), udiff.DefaultContextLines)
	if err != nil {
		return nil
	}
	tab := strings.Repeat(" ", tabWidth)
	firsts := make([]int, len(u.Hunks))
	hunks := make([]hunk, len(u.Hunks))
	prevEnd := 0 // the last old line of the hunk above
	for i, uh := range u.Hunks {
		firsts[i] = uh.ToLine - 1
		h := &hunks[i]
		switch n := uh.FromLine - 1 - prevEnd; {
		case n <= 0:
			h.header = " "
		case n == 1:
			h.header = "  ↕ 1 unchanged line "
		default:
			h.header = fmt.Sprintf("  ↕ %d unchanged lines ", n)
		}
		old, new := uh.FromLine, uh.ToLine
		for _, ul := range uh.Lines {
			l := line{kind: ul.Kind, text: strings.ReplaceAll(strings.TrimSuffix(ul.Content, "\n"), "\t", tab)}
			if ul.Kind != udiff.Insert {
				l.old = old
				old++
			}
			if ul.Kind != udiff.Delete {
				l.new = new
				new++
			}
			h.lines = append(h.lines, l)
		}
		prevEnd = old - 1
	}
	scopes := scopesOf(strings.Split(after, "\n"), firsts)
	for i := range hunks {
		hunks[i].scope = scopes[i]
	}
	return hunks
}

// pairs puts the lines of a hunk side by side. An equal line is on both
// sides. In each run of changed lines, the n-th deleted line is beside the
// n-th inserted line. A nil side has no line.
func pairs(lines []line) [][2]*line {
	var out [][2]*line
	for i := 0; i < len(lines); {
		if lines[i].kind == udiff.Equal {
			out = append(out, [2]*line{&lines[i], &lines[i]})
			i++
			continue
		}
		var dels, ins []*line
		for ; i < len(lines) && lines[i].kind != udiff.Equal; i++ {
			if lines[i].kind == udiff.Delete {
				dels = append(dels, &lines[i])
			} else {
				ins = append(ins, &lines[i])
			}
		}
		for k := range max(len(dels), len(ins)) {
			var p [2]*line
			if k < len(dels) {
				p[0] = dels[k]
			}
			if k < len(ins) {
				p[1] = ins[k]
			}
			out = append(out, p)
		}
	}
	return out
}

// measure sets the widths of the line numbers and of the code.
func (r *renderer) measure(hunks []hunk) {
	natural := 0
	for _, h := range hunks {
		natural = max(natural, ansi.StringWidth(h.header), ansi.StringWidth(h.scope)+1)
		for _, l := range h.lines {
			r.oldDigits = max(r.oldDigits, len(fmt.Sprint(l.old)))
			r.newDigits = max(r.newDigits, len(fmt.Sprint(l.new)))
			natural = max(natural, ansi.StringWidth(Escape(l.text))+1)
		}
	}
	r.code, r.codeRight = natural, natural
	if r.o.Width <= 0 {
		return
	}
	free := r.o.Width - r.numsWidth() - markWidth
	if r.o.Split {
		free -= markWidth
		r.code, r.codeRight = free/2, free-free/2
		return
	}
	r.code = free
}

// numsWidth returns the width of the two line-number columns.
func (r *renderer) numsWidth() int { return r.oldDigits + r.newDigits + 4 }

func (r *renderer) geometry() Geometry {
	g := Geometry{BeforeNum: r.oldDigits + 2, AfterNum: r.newDigits + 2}
	if r.o.Split {
		g.AfterNumStart = g.BeforeNum + markWidth + r.code
		g.BeforeCode = g.BeforeNum + markWidth
		g.AfterCode = g.AfterNumStart + g.AfterNum + markWidth
		return g
	}
	g.AfterNumStart = g.BeforeNum
	g.BeforeCode = g.BeforeNum + g.AfterNum + markWidth
	g.AfterCode = g.BeforeCode
	return g
}

// emit adds the screen lines of one row.
func (r *renderer) emit(row Row, lines ...string) {
	for k, l := range lines {
		if r.o.Width > 0 {
			l = ansi.Truncate(l, r.o.Width, "")
		}
		row.Continued = k > 0
		r.lines = append(r.lines, l)
		r.rows = append(r.rows, row)
	}
}

// header adds the header row of h, and its scope row if it has a scope.
func (r *renderer) header(h hunk) {
	div, eq := r.o.Style.dividerLine, r.o.Style.equalLine
	full := r.code + markWidth
	text := fit(div.code, ansi.Truncate(h.header, full, ellipsis), full)
	if r.o.Split {
		r.emit(Row{Kind: RowHunk}, div.num(ellipsis, r.oldDigits)+text+
			div.num(ellipsis, r.newDigits)+fit(div.code, " ", r.codeRight+markWidth))
	} else {
		r.emit(Row{Kind: RowHunk}, div.num(ellipsis, r.oldDigits)+div.num(ellipsis, r.newDigits)+text)
	}
	if h.scope == "" {
		return
	}
	grey := lipgloss.NewStyle().Foreground(eq.lineNumber.GetForeground())
	scope := eq.num(" ", r.oldDigits)
	if !r.o.Split {
		scope += eq.num(" ", r.newDigits)
	}
	scope += eq.symbol.Render(markNone) + fit(grey, ansi.Truncate(h.scope, r.code, ellipsis), r.code)
	if r.o.Split {
		scope += eq.num(" ", r.newDigits) + fit(eq.code, " ", r.codeRight+markWidth)
	}
	r.emit(Row{Kind: RowHunk}, scope)
}

// style returns the style and the mark of a line of kind k.
func (r *renderer) style(k udiff.OpKind) (lineStyle, string) {
	s := r.o.Style
	switch k {
	case udiff.Insert:
		return s.insertLine, cond(r.o.Signs, markInsert, markBar)
	case udiff.Delete:
		return s.deleteLine, cond(r.o.Signs, markDelete, markBar)
	}
	return s.equalLine, markNone
}

func (r *renderer) unifiedRow(l line) {
	ls, mark := r.style(l.kind)
	num := func(n, digits int) string {
		if n == 0 {
			return ls.num(" ", digits)
		}
		return ls.num(n, digits)
	}
	nums := num(l.old, r.oldDigits) + num(l.new, r.newDigits)
	blank := ls.num(" ", r.oldDigits) + ls.num(" ", r.newDigits)
	kind := map[udiff.OpKind]RowKind{udiff.Equal: RowEqual, udiff.Insert: RowInsert, udiff.Delete: RowDelete}[l.kind]
	r.emit(Row{Kind: kind, Before: l.old, After: l.new}, r.cell(ls, nums, blank, mark, l.text, r.code)...)
}

func (r *renderer) splitRow(old, new *line) {
	left := r.side(old, old.lineNum(true), r.oldDigits, r.code)
	right := r.side(new, new.lineNum(false), r.newDigits, r.codeRight)
	n := max(len(left.lines), len(right.lines))
	lines := make([]string, n)
	for k := range n {
		lines[k] = left.at(k) + right.at(k)
	}
	row := Row{Kind: RowChange, Before: old.lineNum(true), After: new.lineNum(false)}
	switch {
	case old == nil:
		row.Kind = RowInsert
	case new == nil:
		row.Kind = RowDelete
	case old.kind == udiff.Equal:
		row.Kind = RowEqual
	}
	r.emit(row, lines...)
}

// lineNum returns the line number of l on the old or the new side, or 0 if
// l is nil.
func (l *line) lineNum(old bool) int {
	switch {
	case l == nil:
		return 0
	case old:
		return l.old
	}
	return l.new
}

// sideLines are the screen lines of one side of a split row. fill is the
// empty line that pads the side when the other side has more lines.
type sideLines struct {
	lines []string
	fill  string
}

func (s sideLines) at(k int) string {
	if k < len(s.lines) {
		return s.lines[k]
	}
	return s.fill
}

// side renders l, with the line number n, on one side of the split layout.
func (r *renderer) side(l *line, n, digits, width int) sideLines {
	ls, mark, text := r.o.Style.missingLine, markNone, ""
	if l != nil {
		ls, mark = r.style(l.kind)
		text = l.text
	}
	blank := ls.num(" ", digits)
	nums := blank
	if l != nil {
		nums = ls.num(n, digits)
	}
	return sideLines{
		lines: r.cell(ls, nums, blank, mark, text, width),
		fill:  r.cell(ls, blank, blank, markNone, "", width)[0],
	}
}

// cell renders the screen lines of one source line: nums, the mark, and the
// highlighted code in width cells. A long line is cut, or with Wrap goes on
// over more lines, which have blank in place of nums and no mark.
func (r *renderer) cell(ls lineStyle, nums, blank, mark, text string, width int) []string {
	code := r.hl.line(text, ls.code.GetBackground())
	var parts []string
	if r.o.Wrap {
		w := max(width, 1)
		// Wrap can keep a leading space on a full line, so cut again.
		for _, p := range strings.Split(ansi.Hardwrap(lipgloss.Wrap(code, w, ""), w, true), "\n") {
			parts = append(parts, ansi.Truncate(p, w, "")) // a wide character can be wider than w
		}
	} else {
		code = ansi.Truncate(ansi.TruncateLeft(code, r.o.Offset, ""), width, ellipsis)
		if r.o.Offset > 0 && strings.TrimSpace(ansi.Strip(code)) != "" {
			mark = mark[:len(mark)-1] + ellipsis // the code goes on to the left
		}
		parts = []string{code}
	}
	out := make([]string, len(parts))
	for i, p := range parts {
		if i > 0 {
			nums, mark = blank, markNone
		}
		out[i] = nums + ansi.Truncate(ls.symbol.Render(mark)+fit(ls.code, p, width), width+markWidth, "")
	}
	return out
}

// fit renders s with style s in exactly width cells. s must not be wider.
func fit(style lipgloss.Style, s string, width int) string {
	if width <= 0 {
		return ""
	}
	return style.Width(width).Render(s)
}

func cond[T any](b bool, yes, no T) T {
	if b {
		return yes
	}
	return no
}
