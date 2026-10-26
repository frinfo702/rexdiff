package tui

// This file turns files into the lines of a pane: diffs, the file viewer,
// and markdown.

import (
	"errors"
	"fmt"
	"path"
	"strings"

	"charm.land/glamour/v2"
	"charm.land/lipgloss/v2"

	"github.com/frinfo702/rexdiff/diff"
	"github.com/frinfo702/rexdiff/vcs"
)

// gutter is the width of the column for the cursor and the marks, at the
// left of the main pane.
const gutter = 2

// listWidth returns the width of the file list, or 0 if the list does not
// show.
func (m *model) listWidth() int {
	switch {
	case m.tab == tabGuide && len(m.guide.sections) == 0:
		return 0 // the main pane tells why
	case m.tab == tabGuide:
		// The guide is prose, so it takes more room than a list of files,
		// but never more than a third of the screen.
		return min(guideWidth, max(m.w/3, minListWidth))
	case m.listHidden || m.tab.document():
		return 0
	}
	// The list never takes more than a third of a narrow pane.
	return min(m.listW, max(m.w/3, minListWidth))
}

// mainWidth is the width of the main pane, with its gutter.
func (m *model) mainWidth() int {
	w := m.w
	if lw := m.listWidth(); lw > 0 {
		w -= lw + sepWidth
	}
	return max(w, 24)
}

// codeWidth is the width of the main pane, without its gutter.
func (m *model) codeWidth() int { return m.mainWidth() - gutter }

// rerender renders the current pane again, for example after a change of
// the layout, the wrap, or the width. The cursor and the selection stay on
// their source lines. A cursor on a header stays above the first line
// below the header.
func (m *model) rerender() {
	p := m.cur()
	next := p.cur
	for next < len(p.rows) && m.target(next, diff.SideNew) == nil {
		next++
	}
	up := next - p.cur
	cur, anchor := m.target(next, diff.SideNew), m.target(p.anchor, diff.SideNew)
	switch m.tab {
	case tabChanges:
		m.renderChanges()
	case tabFiles:
		m.renderViewer()
	case tabPR:
		m.renderPR()
	case tabOverview:
		m.renderOverview()
	case tabGuide:
		m.renderGuide()
	}
	if cur != nil {
		if i := p.rowOf(cur.side, cur.line); i >= 0 {
			p.cur = max(i-up, 0)
		}
	}
	if anchor != nil {
		if i := p.rowOf(anchor.side, anchor.line); i >= 0 {
			p.anchor = i
		}
	}
}

// minSplitWidth is the narrowest main pane, in columns, that shows a diff
// side by side. A narrower pane shows the diff top to bottom, even if the
// layout is split, because each side would cut most lines.
const minSplitWidth = 110

// splitShown tells if the diff shows side by side now.
func (m *model) splitShown() bool { return m.split && m.codeWidth() >= minSplitWidth }

// diffOptions returns the options to render a diff with sideways scroll
// xoff.
func (m *model) diffOptions(xoff int) diff.Options {
	return diff.Options{
		Width:  m.codeWidth(),
		Offset: xoff,
		Wrap:   m.wrap,
		Split:  m.splitShown(),
		Signs:  m.cfg.DiffSigns,
		Theme: diff.Theme{
			Background: m.bg,
			Foreground: m.fg,
			Green:      m.green,
			Red:        m.red,
		},
	}
}

// isMarkdown reports whether path names a markdown file.
func isMarkdown(path string) bool {
	p := strings.ToLower(path)
	return strings.HasSuffix(p, ".md") || strings.HasSuffix(p, ".markdown")
}

// renderChanges shows the selected file in the Diff tab.
func (m *model) renderChanges() { m.renderChanged(&m.changes, m.changedFile()) }

// renderChanged shows changed file f in p, under a header with its name
// and its counts. f can be nil.
func (m *model) renderChanged(p *pane, f *changed) {
	if f == nil {
		p.path = ""
		p.set(nil, nil)
		return
	}
	p.path = f.Path
	switch {
	case f.Binary:
		p.setNote("binary file, not diffed")
	case f.TooBig:
		p.setNote("file over 2 MB or 50,000 lines, not diffed")
	case f.Old == f.New:
		// A rename without changes, or a change of the file mode.
		note := "no change to the content"
		if f.From != "" {
			note = "renamed from " + diff.Quote(f.From) + ", no change to the content"
		}
		p.setNote(note)
	case m.rendered && isMarkdown(f.Path) && f.Status != vcs.StatusDeleted:
		m.setMarkdown(p, f.New)
	default:
		p.setDiff(diff.Render(f.Path, f.Old, f.New, m.diffOptions(p.xoff)))
	}
	p.addHeader(m.fileHeader(f))
}

