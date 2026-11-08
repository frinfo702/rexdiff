package tui

// This file selects text in the main pane. A drag over the code selects
// the cells under the pointer and copies them when the button is released.
// The selection stays after the button is released, so that y can copy it
// again and esc can clear it.

import (
	"slices"
	"strconv"
	"strings"
	"unicode"

	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/frinfo702/rexdiff/diff"
)

// textPoint is a cell of the main pane: row i of its lines, and column col
// of that line. The column is a screen cell, so a wide character takes two.
type textPoint struct {
	row, col int
}

// textSelection is the text between two cells of the main pane. The end is
// inclusive. side is the side of a split diff that the drag started on, so
// that a copy takes the code of that side and not the line numbers.
type textSelection struct {
	start, end textPoint
	side       diff.Side
}

// ordered returns the first and the last point of s in reading order.
func (s textSelection) ordered() (lo, hi textPoint) {
	lo, hi = s.start, s.end
	if hi.row < lo.row || hi.row == lo.row && hi.col < lo.col {
		lo, hi = hi, lo
	}
	return lo, hi
}

// selectAt returns the cell at column x of row i, cut to the line. A line
// with no text gives column 0.
func (p *pane) selectAt(i, x int) textPoint {
	if len(p.lines) == 0 {
		return textPoint{}
	}
	i = min(max(i, 0), len(p.lines)-1)
	w := ansi.StringWidth(p.lines[i])
	return textPoint{row: i, col: min(max(x, 0), max(w-1, 0))}
}

