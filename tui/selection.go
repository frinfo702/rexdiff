package tui

// This file selects text in the main pane. A drag over the code selects
// the cells under the pointer and copies them when the button is released.
// The selection stays after the button is released, so that y can copy it
// again and esc can clear it.

import (
	"strings"

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
	if start >= end {
		return line
	}
	out := ansi.Cut(line, 0, start) + carrySGR(ansi.Cut(line, start, end), sgr) + ansi.ResetStyle
	if end < ansi.StringWidth(line) {
		out += ansi.Cut(line, end, ansi.StringWidth(line))
	}
	return out
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
