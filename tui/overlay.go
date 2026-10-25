package tui

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/sahilm/fuzzy"

	"github.com/rednafi/rexdiff/diff"
	"github.com/rednafi/rexdiff/internal/search"
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
	overlayQuit
)

// These are the steps of the page keys in the commit picker, in rows, as
// in herdr-reviewr.
const (
	pickerPage = 15
	pickerHalf = 8
)

// pickItem is one row of the list of an overlay.
type pickItem struct {
	label  string
	detail string // the dim text after the label
	tag    string // a short mark at the end, for example "current"
	// id is what the row picks, for example a path, a ref, a commit, or a
	// block. In the search, a row with no id cannot have the cursor.
	id      string
	line    int  // the search: the line of a code match
	head    bool // the search: the header of the code matches of a file
	comment int  // the comment list: the ID of the comment
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
	search searchState
	rowsTo int // the index after the last row that shows
}

// searchState has the results of the search. As in herdr-reviewr, the
// search has two modes. It shows the files that match by name, or the
// lines of the code that contain the query.
type searchState struct {
	code      bool     // the mode that shows the code matches
	paths     []string // the files that the search can find
	files     []string // the files that match
	fileTotal int      // the number of the files that match
	hits      []vcs.Match
	hitsMore  bool   // git finds more matches than hits has
	err       string // the error of the code search
	preview   *preview
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
// agents, comments, and commits, and the prompt before a quit, have none.
func (o *overlay) hasInput() bool {
	switch o.kind {
	case overlayAgents, overlayComments, overlayCommits, overlayQuit:
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
func (o *overlay) down() { o.move(1) }
func (o *overlay) up()   { o.move(-1) }

// move moves the cursor of the list by d rows. It skips the rows that
// cannot have the cursor, and it stops at the ends.
func (o *overlay) move(d int) {
	step := 1
	if d < 0 {
		step = -1
	}
	for ; d != 0; d -= step {
		i := o.cur + step
		for i >= 0 && i < len(o.shown) && !o.pickable(i) {
			i += step
		}
		if i < 0 || i >= len(o.shown) {
			return
		}
		o.cur = i
	}
}

// pickable reports whether row i of the list can have the cursor. In the
// search, the headers of the files and the "… more" row cannot.
func (o *overlay) pickable(i int) bool {
	return o.kind != overlaySearch || o.items[o.shown[i]].id != ""
}

// showSearch shows the results of the search in its mode, with the
// cursor on the first result. As in herdr-reviewr, the code matches show
// under a header for each file, and a "… more" row tells that the list
// does not show all the results. An error shows in place of the results.
func (o *overlay) showSearch() {
	s := &o.search
	var items []pickItem
	switch {
	case s.err != "":
	case !s.code:
		for _, p := range s.files {
			items = append(items, pickItem{label: p, id: p})
		}
		if s.fileTotal > len(s.files) {
			items = append(items, pickItem{label: "… more"})
		}
	default:
		for i, h := range s.hits {
			if i == 0 || h.Path != s.hits[i-1].Path {
				items = append(items, pickItem{label: h.Path, head: true})
			}
			items = append(items, pickItem{
				label: fmt.Sprintf("%5d: %s", h.Line, h.Text),
				id:    h.Path,
				line:  h.Line,
			})
		}
		if s.hitsMore {
			items = append(items, pickItem{label: "… more"})
		}
	}
	o.setItems(items)
	o.cur = 0
	if len(o.shown) > 0 && !o.pickable(0) {
		o.move(1)
	}
}

// chips returns the chips of the modes of the search, as in
// herdr-reviewr: "files N │ code M". A "+" after M tells that git finds
// more matches. The chip of the mode in use is bright.
func (o *overlay) chips() string {
	s := &o.search
	files, code := "files", "code"
	if s.err == "" {
		files = fmt.Sprintf("files %d", s.fileTotal)
		code = fmt.Sprintf("code %d", len(s.hits))
		if s.hitsMore {
			code += "+"
		}
	}
	on := cursorStyle.Underline(true)
	if s.code {
		return dimStyle.Render(files+" │ ") + on.Render(code)
	}
	return on.Render(files) + dimStyle.Render(" │ "+code)
}

// open shows ov and gives the keys to it.
func (m *model) open(ov *overlay) {
	m.overlay, m.mode = ov, modeOverlay
}

// closeOverlay closes the overlay and gives the keys back to the panes.
func (m *model) closeOverlay() {
	m.overlay, m.mode = nil, modeNormal
	m.stopGrep()
}

// stopGrep stops the code search that runs, if any.
func (m *model) stopGrep() {
	if m.grepCancel != nil {
		m.grepCancel()
		m.grepCancel = nil
	}
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

// openSearch opens the search of the files and the code. With no query,
// it lists the files.
func (m *model) openSearch() {
	ov := newOverlay(overlaySearch, "")
	ov.input.Placeholder = "Search files and code…"
	for _, e := range m.allFiles {
		if !e.Ignored {
			ov.search.paths = append(ov.search.paths, e.Path)
		}
	}
	m.open(ov)
	m.runSearch()
}

// runSearch finds the files that match the query of the search, and
// starts a code search for the query. It stops the code search of the
// last query. Until the new code search ends, the old code matches show.
// An empty query finds no code.
func (m *model) runSearch() tea.Cmd {
	ov := m.overlay
	s, q := &ov.search, ov.input.Value()
	s.files, s.fileTotal = search.Files(s.paths, m.recent, q, search.FileLimit)
	s.err = ""
	m.stopGrep()
	if q == "" {
		s.hits, s.hitsMore = nil, false
		ov.showSearch()
		return nil
	}
	ov.showSearch()
	ctx, cancel := context.WithCancel(m.ctx)
	m.grepCancel = cancel
	repo := m.repo
	return func() tea.Msg {
		hits, more, err := repo.Grep(ctx, q, search.CodeLimit)
		return grepMsg{q, hits, more, err}
	}
}

// addRecent puts path first in the list of the files that the search
// shows for an empty query.
func (m *model) addRecent(path string) {
	rest := slices.DeleteFunc(m.recent, func(p string) bool { return p == path })
	m.recent = append([]string{path}, rest[:min(len(rest), search.FileLimit-1)]...)
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
// current base has a tag. As in herdr-reviewr, it does nothing if the base
// comes from the flag.
func (m *model) openBasePicker() {
	if m.baseFlag {
		return
	}
	ov := newOverlay(overlayBase, "base branch for the branch scope (or type any ref)")
	branches, err := m.repo.Branches(m.ctx)
	if err != nil {
		m.flash(err.Error())
		return
	}
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
// the commits scope, as in herdr-reviewr. It lists the first-parent
// commits after the fork point of the base, the newest first. On the base
// itself, or with no fork point, it lists the last commits. The current
// pick has the cursor and the anchor.
func (m *model) openCommitPicker() {
	if m.tab == tabPR {
		return
	}
	since, err := m.repo.ForkPoint(m.ctx, m.base)
	if head, herr := m.repo.Resolve(m.ctx, "HEAD"); err != nil || herr != nil || since == head {
		since = ""
	}
	commits, err := m.repo.Commits(m.ctx, since)
	if err != nil {
		m.flash(err.Error())
		return
	}
	title := fmt.Sprintf("commits · last %d", vcs.LastCommits)
	if since != "" {
		title = fmt.Sprintf("commits · %d over %s", len(commits), m.base)
	}
	ov := newOverlay(overlayCommits, title)
	var items []pickItem
	for _, c := range commits {
		items = append(items, pickItem{
			label:  c.Short + "  " + c.Subject,
			detail: c.Author + ", " + c.When,
			id:     c.SHA,
		})
	}
	ov.setItems(items)
	isSHA := func(sha string) func(pickItem) bool {
		return func(it pickItem) bool { return it.id == sha }
	}
	newest := slices.IndexFunc(items, isSHA(m.to))
	oldest := slices.IndexFunc(items, isSHA(m.from))
	if m.to != "" && newest >= 0 && oldest >= 0 {
		ov.cur = newest
		if oldest > newest {
			ov.mark = oldest
		}
	}
	m.open(ov)
}

// updateCommitPicker handles a key in the commit picker, as herdr-reviewr
// does. The key of Select sets or clears the anchor of a range. Esc clears
// the anchor, else it closes the picker.
func (m *model) updateCommitPicker(msg tea.KeyPressMsg) tea.Cmd {
	ov, k := m.overlay, m.keys
	switch {
	case key.Matches(msg, overlayKeys.Close) && ov.mark >= 0:
		ov.mark = -1
	case key.Matches(msg, overlayKeys.Close):
		m.closeOverlay()
	case key.Matches(msg, overlayKeys.Choose):
		return m.chooseOverlay()
	case key.Matches(msg, k.Down):
		ov.move(1)
	case key.Matches(msg, k.Up):
		ov.move(-1)
	case key.Matches(msg, k.PageDown):
		ov.move(pickerPage)
	case key.Matches(msg, k.PageUp):
		ov.move(-pickerPage)
	case key.Matches(msg, k.HalfDown):
		ov.move(pickerHalf)
	case key.Matches(msg, k.HalfUp):
		ov.move(-pickerHalf)
	case key.Matches(msg, k.Select) && ov.cur < len(ov.shown):
		if ov.mark == ov.shown[ov.cur] {
			ov.mark = -1
		} else {
			ov.mark = ov.shown[ov.cur]
		}
	}
	return nil
}

// commitHint returns the keys of the commit picker, as the footer of
// herdr-reviewr shows them.
func (m *model) commitHint() string {
	ov, k := m.overlay, m.keys
	if len(ov.shown) == 0 {
		return "esc cancel"
	}
	open, esc := "open", "cancel"
	if ov.mark >= 0 {
		esc = "clear"
		if n := max(ov.mark, ov.cur) - min(ov.mark, ov.cur) + 1; n > 1 {
			open = fmt.Sprintf("open %d", n)
		}
	}
	return fmt.Sprintf("enter %s · esc %s · %s %s move · %s select",
		open, esc, firstKey(k.Down), firstKey(k.Up), firstKey(k.Select))
}

// firstKey returns the first key of b, as herdr-reviewr names it, or "".
func firstKey(b key.Binding) string {
	if ks := b.Keys(); len(ks) > 0 {
		return herdrKeyName(ks[0])
	}
	return ""
}

// openCommentList lists the comments, to jump to one, edit one, delete
// one, or send or copy them all.
func (m *model) openCommentList() {
	if m.comments.Len() == 0 {
		m.flash("no comments yet")
		return
	}
	ov := newOverlay(overlayComments, "")
	m.fillCommentList(ov)
	m.open(ov)
}

// fillCommentList sets the comments as the items of ov, and the count in
// the title, as in herdr-reviewr.
func (m *model) fillCommentList(ov *overlay) {
	ov.title = fmt.Sprintf("Comments (%d)", m.comments.Len())
	var items []pickItem
	for _, c := range m.comments.All() {
		tag := ""
		if m.stale(c) {
			tag = "(stale)"
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
	switch ov.kind {
	case overlayQuit:
		return m.updateQuit(msg)
	case overlayComments:
		return m.updateCommentList(msg)
	case overlayCommits:
		return m.updateCommitPicker(msg)
	case overlaySearch:
		pv := ov.search.preview
		switch {
		case key.Matches(msg, overlayKeys.Flip):
			ov.search.code = !ov.search.code
			ov.showSearch()
			return nil
		case key.Matches(msg, overlayKeys.PreviewDown) && pv != nil:
			pv.scroll(pickerPage)
			return nil
		case key.Matches(msg, overlayKeys.PreviewUp) && pv != nil:
			pv.scroll(-pickerPage)
			return nil
		}
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
	if ov.kind == overlayGoto {
		if !lineKey(msg) {
			return nil
		}
		// "$" stands alone, and a digit after "$" starts a number.
		if msg.Text == "$" || (msg.Text != "" && ov.input.Value() == "$") {
			ov.input.SetValue("")
		}
	}
	var cmd tea.Cmd
	before := ov.input.Value()
	ov.input, cmd = ov.input.Update(msg)
	if ov.input.Value() != before {
		return tea.Batch(cmd, m.inputChanged())
	}
	return cmd
}

// updateQuit handles a key in the prompt before a quit, as herdr-reviewr
// does. The key of QuitDiscard quits. The key of Quit does nothing, so a
// held q does not quit. The keys of Send and Copy close the prompt and
// send or copy. Any other key closes the prompt.
func (m *model) updateQuit(msg tea.KeyPressMsg) tea.Cmd {
	k := m.keys
	switch {
	case key.Matches(msg, k.QuitDiscard):
		return tea.Quit
	case key.Matches(msg, k.Quit):
		return nil
	}
	m.closeOverlay()
	switch {
	case key.Matches(msg, k.Send):
		return m.startSend()
	case key.Matches(msg, k.Copy):
		return m.copyComments()
	}
	return nil
}

// updateCommentList handles a key in the comment list. The keys are those
// of herdr-reviewr. Enter also jumps to the comment.
func (m *model) updateCommentList(msg tea.KeyPressMsg) tea.Cmd {
	ov, k := m.overlay, m.keys
	it := ov.selected()
	switch {
	case key.Matches(msg, k.Comments, k.Cancel):
		m.closeOverlay()
	case key.Matches(msg, overlayKeys.Choose):
		return m.chooseOverlay()
	case key.Matches(msg, k.Down):
		ov.down()
	case key.Matches(msg, k.Up):
		ov.up()
	case key.Matches(msg, k.Send):
		return m.startSend()
	case key.Matches(msg, k.Copy):
		return m.copyComments()
	case key.Matches(msg, k.Edit) && it != nil:
		// As in herdr-reviewr, a comment on another diff takes no edit.
		c := m.comments.Find(it.comment)
		if c == nil || !m.revCurrent(c) {
			return nil
		}
		row := ov.cur
		m.closeOverlay()
		m.jumpToComment(c)
		m.focusList = false
		m.ensureVisible()
		return m.editComment(c, row)
	case key.Matches(msg, k.Delete) && it != nil:
		m.comments.Remove(it.comment)
		m.flash("comment deleted")
		if m.comments.Len() == 0 {
			m.closeOverlay()
			return nil
		}
		m.fillCommentList(ov)
	}
	return nil
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
		return m.runSearch()
	case overlayBase:
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
	case overlaySearch:
		m.closeOverlay()
		if it != nil && it.id != "" {
			m.addRecent(it.id)
			m.jumpTo(it.id, diff.SideNew, max(it.line, 1))
		}
	case overlayBase:
		ref, ok := m.baseRef(it)
		m.closeOverlay()
		if !ok {
			m.flash("no such ref: " + ref)
			return nil
		}
		if err := m.repo.SaveBase(m.ctx, ref); err != nil {
			m.flash(err.Error())
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
			m.jumpToComment(c)
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
	if ov.kind == overlaySearch {
		w = m.w - 4
	}
	inner := w - 4
	// The title takes one line, for listY.
	lines := []string{truncate(headerStyle.Render(ov.title), inner)}
	switch ov.kind {
	case overlayQuit:
		lines = append(lines, "", dimStyle.Render(m.quitHint()))
	case overlaySearch:
		lines, listY = m.searchLines(inner)
	default:
		if ov.hasInput() {
			ov.input.SetWidth(inner - 2)
			lines = append(lines, ov.input.View())
		}
		rows, start := m.overlayRows(inner, m.h-10)
		listY = len(lines) + 1 - start // 1 for the border
		lines = append(lines, rows...)
		switch ov.kind {
		case overlayComments:
			lines = append(lines, "", truncate(dimStyle.Render(m.listHint()), inner))
		case overlayCommits:
			lines = append(lines, "", truncate(dimStyle.Render(m.commitHint()), inner))
		}
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

// quitHint returns the keys of the prompt before a quit, as the footer
// of herdr-reviewr shows them.
func (m *model) quitHint() string {
	k, n := m.keys, m.comments.Len()
	return fmt.Sprintf("%s quit (%d pending) · esc cancel · %s send %d · %s copy",
		k.QuitDiscard.Help().Key, n, k.Send.Help().Key, n, k.Copy.Help().Key)
}

// listHint returns the keys of the comment list, as the footer of
// herdr-reviewr shows them. The edit key shows only if the selected
// comment takes an edit.
func (m *model) listHint() string {
	k := m.keys
	edit := ""
	if it := m.overlay.selected(); it != nil {
		if c := m.comments.Find(it.comment); c != nil && m.revCurrent(c) {
			edit = fmt.Sprintf(" · %s edit", k.Edit.Help().Key)
		}
	}
	return fmt.Sprintf("%s send %d · esc close · %s copy%s · %s delete · enter jump",
		k.Send.Help().Key, m.comments.Len(), k.Copy.Help().Key, edit,
		k.Delete.Help().Key)
}

// overlayRows renders at most limit rows of the list of the overlay,
// inner columns wide. start is the index of the first row.
func (m *model) overlayRows(inner, limit int) (rows []string, start int) {
	ov := m.overlay
	maxRows := min(len(ov.shown), max(limit, 1))
	start = max(0, ov.cur-maxRows+1)
	ov.rowsTo = start + maxRows
	for i := start; i < len(ov.shown) && i < start+maxRows; i++ {
		it := ov.items[ov.shown[i]]
		if !ov.pickable(i) {
			style := dimStyle
			if it.head {
				style = headerStyle
			}
			rows = append(rows, "  "+truncate(style.Render(termtext.Quote(it.label)), inner-2))
			continue
		}
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
	switch {
	case ov.search.err != "":
		rows = append(rows, delStyle.Render("  "+termtext.Quote(ov.search.err)))
	case len(ov.shown) == 0 && ov.kind == overlayCommits:
		rows = append(rows, dimStyle.Render("  no commits yet"))
	case len(ov.shown) == 0 && ov.kind == overlaySearch:
		// As in herdr-reviewr, an empty query finds no code, and that is
		// not a miss.
		if !ov.search.code || ov.input.Value() != "" {
			rows = append(rows, dimStyle.Render("  no matches"))
		}
	case len(ov.shown) == 0 && ov.kind != overlayGoto:
		rows = append(rows, dimStyle.Render("  no matches"))
	}
	return rows, start
}
