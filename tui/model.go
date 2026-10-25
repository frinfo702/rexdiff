package tui

import (
	"cmp"
	"context"
	"image/color"
	"log/slog"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"

	"github.com/rednafi/rexdiff/diff"
	"github.com/rednafi/rexdiff/internal/termtext"
	"github.com/rednafi/rexdiff/internal/turn"
	"github.com/rednafi/rexdiff/rex"
	"github.com/rednafi/rexdiff/update"
	"github.com/rednafi/rexdiff/vcs"
)

type tab int

const (
	tabChanges tab = iota
	tabFiles
	tabPR
)

// scopeCommits compares the commits that the user picks. It is not a
// value of Config.DefaultScope.
const scopeCommits Scope = "commits"

// scopes are the scopes in the order of the header.
var scopes = [...]Scope{ScopeUncommitted, ScopeBranch, ScopeLastTurn, scopeCommits}

// label returns the name of the scope in the header.
func (s Scope) label() string { return strings.ReplaceAll(string(s), "-", " ") }

// mode tells which part of the screen takes the keys.
type mode int

const (
	modeNormal mode = iota
	modeComment
	modeOverlay
	modeFind // the find band
)

// pane is one view that scrolls and takes comments. It has the rendered
// lines and the source lines that they show.
type pane struct {
	lines  []string
	rows   []diff.Row // rows[i] is the source line that lines[i] shows
	diff   *diff.Diff // the render for mouse hits, or nil for Markdown or PR
	path   string     // the file of the rows, or "" for a pane with no comments
	cur    int        // the row of the cursor
	top    int        // the first row on the screen
	anchor int        // the start of the visual selection, or -1
	xoff   int        // the sideways scroll, in columns
	note   string     // the text that shows if there are no lines
	// free is true after the wheel moves the view. Then the cursor can
	// be off the screen until the next key or click.
	free bool
}

// set shows lines, and rows tells which source lines they show. It keeps
// the cursor, the top, and the selection in the new lines.
func (p *pane) set(lines []string, rows []diff.Row) {
	p.lines, p.rows, p.diff = lines, rows, nil
	p.cur = min(p.cur, max(len(lines)-1, 0))
	p.top = min(p.top, max(len(lines)-1, 0))
	if p.anchor >= len(rows) { // the file is shorter than before
		p.anchor = -1
	}
}

// cont tells if line i continues the source line of the line above.
func (p *pane) cont(i int) bool { return i > 0 && i < len(p.rows) && p.rows[i].Continued }

// head returns the first line of the source line that line i shows.
func (p *pane) head(i int) int {
	for p.cont(i) {
		i--
	}
	return i
}

// end returns the last line of the source line that line i shows.
func (p *pane) end(i int) int {
	for p.cont(i + 1) {
		i++
	}
	return i
}

// toStart moves the cursor and the view to the first row. It clears the
// selection and the sideways scroll.
func (p *pane) toStart() { p.cur, p.top, p.anchor, p.xoff = 0, 0, -1, 0 }

// setDiff shows the lines of d.
func (p *pane) setDiff(d *diff.Diff) {
	p.set(d.Lines, d.Rows)
	p.diff = d
}

// side tells which side column x of the screen is on, and if x is on a line
// number.
func (p *pane) side(x int) (s diff.Side, onNumber bool) {
	if p.diff == nil {
		return diff.SideNew, false
	}
	return p.diff.SideAt(x)
}

// changed is a changed file with its line counts.
type changed struct {
	vcs.File
	adds, dels int
}

// turnState is a turn of the agents in the repository. It keeps a
// snapshot of the working tree from the start of the turn.
type turnState struct {
	block   string // the first agent that works in the turn
	app     string // the name of that agent, for example "pi"
	tree    string // the snapshot at the start of the turn
	running bool
	at      time.Time // the start of the turn
}

// dragState is a drag of the left mouse button in the main pane.
type dragState struct {
	active bool      // the button is down
	moved  bool      // the cursor has left the start row
	gutter bool      // the drag starts on the line numbers
	start  int       // the row where the drag starts
	pref   diff.Side // the side of the line numbers or the code at the start
}

// draft is the comment that the user writes. It is a new comment, or an
// edit of the comment with ID editID. An edit from the comment list goes
// back to the list at row listRow. Else listRow is -1.
type draft struct {
	diff.Comment
	editID  int
	listRow int
}

