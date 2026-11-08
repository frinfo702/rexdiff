package tui

// This file finds text as the user types it in the find bar of the footer.
// The pane and the file list stay on the screen: the cursor moves to the
// first match and the matches are painted. In the main pane, the find looks
// at the code. When the file list has the focus, it looks at the names of
// the files and selects the first one that matches.

import (
	"fmt"
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/frinfo702/rexdiff/diff"
)

// findSGR marks a match: black on yellow, from the terminal palette.
var findSGR = ansi.NewStyle().ForegroundColor(ansi.Black).BackgroundColor(ansi.Yellow).String()

// startFind opens the find bar. The find looks in the file list if the
// list shows and has the focus, else in the main pane.
func (m *model) startFind() {
	list := m.focusList && m.listWidth() > 0 && !m.tab.document()
	from := m.cur().cur
	if list {
		from = m.listSel()
	}
	m.find = findState{list: list, from: from}
	m.findBar.Reset()
	m.findBar.Focus()
	m.mode = modeFind
}

// updateFind handles a key while the find bar is open. Enter keeps the
// find, so that n and N step through its matches. Esc goes back to where
// the find started.
func (m *model) updateFind(msg tea.KeyPressMsg) tea.Cmd {
	switch {
	case key.Matches(msg, m.keys.Save):
		m.mode = modeNormal
		if !m.find.active() {
			m.find = findState{}
		}
		return nil
	case key.Matches(msg, m.keys.Cancel):
		m.mode = modeNormal
		f := m.find
		m.find = findState{}
		if f.list {
			m.pickFile(f.from)
		} else {
			m.cur().cur = f.from
		}
		m.ensureVisible()
		return nil
	}
	before := m.findBar.Value()
	var cmd tea.Cmd
	m.findBar, cmd = m.findBar.Update(msg)
	if q := m.findBar.Value(); q != before {
		m.runFind(q)
		m.ensureVisible()
	}
	return cmd
}

// runFind finds q, without case, from where the find started. It moves the
// cursor, or the selection of the list, to the first match at or after
// the start, or back to the start if nothing matches.
func (m *model) runFind(q string) {
	f := &m.find
	f.query, f.hits, f.i = q, nil, 0
	if q == "" {
		m.moveToFind(f.from)
		return
	}
	lq := strings.ToLower(q)
	if f.list {
		for i := range m.listLen() {
			if strings.Contains(strings.ToLower(m.listPath(i)), lq) {
				f.hits = append(f.hits, i)
			}
		}
	} else {
		p := m.cur()
		for i := range p.lines {
			if !strings.Contains(strings.ToLower(p.searchText(i)), lq) {
				continue
			}
			// The rows of a wrapped line make one hit.
			if h := p.head(i); len(f.hits) == 0 || f.hits[len(f.hits)-1] != h {
				f.hits = append(f.hits, h)
			}
		}
	}
	if !f.active() {
		m.moveToFind(f.from)
		return
	}
	f.i = max(slices.IndexFunc(f.hits, func(h int) bool { return h >= f.from }), 0)
	m.moveToFind(f.hits[f.i])
}

// stepFind moves to the next (d = 1) or the previous (d = -1) match of the
// find. It wraps around.
func (m *model) stepFind(d int) {
	f := &m.find
	// A reload can make the pane or the list shorter than the matches.
	n := len(m.cur().lines)
	if f.list {
		n = m.listLen()
	}
	f.hits = slices.DeleteFunc(f.hits, func(h int) bool { return h >= n })
	if len(f.hits) == 0 {
		m.find = findState{}
		return
	}
	f.i = (min(f.i, len(f.hits)-1) + d + len(f.hits)) % len(f.hits)
	m.moveToFind(f.hits[f.i])
}

// moveToFind moves the cursor to row i of the pane, or selects file i of
// the list.
func (m *model) moveToFind(i int) {
	if !m.find.list {
		m.cur().cur = i
		return
	}
	if i != m.listSel() {
		m.pickFile(i)
	}
}

// findCount tells which match of the find the cursor is on, for the find
// bar.
func (m *model) findCount() string {
	switch f := m.find; {
	case f.query == "":
		return ""
	case !f.active():
		return "no match"
	default:
		return fmt.Sprintf("%d/%d", f.i+1, len(f.hits))
	}
}

// listPath returns the path of file i of the file list of the current tab.
func (m *model) listPath(i int) string {
	switch m.tab {
	case tabFiles:
		return m.allFiles[i].Path
	case tabGuide:
		return m.guideFiles()[i].path
	}
	return m.files[i].Path
}

// searchText returns line i of p as plain text, with the line numbers of a
// diff blanked, so that a find matches only the code. The text has the
// cells of the line.
func (p *pane) searchText(i int) string {
	s := ansi.Strip(p.lines[i])
	if p.diff == nil || !p.hasCode(i) {
		return s
	}
	oldLo, oldHi := p.diff.CodeRange(diff.SideOld)
	newLo, _ := p.diff.CodeRange(diff.SideNew)
	s = blankCells(s, 0, oldLo)
	if oldHi > 0 { // the split layout: the new line numbers are in the middle
		s = blankCells(s, oldHi, newLo)
	}
	return s
}

// blankCells replaces the cells [lo, hi) of the plain text s with spaces.
func blankCells(s string, lo, hi int) string {
	w := ansi.StringWidth(s)
	lo, hi = min(lo, w), min(hi, w)
	if lo >= hi {
		return s
	}
	return ansi.Cut(s, 0, lo) + strings.Repeat(" ", hi-lo) + ansi.Cut(s, hi, w)
}

// paintMatches marks each match of q in line. text is the plain text of
// line, with the same cells, where the find looks for q.
func paintMatches(line, text, q string) string {
	lt, lq := strings.ToLower(text), strings.ToLower(q)
	for at := 0; ; {
		i := strings.Index(lt[at:], lq)
		if i < 0 || lq == "" {
			return line
		}
		i += at
		start := ansi.StringWidth(lt[:i])
		line = paintCells(line, start, start+ansi.StringWidth(lq), findSGR)
		at = i + len(lq)
	}
}

// paintCells returns line with the cells [start, end) on sgr. The cells
// after them keep their own style only from their next escape sequence.
func paintCells(line string, start, end int, sgr string) string {
	w := ansi.StringWidth(line)
	end = min(end, w)
	if start >= end {
		return line
	}
	out := ansi.Cut(line, 0, start) + carrySGR(ansi.Cut(line, start, end), sgr) + ansi.ResetStyle
	if end < w {
		out += ansi.Cut(line, end, w)
	}
	return out
}
