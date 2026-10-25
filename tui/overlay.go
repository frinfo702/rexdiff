package tui

import (
	"fmt"
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/sahilm/fuzzy"

	"github.com/rednafi/rexdiff/diff"
	"github.com/rednafi/rexdiff/internal/termtext"
	"github.com/rednafi/rexdiff/rex"
	"github.com/rednafi/rexdiff/vcs"
)

// overlayKind tells what an overlay is for.
type overlayKind int

const (
	overlayAgents overlayKind = iota
	overlayBase
	overlayCommits
	overlayComments
	overlaySearch
	overlayGoto
	overlayFind
	overlayQuit
)

// The limits of the lists in the overlays.
const (
	maxFileHits = 15  // the files that match by name in the search
	maxCodeHits = 60  // the code matches in the search
	maxCommits  = 300 // the commits in the commit picker
	minGrepLen  = 2   // the search finds code only for a query this long
)

// pickItem is one row of the list of an overlay.
type pickItem struct {
	label  string
	detail string // the dim text after the label
	tag    string // a short mark at the end, for example "current"
	// id is what the row picks, for example a path, a ref, a commit, or a
	// block.
	id      string
	line    int // the search: the line of a code match
	comment int // the comment list: the ID of the comment
}

// overlay is a modal box. It shows a list to pick from, a search, or a
// prompt.
type overlay struct {
	kind   overlayKind
	title  string
	input  textinput.Model
	items  []pickItem
	shown  []int // the indices of the items that match the input
	cur    int
	mark   int // the commit picker: the item at the start of the range, or -1
	agents []rex.Agent
}

// newOverlay returns an overlay. If its kind has an input, the input has
// the focus.
func newOverlay(kind overlayKind, title string) *overlay {
	ti := textinput.New()
	ti.Prompt = "› "
	ov := &overlay{kind: kind, title: title, input: ti, mark: -1}
	if ov.hasInput() {
		ov.input.Focus()
	}
	return ov
}

// hasInput reports whether the overlay has a text input. The lists of
// agents and comments, and the prompt before a quit, have none.
func (o *overlay) hasInput() bool {
	switch o.kind {
	case overlayAgents, overlayComments, overlayQuit:
		return false
	}
	return true
}

// setItems sets the items, and shows the ones that match the input.
func (o *overlay) setItems(items []pickItem) {
	o.items = items
	o.filter()
}

// filter shows the items that match the input. The search shows all its
// items, because they match already.
func (o *overlay) filter() {
	q := o.input.Value()
	o.shown = o.shown[:0]
	if q == "" || o.kind == overlaySearch {
		for i := range o.items {
			o.shown = append(o.shown, i)
		}
	} else {
		labels := make([]string, len(o.items))
		for i, it := range o.items {
			labels[i] = it.label + " " + it.detail
		}
		for _, mt := range fuzzy.Find(q, labels) {
			o.shown = append(o.shown, mt.Index)
		}
	}
	o.cur = min(o.cur, max(len(o.shown)-1, 0))
}

// selected returns the item at the cursor, or nil.
func (o *overlay) selected() *pickItem {
	if o.cur < 0 || o.cur >= len(o.shown) {
		return nil
	}
	return &o.items[o.shown[o.cur]]
}

// down and up move the cursor of the list by one row.
func (o *overlay) down() { o.cur = min(o.cur+1, max(len(o.shown)-1, 0)) }
func (o *overlay) up()   { o.cur = max(o.cur-1, 0) }

// setSearch shows the search results. It shows the files that match by
// name, then the matches in the code.
func (o *overlay) setSearch(files []vcs.Entry, hits []vcs.Match) {
	q := o.input.Value()
	var items []pickItem
	if q != "" {
		paths := make([]string, 0, len(files))
		for _, f := range files {
			if !f.Ignored {
				paths = append(paths, f.Path)
			}
		}
		for i, mt := range fuzzy.Find(q, paths) {
			if i == maxFileHits {
				break
			}
			items = append(items, pickItem{label: mt.Str, tag: "file", id: mt.Str})
		}
	}
	for _, h := range hits {
		items = append(items, pickItem{
			label:  fmt.Sprintf("%s:%d", h.Path, h.Line),
			detail: h.Text,
			tag:    "code",
			id:     h.Path,
			line:   h.Line,
		})
	}
	o.cur = 0
	o.setItems(items)
}