// prState is the content of the PR tab.
type prState struct {
	loading bool
	pr      *vcs.PullRequest
	err     string
}

// agentState is what the program knows about the agent in a block.
type agentState struct {
	app    string    // the name of the agent, for example "pi"
	state  rex.State // "" if the program sends no status
	cwd    string    // the working directory, or "" if Rex does not know it
	hasCwd bool      // the program has the answer of Rex for cwd
}

// model is the state of the program. It implements tea.Model.
type model struct {
	// ctx comes from Run. The methods of tea.Model have fixed signatures,
	// so the model keeps ctx for its git and Rex calls.
	ctx  context.Context
	repo *vcs.Repo
	rex  *rex.Client // nil outside Rex
	log  *slog.Logger

	cfg    config
	keys   keyMap
	notice string // a problem that the footer shows, or ""

	updater      Updater         // nil if rexdiff cannot update itself
	release      *update.Release // a newer release, or nil
	phase        updatePhase
	restartState string // the file of the state for the restart, or ""

	w, h    int
	bg, fg  color.Color
	red     color.Color // the ANSI red of the terminal, or nil
	green   color.Color // the ANSI green of the terminal, or nil
	dark    bool
	focused bool
	ticks   int

	// watch tells when the files or the refs change, or is nil. If it is
	// nil, each tick reloads the current tab.
	watch *vcs.Watcher
	// reloadOwed is true if a change is seen but the Changes tab does not
	// show it yet. A reload waits until reloadAt, after the pause of the
	// last one, and until retryAt, after a failure.
	reloadOwed bool
	filesOwed  bool // the same for the list of all files
	reloadAt   time.Time
	retryAt    time.Time
	retryWait  time.Duration // the wait after the next failure, or 0

	tab   tab
	scope Scope
	// folds has the folds that the user expands in the Changes tab, by path.
	folds map[string]*openFolds
	base  string
	// baseFlag is true if the base comes from the flag. Then the user
	// cannot pick another.
	baseFlag bool
	from     string // the commits scope: the oldest commit that the user picks
	to       string // the commits scope: the newest commit that the user picks

	// turn is the last turn that changes a file, or nil. The last-turn
	// scope compares with it. pending is a turn that starts but changes
	// no file yet, or nil.
	turn, pending *turnState
	tracker       turn.Tracker
	// sampled is true after the first list of the agents. An edge before
	// it can be wrong, so the tracker gets no states before it.
	sampled bool
	// agentsHere tells if an agent works in the repository, or is nil if
	// the program does not know.
	agentsHere *bool
	memberDirs map[string]bool // the working directories of the members

	files      []changed // the Changes tab
	changeSel  int       // the file that shows in the Changes tab
	shownTrees string    // the two trees of files, to notice a change
	changeTree navTree
	allFiles   []vcs.Entry // the Files tab
	allSel     int
	fileTree   navTree
	viewed     string // the content in the Files tab, to notice edits

	changes, viewer, prPane pane
	pr                      prState
	prf                     prFetch
	prPlace                 *savedPane // the place of prPane after a restart

	split      bool // show the diff side by side
	wrap       bool
	rendered   bool // show markdown rendered
	listPos    position
	listHidden bool
	focusList  bool // the keys j and k move in the file list
	// armed is 1 or -1 after a ] or a [ past the last change arms a
	// crossing to the file at armedPath. Else it is 0.
	armed     int
	armedPath string
	listW     int        // the width of the file list at a side, in columns
	listPct   int        // the share of the body rows, above or below
	cache     diff.Cache // the syntax colours, for a fast sideways scroll

	comments diff.Comments
	editor   textarea.Model
	draft    *draft
	mode     mode
	overlay  *overlay
	// recent has the files that the user picks in the search in this
	// session, the last pick first.
	recent     []string
	grepCancel context.CancelFunc // stops the code search that runs
	find       findState
	drag       dragState
	click      clickChain
	help       help.Model

	status     string
	statusTill time.Time

	agents   map[string]*agentState // by block
	sends    int                    // the steps of the sends that run
	lastSent string                 // the block of the last send
	beside   string                 // the agent block beside the pane
}