// fileHeader returns the line above the diff of f: its name, its
// directory, and its counts, on a tint of the background.
func (m *model) fileHeader(f *changed) string {
	dir, name := path.Split(diff.Quote(f.Path))
	left := " " + statusMark(f.Status) + " " + lipgloss.NewStyle().Bold(true).Render(name)
	if dir != "" {
		left += " " + dimStyle.Render(dir)
	}
	if f.From != "" {
		left += dimStyle.Render(" ← " + diff.Quote(f.From))
	}
	stat := changeStat(*f)
	if c := m.comments.Count(f.Path); c > 0 {
		stat = cursorStyle.Render(fmt.Sprintf("●%d", c)) + "  " + stat
	}
	w := m.codeWidth()
	left = truncate(left, w-lipgloss.Width(stat)-2)
	pad := strings.Repeat(" ", max(w-lipgloss.Width(left)-lipgloss.Width(stat)-1, 1))
	return m.barStyle().Render(left + pad + stat + " ")
}

// barStyle returns the style of a file header: a faint tint of the
// terminal background, fainter than a selected row.
func (m *model) barStyle() lipgloss.Style {
	if m.bg == nil {
		return lipgloss.NewStyle()
	}
	tint := lipgloss.Lighten(m.bg, 0.06)
	if !m.dark {
		tint = lipgloss.Darken(m.bg, 0.04)
	}
	return lipgloss.NewStyle().Background(tint)
}

// addHeader shows line h above the lines of p, on a row that shows no
// source line.
func (p *pane) addHeader(h string) {
	if len(p.lines) == 0 {
		return
	}
	p.lines = append([]string{h}, p.lines...)
	p.rows = append([]diff.Row{{}}, p.rows...)
}

// setNote shows a dim note on one row that shows no source line. The
// pane keeps its path.
func (p *pane) setNote(note string) {
	p.set([]string{dimStyle.Render("  " + note)}, []diff.Row{{}})
}

// renderViewer shows the selected file in the Files tab, even if the file
// did not change.
func (m *model) renderViewer() {
	m.viewer.path = "" // no file has the path "", so refreshViewer renders
	m.refreshViewer()
}

// refreshViewer shows the selected file in the Files tab if the file
// changed.
func (m *model) refreshViewer() {
	p := &m.viewer
	if m.allSel >= len(m.allFiles) {
		p.path = ""
		p.set(nil, nil)
		return
	}
	path := m.allFiles[m.allSel].Path
	content, err := m.repo.ReadFile(path)
	if content == m.viewed && p.path == path {
		return
	}
	m.viewed = content
	p.path = path
	switch {
	case errors.Is(err, vcs.ErrBinary):
		p.path = ""
		p.set([]string{dimStyle.Render("  binary file")}, nil)
	case errors.Is(err, vcs.ErrTooBig):
		p.path = ""
		p.set([]string{dimStyle.Render("  file over 2 MB, not shown")}, nil)
	case m.rendered && isMarkdown(path):
		m.setMarkdown(p, content)
	default:
		p.setDiff(diff.RenderFile(path, content, m.diffOptions(p.xoff)))
	}
}

// markdown returns a glamour renderer for the terminal theme, or nil. The
// renderer wraps at wrap columns.
func (m *model) markdown(wrap int) *glamour.TermRenderer {
	style := "dark"
	if m.bg != nil && !m.dark {
		style = "light"
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(style),
		glamour.WithWordWrap(wrap),
	)
	if err != nil {
		return nil
	}
	return r
}

// renderMarkdown renders src with r. If r is nil or fails, it returns src.
func renderMarkdown(r *glamour.TermRenderer, src string) []string {
	if r != nil {
		if out, err := r.Render(src); err == nil {
			src = out
		}
	}
	return strings.Split(strings.TrimRight(src, "\n"), "\n")
}

// setMarkdown shows src as rendered markdown. The rendered lines take no
// comments. Press m to see the source.
func (m *model) setMarkdown(p *pane, src string) {
	lines := renderMarkdown(m.markdown(m.codeWidth()-2), diff.Plain(src))
	p.path = ""
	p.set(lines, make([]diff.Row, len(lines)))
}