// open shows ov and gives the keys to it.
func (m *model) open(ov *overlay) {
	m.overlay, m.mode = ov, modeOverlay
}

// closeOverlay closes the overlay and gives the keys back to the panes.
func (m *model) closeOverlay() {
	m.overlay, m.mode = nil, modeNormal
}

// openInput opens a prompt of kind with the text value.
func (m *model) openInput(kind overlayKind, title, value string) {
	ov := newOverlay(kind, title)
	ov.input.SetValue(value)
	m.open(ov)
}

// openConfirmQuit asks the user to confirm a quit that loses comments.
func (m *model) openConfirmQuit() {
	title := plural(m.comments.Len(), "unsent comment") + " will be lost"
	m.open(newOverlay(overlayQuit, title))
}

// openSearch opens the search of the files and the code.
func (m *model) openSearch() {
	m.open(newOverlay(overlaySearch, "search files and code"))
}

// openAgentPicker lets the user pick the agent to send to. The cursor
// starts on the agent of the last send, else the agent beside the pane.
func (m *model) openAgentPicker(agents []rex.Agent) {
	ov := newOverlay(overlayAgents, "send to which agent?")
	ov.agents = agents
	var items []pickItem
	armed := 0
	for i, a := range agents {
		detail := a.App + " · " + string(a.State)
		if a.State == rex.StateBlocked {
			detail = a.App + " · at a prompt"
		}
		if a.Window != "" {
			detail += " · " + a.Window
		}
		tag := ""
		switch a.Block {
		case m.lastSent:
			tag, armed = "last used", i
		case m.beside:
			tag = "opened beside"
			if m.lastSent == "" {
				armed = i
			}
		}
		items = append(items, pickItem{
			label:  fmt.Sprintf("%d  %s", i+1, a.Name),
			detail: detail,
			tag:    tag,
			id:     a.Block,
		})
	}
	ov.setItems(items)
	ov.cur = armed
	m.open(ov)
}

// openBasePicker lets the user pick the base of the branch scope. The
// current base has a tag.
func (m *model) openBasePicker() {
	ov := newOverlay(overlayBase, "base branch for the branch scope (or type any ref)")
	branches, _ := m.repo.Branches(m.ctx) // an error shows an empty picker
	var items []pickItem
	for _, b := range branches {
		tag := ""
		if b == m.base {
			tag = "current"
		}
		items = append(items, pickItem{label: b, id: b, tag: tag})
	}
	ov.setItems(items)
	m.open(ov)
}

// openCommitPicker lets the user pick a commit or a range of commits for
// the commits scope. It lists the newest commits first.
func (m *model) openCommitPicker() {
	ov := newOverlay(overlayCommits, "pick a commit, or space to start a range")
	// An error shows an empty picker.
	commits, _ := m.repo.Commits(m.ctx, maxCommits)
	var items []pickItem
	for _, c := range commits {
		items = append(items, pickItem{
			label:  c.Short + "  " + c.Subject,
			detail: c.Author + ", " + c.When,
			id:     c.SHA,
		})
	}
	ov.setItems(items)
	m.open(ov)
}

// openCommentList lists the comments, to jump to one or to delete one.
func (m *model) openCommentList() {
	if m.comments.Len() == 0 {
		m.flash("no comments yet")
		return
	}
	ov := newOverlay(overlayComments, "comments: enter to jump, d to delete")
	m.fillCommentList(ov)
	m.open(ov)
}

// fillCommentList sets the comments as the items of ov.
func (m *model) fillCommentList(ov *overlay) {
	var items []pickItem
	for _, c := range m.comments.All() {
		tag := ""
		if c.Stale {
			tag = "stale"
		}
		text, _, _ := strings.Cut(c.Text, "\n")
		items = append(items, pickItem{
			label:   c.Location(),
			detail:  text,
			tag:     tag,
			comment: c.ID,
		})
	}
	ov.setItems(items)
}