// newModel returns the model of the program with the options o and the
// settings cfg.
func newModel(ctx context.Context, o Options, cfg config) model {
	ta := textarea.New()
	ta.ShowLineNumbers = false
	ta.Prompt = "│ "
	ta.SetHeight(3)
	ta.KeyMap.InsertNewline.SetKeys(defaultKeys().Newline.Keys()...)

	m := model{
		ctx:      ctx,
		repo:     o.Repo,
		rex:      o.Rex,
		log:      o.Log,
		notice:   o.Notice,
		updater:  o.Updater,
		w:        120,
		h:        40,
		focused:  true,
		base:     o.Base,
		baseFlag: o.Base != "",
		wrap:     o.Wrap,
		editor:   ta,
		help:     newHelp(),
		agents:   map[string]*agentState{},
	}
	if m.log == nil {
		m.log = slog.New(slog.DiscardHandler)
	}
	// As in herdr-reviewr, the flag wins over the saved base, and a saved
	// base that does not resolve does not count.
	if m.base == "" {
		if b := o.Repo.SavedBase(ctx); b != "" {
			if _, err := o.Repo.Resolve(ctx, b); err == nil {
				m.base = b
			}
		}
	}
	if m.base == "" {
		m.base = o.Repo.DefaultBase(ctx)
	}
	if tree := o.Repo.SavedTurn(ctx); tree != "" {
		m.turn = &turnState{tree: tree}
	}
	if o.Rex != nil && o.Agent != "" {
		// If Rex does not know the agent, the picker selects no agent
		// first. The picker still lists all the agents.
		m.beside, _ = o.Rex.Resolve(ctx, o.Agent)
	}
	m.changes.anchor, m.viewer.anchor, m.prPane.anchor = -1, -1, -1
	m.cfg = cfg
	m.keys = defaultKeys()
	m.keys.applyConfig(cfg.Keybindings)
	m.listPos, m.listW, m.listPct = cfg.FileList, cfg.FileListWidth, listStackPct
	m.rendered = cfg.MarkdownView == markdownViewRendered
	// The flags win over the settings.
	m.split = cmp.Or(o.Layout, cfg.Layout) == LayoutSplit
	m.scope = cmp.Or(o.Scope, cfg.DefaultScope)
	return m
}

// These are the messages of the program.
type (
	tickMsg   struct{}
	agentsMsg struct {
		agents []rex.Agent
		err    error
		send   bool // true if the list is for a send, not to learn the states
	}
	sentMsg struct {
		agent rex.Agent
		ids   []int // the IDs of the sent comments
		err   error
	}
	copiedMsg struct {
		ids  []int  // the comments to remove, or nil
		done string // the status if the copy works
		err  error
	}
	grepMsg struct {
		query string
		hits  []vcs.Match
		more  bool // git finds more matches than hits has
		err   error
	}
	prMsg struct {
		gen int // the read, see prFetch
		pr  *vcs.PullRequest
		err error
	}
	eventMsg rex.Event
	// editorDoneMsg tells that the editor of path stops. A window editor
	// sends it only if it fails.
	editorDoneMsg struct {
		path   string
		err    error
		window bool
	}
)

// requestColors are the commands that ask the terminal for its colours.
// The replies change the tints of the diff.
var requestColors = []tea.Cmd{
	tea.RequestBackgroundColor,
	tea.RequestForegroundColor,
	requestPalette,
}

// tick returns a command that sends a tickMsg after d.
func tick(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return tickMsg{} })
}

// Init starts the ticks, asks for the colours of the terminal, and loads
// the pull request and the agents.
func (m *model) Init() tea.Cmd {
	cmds := []tea.Cmd{tick(time.Second), m.requestPR(true)}
	cmds = append(cmds, requestColors...)
	if m.rex != nil {
		cmds = append(cmds, m.listAgents(agentsMsg{}))
	}
	if m.updater != nil {
		cmds = append(cmds, m.checkRelease())
	}
	return tea.Batch(cmds...)
}

// flashTime is how long a flash shows in the footer. herdr-reviewr uses
// the same time.
const flashTime = 4 * time.Second

// flash shows s in the footer for a short time. The text can come from Rex
// or from git, so flash quotes it if it has control characters.
func (m *model) flash(s string) {
	m.status, m.statusTill = termtext.Quote(s), time.Now().Add(flashTime)
}