// wordAt returns the selection of the word at pt: the letters, the digits,
// and the underscores around it. A cell that is not in a word selects only
// its character.
func (p *pane) wordAt(pt textPoint, side diff.Side) *textSelection {
	type cell struct {
		col, w int
		word   bool
	}
	var cells []cell
	col := 0
	for _, r := range ansi.Strip(p.lines[pt.row]) {
		w := max(ansi.StringWidth(string(r)), 1)
		cells = append(cells, cell{col, w, unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'})
		col += w
	}
	k := slices.IndexFunc(cells, func(c cell) bool { return pt.col < c.col+c.w })
	if k < 0 {
		return &textSelection{start: pt, end: pt, side: side}
	}
	lo, hi := k, k
	for cells[k].word && lo > 0 && cells[lo-1].word {
		lo--
	}
	for cells[k].word && hi+1 < len(cells) && cells[hi+1].word {
		hi++
	}
	return &textSelection{
		start: textPoint{pt.row, cells[lo].col},
		end:   textPoint{pt.row, cells[hi].col + cells[hi].w - 1},
		side:  side,
	}
}

// selectedText returns the text that p.text selects. It leaves out the
// spaces at the end of each line, and in a diff it leaves out the line
// numbers and the marks. A wrapped line gives one line, as the source does.
func (p *pane) selectedText() string {
	if p.text == nil {
		return ""
	}
	lo, hi := p.text.ordered()
	var out []string
	for i := lo.row; i <= hi.row; i++ {
		if i < 0 || i >= len(p.lines) {
			continue
		}
		start, end := p.selectionCols(i, lo, hi)
		if p.diff != nil && p.hasCode(i) {
			code, last := p.diff.CodeRange(p.text.side)
			if last == 0 {
				last = ansi.StringWidth(p.lines[i])
			}
			start, end = max(start, code), min(end, last)
		}
		var s string
		if start < end {
			s = strings.TrimRight(ansi.Strip(ansi.Cut(p.lines[i], start, end)), " ")
		}
		if i > lo.row && p.cont(i) && len(out) > 0 {
			out[len(out)-1] += s
			continue
		}
		out = append(out, s)
	}
	return strings.Join(out, "\n")
}

// hasCode reports whether row i shows a source line, and so has line
// numbers to leave out of a copy.
func (p *pane) hasCode(i int) bool {
	if i < 0 || i >= len(p.rows) {
		return false
	}
	r := p.rows[i]
	return r.Kind != diff.KindHunk && (r.Old > 0 || r.New > 0)
}

// selectionCols returns the screen columns [start, end) that row i of the
// selection covers.
func (p *pane) selectionCols(i int, lo, hi textPoint) (start, end int) {
	w := ansi.StringWidth(p.lines[i])
	start, end = 0, w
	if i == lo.row {
		start = lo.col
	}
	if i == hi.row {
		end = hi.col + 1
	}
	return min(max(start, 0), w), min(max(end, 0), w)
}

// markSelection returns line i with its selected cells on the background
// of sgr. The colours of the text stay.
func (p *pane) markSelection(i int, sgr string) string {
	line := p.lines[i]
	if p.text == nil {
		return line
	}
	lo, hi := p.text.ordered()
	if i < lo.row || i > hi.row {
		return line
	}
	start, end := p.selectionCols(i, lo, hi)
	return paintCells(line, start, end, sgr)
}

// carrySGR repeats sgr after each escape sequence of s. A reset in the
// text then cannot turn the selection off.
func carrySGR(s, sgr string) string {
	var b strings.Builder
	b.WriteString(sgr)
	var state byte
	for len(s) > 0 {
		seq, width, n, next := ansi.DecodeSequence(s, state, nil)
		if n <= 0 {
			b.WriteString(s)
			break
		}
		b.WriteString(seq)
		if width == 0 {
			b.WriteString(sgr)
		}
		s, state = s[n:], next
	}
	return b.String()
}

// underSGR puts sgr under the cells of s that have no background of their
// own, so that sgr shows only where s shows the terminal background.
func underSGR(s, sgr string) string {
	var b strings.Builder
	b.WriteString(sgr)
	own := false // the text sets its own background now
	var state byte
	for len(s) > 0 {
		seq, width, n, next := ansi.DecodeSequence(s, state, nil)
		if n <= 0 {
			b.WriteString(s)
			break
		}
		b.WriteString(seq)
		if width == 0 && strings.HasPrefix(seq, "\x1b[") && strings.HasSuffix(seq, "m") {
			own = setsBackground(seq[2:len(seq)-1], own)
			if !own {
				b.WriteString(sgr)
			}
		}
		s, state = s[n:], next
	}
	return b.String()
}

// setsBackground tells if the text has a background of its own after the
// SGR parameters params. own tells if it had one before.
func setsBackground(params string, own bool) bool {
	if params == "" {
		return false // a reset
	}
	ps := strings.Split(params, ";")
	for i := 0; i < len(ps); i++ {
		p := ps[i]
		if base, _, ok := strings.Cut(p, ":"); ok { // 48:2::r:g:b
			own = own || base == "48"
			continue
		}
		switch n, _ := strconv.Atoi(p); {
		case n == 0 || n == 49:
			own = false
		case n >= 40 && n <= 47 || n >= 100 && n <= 107:
			own = true
		case n == 38 || n == 48 || n == 58: // 5;i or 2;r;g;b follow
			if n == 48 {
				own = true
			}
			if i+1 < len(ps) && ps[i+1] == "5" {
				i += 2
			} else if i+1 < len(ps) && ps[i+1] == "2" {
				i += 4
			}
		}
	}
	return own
}

// textSelectSGR returns the sequence that marks a selection. It tints the
// background, or reverses the colours if the terminal does not tell its
// background.
func (m *model) textSelectSGR() string {
	if m.bg == nil {
		return ansi.NewStyle().Reverse(true).String()
	}
	tint := lipgloss.Lighten(m.bg, 0.22)
	if !m.dark {
		tint = lipgloss.Darken(m.bg, 0.12)
	}
	return ansi.NewStyle().BackgroundColor(tint).String()
}

// copyTextSelection copies the selected text. It returns nil if the pane
// has no text selection.
func (m *model) copyTextSelection() tea.Cmd {
	p := m.cur()
	if p.text == nil {
		return nil
	}
	text := p.selectedText()
	if text == "" {
		m.flash("nothing selected")
		return nil
	}
	m.flash("copied " + plural(len([]rune(text)), "char"))
	return tea.SetClipboard(text)
}