// updateOverlay handles a key while an overlay is open.
func (m *model) updateOverlay(msg tea.KeyPressMsg) tea.Cmd {
	ov, k := m.overlay, overlayKeys
	if ov.kind == overlayQuit {
		switch {
		case key.Matches(msg, k.Quit):
			return tea.Quit
		case key.Matches(msg, k.Stay):
			m.closeOverlay()
		}
		return nil
	}
	switch {
	case key.Matches(msg, k.Close):
		m.closeOverlay()
		return nil
	case key.Matches(msg, k.Choose):
		return m.chooseOverlay()
	case key.Matches(msg, k.Up):
		ov.up()
		return nil
	case key.Matches(msg, k.Down):
		ov.down()
		return nil
	}
	if !ov.hasInput() {
		return m.updateList(msg)
	}
	if ov.kind == overlayCommits && key.Matches(msg, k.Mark) {
		if ov.mark >= 0 {
			ov.mark = -1
		} else if ov.cur < len(ov.shown) {
			ov.mark = ov.shown[ov.cur]
		}
		return nil
	}
	var cmd tea.Cmd
	before := ov.input.Value()
	ov.input, cmd = ov.input.Update(msg)
	if ov.input.Value() != before {
		return tea.Batch(cmd, m.inputChanged())
	}
	return cmd
}

// updateList handles a key in an overlay that has no input. In the agent
// picker, the digits 1 to 9 pick an agent.
func (m *model) updateList(msg tea.KeyPressMsg) tea.Cmd {
	ov, k, s := m.overlay, overlayKeys, msg.String()
	digit := len(s) == 1 && s[0] >= '1' && s[0] <= '9'
	switch {
	case key.Matches(msg, k.ListDown):
		ov.down()
	case key.Matches(msg, k.ListUp):
		ov.up()
	case ov.kind == overlayComments && key.Matches(msg, k.Delete):
		if it := ov.selected(); it != nil {
			m.comments.Remove(it.comment)
			if m.comments.Len() == 0 {
				m.closeOverlay()
				return nil
			}
			m.fillCommentList(ov)
		}
	case ov.kind == overlayAgents && digit:
		if i := int(s[0] - '1'); i < len(ov.shown) {
			ov.cur = i
			return m.chooseOverlay()
		}
	}
	return nil
}

// inputChanged updates the overlay after a change of its input.
func (m *model) inputChanged() tea.Cmd {
	ov := m.overlay
	switch ov.kind {
	case overlaySearch:
		q := ov.input.Value()
		ov.setSearch(m.allFiles, nil)
		if len(q) < minGrepLen {
			return nil
		}
		repo, ctx := m.repo, m.ctx
		return func() tea.Msg {
			hits, _ := repo.Grep(ctx, q, maxCodeHits)
			return grepMsg{q, hits}
		}
	case overlayBase, overlayCommits, overlayComments, overlayAgents:
		ov.filter()
	}
	return nil
}

// chooseOverlay acts on the selected item or on the input.
func (m *model) chooseOverlay() tea.Cmd {
	ov := m.overlay
	it := ov.selected()
	switch ov.kind {
	case overlayGoto:
		m.closeOverlay()
		m.gotoLine(ov.input.Value())
	case overlayFind:
		m.closeOverlay()
		m.runFind(ov.input.Value())
	case overlaySearch:
		m.closeOverlay()
		if it != nil {
			m.jumpTo(it.id, diff.SideNew, max(it.line, 1))
		}
	case overlayBase:
		ref, ok := m.baseRef(it)
		m.closeOverlay()
		if !ok {
			m.flash("no such ref: " + ref)
			return nil
		}
		m.base = ref
		m.setScope(ScopeBranch)
	case overlayCommits:
		if it == nil {
			return nil
		}
		m.closeOverlay()
		m.from, m.to = ov.commitRange(it)
		m.setScope(scopeCommits)
	case overlayComments:
		m.closeOverlay()
		if it == nil {
			break
		}
		if c := m.comments.Find(it.comment); c != nil {
			m.jumpTo(c.Path, c.Side, c.Start)
		}
	case overlayAgents:
		m.closeOverlay()
		if a, ok := ov.agent(it); ok {
			return m.deliver(a)
		}
	}
	m.ensureVisible()
	return nil
}

