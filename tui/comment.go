package tui

import (
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/rednafi/rexdiff/diff"
)

// targetLine is the source line that a comment on a row attaches to.
type targetLine struct {
	side diff.Side
	line int
}

// target returns the line that a comment on row i of the current pane
// attaches to, or nil. If the row changes a line on side pref, it uses
// that side. Else it prefers the new side. So a comment on an unchanged
// line is on the new side.
func (m *model) target(i int, pref diff.Side) *targetLine {
	p := m.cur()
	if p.path == "" || i < 0 || i >= len(p.rows) {
		return nil
	}
	r := p.rows[i]
	switch {
	case pref == diff.SideOld && r.Old > 0 && r.Kind != diff.KindEqual:
		return &targetLine{diff.SideOld, r.Old}
	case r.New > 0:
		return &targetLine{diff.SideNew, r.New}
	case r.Old > 0:
		return &targetLine{diff.SideOld, r.Old}
	}
	return nil
}

// changedFile returns the selected file of the Changes tab, or nil.
func (m *model) changedFile() *changed {
	if m.changeSel >= len(m.files) {
		return nil
	}
	return &m.files[m.changeSel]
}

// contentFor returns the text of side s of the file at path.
func (m *model) contentFor(path string, s diff.Side) string {
	if f := m.changedFile(); m.tab == tabChanges && f != nil && f.Path == path {
		if s == diff.SideOld {
			return f.Old
		}
		return f.New
	}
	content, _ := m.repo.ReadFile(path) // an unreadable file gives a comment no code
	return content
}

// commentAtCursor returns the comment on the cursor line, or nil.
func (m *model) commentAtCursor() *diff.Comment {
	p := m.cur()
	t := m.target(p.cur, diff.SideNew)
	if t == nil {
		return nil
	}
	return m.comments.At(p.path, t.side, t.line)
}

// selection returns the first and the last row of the visual selection.
// With no selection, it returns the rows of the cursor line.
func (p *pane) selection() (lo, hi int) {
	if p.anchor < 0 {
		return p.cur, p.end(p.cur)
	}
	return p.span(p.anchor, p.cur)
}

// span returns the first and the last row of the lines between rows a and
// b, in either order.
func (p *pane) span(a, b int) (lo, hi int) {
	return min(a, b), p.end(max(a, b))
}

// startComment opens the comment box for the selection, or for the cursor
// line. On a line that has a comment, it edits the comment.
func (m *model) startComment(pref diff.Side) tea.Cmd {
	p := m.cur()
	t := m.target(p.cur, pref)
	if t == nil {
		return nil
	}
	if c := m.comments.At(p.path, t.side, t.line); c != nil && p.anchor < 0 {
		return m.editComment(c)
	}
	lo, hi := p.selection()
	c := diff.NewComment(
		p.path,
		t.side,
		p.rows[lo:hi+1],
		m.contentFor(p.path, diff.SideOld),
		m.contentFor(p.path, diff.SideNew),
	)
	m.draft = &draft{Comment: c}
	m.editor.SetValue("")
	m.mode = modeComment
	// Put the cursor on the last line of the range, so that the box opens
	// below the range.
	cursorToLine(p, c.Side, c.End)
	return m.editor.Focus()
}

// editComment opens the comment box with the text of c.
func (m *model) editComment(c *diff.Comment) tea.Cmd {
	m.draft = &draft{Comment: *c, editID: c.ID}
	m.editor.SetValue(c.Text)
	m.mode = modeComment
	cursorToLine(m.cur(), c.Side, c.End)
	return m.editor.Focus()
}

// updateComment handles a key while the comment box is open.
func (m *model) updateComment(msg tea.KeyPressMsg) tea.Cmd {
	switch {
	case key.Matches(msg, m.keys.Save):
		d := m.draft
		if d.editID > 0 {
			m.comments.Remove(d.editID)
		}
		if text := strings.TrimSpace(m.editor.Value()); text != "" {
			c := d.Comment
			c.Text, c.Stale = text, false
			m.comments.Add(c)
		}
		m.closeComment()
		return nil
	case key.Matches(msg, m.keys.Cancel):
		m.closeComment()
		return nil
	}
	var cmd tea.Cmd
	m.editor, cmd = m.editor.Update(msg)
	return cmd
}

// closeComment closes the comment box and clears the selection.
func (m *model) closeComment() {
	m.mode, m.draft = modeNormal, nil
	m.cur().anchor = -1
	m.editor.Blur()
}

// newLineNear returns the new line of row i. A row with no new line, for
// example a deleted line, gives the first new line below it, else the last
// new line above it, else 1.
func newLineNear(rows []diff.Row, i int) int {
	for _, r := range rows[i:] {
		if r.New > 0 {
			return r.New
		}
	}
	for _, r := range slices.Backward(rows[:i]) {
		if r.New > 0 {
			return r.New
		}
	}
	return 1
}

// editOrOpen edits the comment at the cursor. If there is no comment, it
// opens the file in an editor at the cursor line.
func (m *model) editOrOpen() tea.Cmd {
	if c := m.commentAtCursor(); c != nil {
		return m.editComment(c)
	}
	p := m.cur()
	if p.path == "" {
		return nil
	}
	return m.openEditor(p.path, newLineNear(p.rows, p.cur))
}