// trees returns the two trees that the Changes tab compares in the scope.
// If there is nothing to compare, note tells why.
func (m *model) trees() (from, to, note string, err error) {
	ctx := m.ctx
	switch m.scope {
	case scopeCommits:
		if m.to == "" {
			return "", "", "no commits picked: press G to pick one or a range", nil
		}
		if from, err = m.repo.Parent(ctx, m.from); err != nil {
			return "", "", "", err
		}
		to, err = m.repo.Tree(ctx, m.to)
		return from, to, "", err
	case ScopeLastTurn:
		if m.turn == nil {
			return "", "", m.turnWaitMessage(), nil
		}
		from = m.turn.tree
	case ScopeBranch:
		if from, err = m.repo.MergeBase(ctx, m.base); err != nil {
			return "", "", "", err
		}
	default:
		if from, err = m.repo.Head(ctx); err != nil {
			return "", "", "", err
		}
	}
	to, err = m.repo.Snapshot(ctx)
	return from, to, "", err
}

// reload reads the changes of the scope again, even if the trees do not
// change.
func (m *model) reload() {
	m.shownTrees = "" // no trees are "", so reloadIfChanged reads them
	m.reloadIfChanged()
}

// reloadIfChanged reads the changes of the scope again if the trees differ
// from the shown trees. After a failure, the next tick tries again after a
// wait.
func (m *model) reloadIfChanged() {
	m.reloadOwed = false
	from, to, note, err := m.trees()
	trees := from + ".." + to
	var files []vcs.File
	if err == nil && note == "" && trees != m.shownTrees {
		files, err = m.repo.Diff(m.ctx, from, to)
	}
	m.retry(err)
	if err == nil && note == "" && trees == m.shownTrees {
		return
	}
	if err != nil {
		note = err.Error()
	}
	if note != "" {
		m.changes.note, m.files, m.shownTrees = note, nil, ""
		m.buildTrees()
		m.renderChanges()
		return
	}
	var selPath string
	if f := m.changedFile(); f != nil {
		selPath = f.Path
	}
	m.files = make([]changed, len(files))
	m.shownTrees, m.changeSel, m.changes.note = trees, -1, ""
	for i, f := range files {
		c := &m.files[i]
		c.File = f
		if !f.Binary && !f.TooBig {
			c.adds, c.dels = diff.Count(f.Old, f.New)
		}
		if f.Path == selPath {
			m.changeSel = i
		}
	}
	m.buildTrees()
	if f := m.changedFile(); f == nil || f.Path != selPath {
		// Another file shows, so the cursor and the selection do not apply.
		m.changes.cur, m.changes.top, m.changes.anchor = 0, 0, -1
	}
	if len(m.files) == 0 {
		m.changes.note = "no changes"
	}
	m.renderChanges()
}

// loadFiles reads the list of all files if the model does not have it.
func (m *model) loadFiles() {
	if m.allFiles == nil {
		m.readFiles()
	}
}

// reloadFiles reads the list of all files again.
func (m *model) reloadFiles() {
	m.filesOwed = false
	m.readFiles()
	m.refreshViewer()
}

// readFiles reads the list of all files. The selected file stays. On an
// error, it keeps the old list and shows the error.
func (m *model) readFiles() {
	files, err := m.repo.Files(m.ctx)
	if err != nil {
		m.flash("refresh failed: " + err.Error())
		return
	}
	var sel string
	if m.allSel < len(m.allFiles) {
		sel = m.allFiles[m.allSel].Path
	}
	m.allFiles = m.withIgnored(files)
	m.allSel = slices.IndexFunc(m.allFiles, func(e vcs.Entry) bool { return e.Path == sel })
	m.buildFileTree()
}

// buildTrees makes the rows of the file lists after a change of the
// Changes tab. The Files tab marks the directories with a change.
func (m *model) buildTrees() {
	m.buildChangeTree()
	if m.allFiles != nil {
		m.buildFileTree()
	}
}

// Update handles a message. After an install, Update restarts the program
// when no work can get lost.
func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmd := m.handle(msg)
	if m.phase != phaseInstalled {
		return m, cmd
	}
	// A tick only starts the next tick, so it is no work.
	pending := cmd
	if _, ok := msg.(tickMsg); ok {
		pending = nil
	}
	if m.restartIfReady(pending) {
		return m, tea.Quit
	}
	return m, cmd
}