// commitRange returns the oldest and the newest commit that the commit
// picker picks with item it. That is one commit, or the range from the
// mark to it. The list shows the newest commit first.
func (o *overlay) commitRange(it *pickItem) (oldest, newest string) {
	if o.mark < 0 {
		return it.id, it.id
	}
	a, b := o.mark, o.shown[o.cur]
	return o.items[max(a, b)].id, o.items[min(a, b)].id
}

// agent returns the agent of item it of the agent picker. If it is nil,
// ok is false.
func (o *overlay) agent(it *pickItem) (a rex.Agent, ok bool) {
	if it == nil {
		return rex.Agent{}, false
	}
	i := slices.IndexFunc(o.agents, func(a rex.Agent) bool { return a.Block == it.id })
	if i < 0 {
		return rex.Agent{}, false
	}
	return o.agents[i], true
}

// baseRef returns the ref that the base picker picks, with the selected
// item it. A typed ref, for example a tag, wins over the first match. A
// match that the user moves to wins over the typed text. If the ref is
// not valid, ok is false.
func (m *model) baseRef(it *pickItem) (ref string, ok bool) {
	ref = strings.TrimSpace(m.overlay.input.Value())
	_, err := m.repo.Resolve(m.ctx, ref)
	if it != nil && (ref == "" || err != nil || m.overlay.cur > 0) {
		ref = it.id
		_, err = m.repo.Resolve(m.ctx, ref)
	}
	return ref, ref != "" && err == nil
}

// overlayBox renders the box of the overlay, and returns its position.
// Row i of the list shows at line listY+i of the box, for mouse hits.
func (m *model) overlayBox() (box string, x, y, listY int) {
	ov := m.overlay
	w := min(max(m.w*2/3, 50), m.w-4)
	inner := w - 4
	// The title takes one line, for listY.
	lines := []string{truncate(headerStyle.Render(ov.title), inner)}
	if ov.kind == overlayQuit {
		lines = append(lines, "", dimStyle.Render("q/enter quit · esc stay"))
	} else {
		if ov.hasInput() {
			ov.input.SetWidth(inner - 2)
			lines = append(lines, ov.input.View())
		}
		rows, start := m.overlayRows(inner)
		listY = len(lines) + 1 - start // 1 for the border
		lines = append(lines, rows...)
	}
	box = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("8")).
		Padding(0, 1).
		Width(w).
		Render(strings.Join(lines, "\n"))
	x = (m.w - lipgloss.Width(box)) / 2
	y = max((m.h-lipgloss.Height(box))/3, 1)
	return box, x, y, listY
}

// overlayRows renders the rows of the list of the overlay that fit the
// screen, inner columns wide. start is the index of the first row.
func (m *model) overlayRows(inner int) (rows []string, start int) {
	ov := m.overlay
	maxRows := min(len(ov.shown), max(m.h-10, 1))
	start = max(0, ov.cur-maxRows+1)
	for i := start; i < len(ov.shown) && i < start+maxRows; i++ {
		it := ov.items[ov.shown[i]]
		tag := ""
		if it.tag != "" {
			tag = "  " + cursorStyle.Render(it.tag)
		}
		if ov.kind == overlayCommits && ov.mark == ov.shown[i] {
			tag += "  " + cursorStyle.Render("range start")
		}
		text := termtext.Quote(it.label) + "  " + dimStyle.Render(termtext.Quote(it.detail)) + tag
		row := truncate(text, inner-2)
		if i == ov.cur {
			row = m.selStyle().Width(inner).Render("▶ " + row)
		} else {
			row = "  " + row
		}
		rows = append(rows, row)
	}
	if len(ov.shown) == 0 && ov.kind != overlayGoto && ov.kind != overlayFind {
		rows = append(rows, dimStyle.Render("  nothing matches"))
	}
	return rows, start
}