// handle does the work of Update. Update then checks for a restart.
func (m *model) handle(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	switch msg.(type) {
	case tea.KeyPressMsg, tea.MouseClickMsg: // these can move the cursor
		m.changes.free, m.viewer.free, m.prPane.free = false, false, false
	}
	switch msg.(type) {
	case tea.KeyPressMsg, tea.WindowSizeMsg: // these end a chain of clicks
		m.click = clickChain{}
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.editor.SetWidth(max(m.mainWidth()-8, 20))
		m.rerender()
	case tea.FocusMsg:
		m.focused = true
		m.reloadIfChanged()
	case tea.BlurMsg:
		m.focused = false
	case tickMsg:
		cmd = m.onTick()
	case tea.BackgroundColorMsg:
		if !sameColor(m.bg, msg.Color) {
			m.bg, m.dark = msg.Color, msg.IsDark()
			m.rerender()
		}
	case tea.ForegroundColorMsg:
		if !sameColor(m.fg, msg.Color) {
			m.fg = msg.Color
			m.rerender()
		}
	case uv.UnknownOscEvent:
		m.onPalette(string(msg))
	case agentsMsg:
		cmd = m.onAgents(msg)
	case sentMsg:
		m.onSent(msg)
	case copiedMsg:
		m.onCopied(msg)
	case eventMsg:
		m.onEvent(rex.Event(msg))
	case prMsg:
		cmd = m.onPR(msg)
	case grepMsg:
		m.onGrep(msg)
	case *update.Release:
		m.release = msg
	case installedMsg:
		m.onInstalled(msg)
	case editorDoneMsg:
		m.onEditorDone(msg)
	case tea.MouseClickMsg:
		return m.onClick(msg.Mouse())
	case tea.MouseMotionMsg:
		m.onDrag(msg.Mouse())
	case tea.MouseReleaseMsg:
		return m.onRelease()
	case tea.MouseWheelMsg:
		m.onWheel(msg.Mouse())
	case tea.PasteMsg:
		cmd = m.onPaste(msg.Content)
	case tea.KeyPressMsg:
		return m.onKey(msg)
	}
	m.ensureVisible()
	return cmd
}

// onKey gives a key to the part of the screen that takes the keys.
func (m *model) onKey(msg tea.KeyPressMsg) tea.Cmd {
	m.log.Debug("key", "key", msg.String(), "mode", m.mode, "scope", m.scope)
	// As in herdr-reviewr, a key other than ], [, or esc drops an armed
	// crossing. The keys of the prompt before a quit do not.
	inQuit := m.mode == modeOverlay && m.overlay != nil && m.overlay.kind == overlayQuit
	step := key.Matches(msg, m.keys.NextHunk, m.keys.PrevHunk)
	if !inQuit && !step && msg.Code != tea.KeyEscape {
		m.armed = 0
	}
	switch m.mode {
	case modeComment:
		return m.updateComment(msg)
	case modeOverlay:
		return m.updateOverlay(msg)
	case modeFind:
		return m.updateFind(msg)
	}
	return m.updateNormal(msg)
}

// onGrep shows the code matches of the search, but only if the search
// still shows their query.
func (m *model) onGrep(msg grepMsg) {
	ov := m.overlay
	if ov == nil || ov.kind != overlaySearch || ov.input.Value() != msg.query {
		return
	}
	s := &ov.search
	s.hits, s.hitsMore, s.err = msg.hits, msg.more, ""
	if msg.err != nil {
		// Show only the error, and no old results under it.
		s.hits, s.hitsMore, s.err = nil, false, msg.err.Error()
	}
	ov.showSearch()
}

// onPR shows the pull request that loadPR reads, unless a later read
// replaces it. After a read with a trailing request, onPR starts the next
// read.
func (m *model) onPR(msg prMsg) tea.Cmd {
	f := &m.prf
	if msg.gen != f.gen {
		return nil
	}
	f.inFlight, f.cancel = false, nil
	var next tea.Cmd
	if f.trailing {
		next = m.loadPR()
	}
	m.pr = prState{pr: msg.pr}
	if msg.err != nil {
		m.pr.err = msg.err.Error()
	}
	m.renderPR()
	// prPlace is from before a restart.
	if p := m.prPlace; p != nil && msg.err == nil && msg.pr != nil {
		m.prPlace = nil
		m.prPane.restore(*p)
	}
	return next
}

// onPaste adds text to the comment box or to the input of the overlay.
func (m *model) onPaste(text string) tea.Cmd {
	switch {
	case m.mode == modeComment:
		m.editor.InsertString(text)
	case m.mode == modeOverlay && m.overlay != nil && m.overlay.kind == overlayGoto:
		if n, ok := pastedLine(text); ok {
			m.overlay.input.SetValue(n)
		}
	case m.mode == modeOverlay && m.overlay != nil && m.overlay.hasInput():
		m.overlay.input.SetValue(m.overlay.input.Value() + text)
		return m.inputChanged()
	case m.mode == modeFind:
		m.find.input.SetValue(m.find.input.Value() + text)
	}
	return nil
}

// colorTicks is the interval of the requests for the colours of the
// terminal, in ticks of one second.
const colorTicks = 3

// The waits of a reload, as in herdr-reviewr.
const (
	// budgetFactor sets the pause after a reload on a tick. The next one
	// waits this many times as long as the last one took.
	budgetFactor = 20
	retryFirst   = time.Second // the first wait after a failure
	retryMax     = time.Minute // the longest wait after a failure
)

// retry sets the wait before the next reload after the error err. Each
// failure doubles the wait, from 1 s to 60 s. A reload with no error
// clears the wait.
func (m *model) retry(err error) {
	if err == nil {
		m.retryWait, m.retryAt = 0, time.Time{}
		return
	}
	wait := max(m.retryWait, retryFirst)
	m.retryWait = min(2*wait, retryMax)
	m.retryAt = time.Now().Add(wait)
	m.reloadOwed = true
}

// changed tells what the watcher sees. Without a watcher, or after an
// error of the watcher, it tells that both change.
func (m *model) changed() vcs.Change {
	all := vcs.Change{Files: true, Refs: true}
	if m.watch == nil {
		return all
	}
	start := time.Now()
	ch, err := m.watch.Changed(m.ctx)
	m.log.Debug("watch", "took", time.Since(start), "files", ch.Files, "refs", ch.Refs)
	if err != nil {
		m.log.Warn("watch failed", "err", err)
		return all
	}
	return ch
}

// onTick reloads the current tab if the watcher sees a change. On the
// Changes tab, a reload waits for the pause after the last one, and for
// the wait after a failure.
func (m *model) onTick() tea.Cmd {
	m.ticks++
	ch := m.changed()
	if ch.Files && m.pending != nil {
		m.checkTurn()
	}
	m.reloadOwed = m.reloadOwed || ch.Files || ch.Refs
	m.filesOwed = m.filesOwed || ch.Files
	now := time.Now()
	switch {
	case m.tab == tabChanges && m.reloadOwed && !now.Before(m.reloadAt) && !now.Before(m.retryAt):
		m.reloadIfChanged()
		m.reloadAt = time.Now().Add(budgetFactor * time.Since(now))
	case m.tab == tabFiles && m.filesOwed:
		m.reloadFiles() // reloadFiles renders the file too
	case m.tab == tabFiles:
		m.refreshViewer()
	}
	if m.status != "" && time.Now().After(m.statusTill) {
		m.status = ""
	}
	cmds := []tea.Cmd{tick(time.Second)}
	// Without a watcher, each tick reports a change of the refs. Then only
	// the poll reads the pull request.
	if ch.Refs && m.watch != nil {
		cmds = append(cmds, m.refsChanged())
	}
	if m.tab == tabPR && time.Since(m.prf.polled) >= prPoll {
		cmds = append(cmds, m.requestPR(false))
	}
	if m.ticks%colorTicks == 0 { // follow the changes of the terminal theme
		cmds = append(cmds, requestColors...)
	}
	return tea.Batch(cmds...)
}

// cur returns the pane of the current tab.
func (m *model) cur() *pane {
	switch m.tab {
	case tabFiles:
		return &m.viewer
	case tabPR:
		return &m.prPane
	}
	return &m.changes
}

// onPalette takes the red or the green from an OSC 4 reply.
func (m *model) onPalette(reply string) {
	i, c, ok := paletteReply(reply)
	if !ok {
		return
	}
	switch {
	case i == 1 && !sameColor(m.red, c):
		m.red = c
	case i == 2 && !sameColor(m.green, c):
		m.green = c
	default:
		return
	}
	m.rerender()
}

// sameColor reports whether a and b are the same colour. Nil is the same
// only as nil.
func sameColor(a, b color.Color) bool {
	if a == nil || b == nil {
		return a == b
	}
	ar, ag, ab, _ := a.RGBA()
	br, bg, bb, _ := b.RGBA()
	return ar == br && ag == bg && ab == bb
}
