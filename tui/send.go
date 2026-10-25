package tui

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/rednafi/rexdiff/internal/clipboard"
	"github.com/rednafi/rexdiff/rex"
)

// maxSend is the largest review, in bytes, that rexdiff pastes into an
// agent. The user must copy a larger review.
const maxSend = 256 << 10

// listAgents returns a command that lists the agents of the session, other
// than rexdiff panes. The command returns msg with the agents. Set
// msg.send to send the review to them.
func (m *model) listAgents(msg agentsMsg) tea.Cmd {
	c, ctx := m.rex, m.ctx
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(ctx, rexTimeout)
		defer cancel()
		agents, err := c.Agents(ctx)
		isRexdiff := func(a rex.Agent) bool { return a.Status.App == appName }
		agents = slices.DeleteFunc(agents, isRexdiff)
		msg.agents, msg.err = agents, err
		return msg
	}
}

// rexTimeout is the longest time of the calls to Rex for one action. A
// Rex that does not answer then fails the action.
const rexTimeout = 10 * time.Second

// inRepo tells if dir is in the repository. An empty dir is unknown, and
// counts as in the repository.
func (m *model) inRepo(dir string) bool { return dir == "" || m.repo.Contains(dir) }

// agent returns the state of the agent in block. If the block is new, agent
// adds it.
func (m *model) agent(block string) *agentState {
	a := m.agents[block]
	if a == nil {
		a = &agentState{}
		m.agents[block] = a
	}
	return a
}

// onEvent keeps the state of the agent in the block of e, and follows the
// turns. After a reconnect, it forgets the agents and lists them again,
// as herdr-reviewr does after a lost connection. Nobody sees the agents
// in the gap, so the next state starts no turn.
func (m *model) onEvent(e rex.Event) tea.Cmd {
	if e.Reconnect {
		m.tracker.Gap()
		clear(m.agents)
		m.sampled = false
		return m.listAgents(agentsMsg{})
	}
	if e.Closed {
		delete(m.agents, e.Block)
	} else {
		app, state := e.Status.App, e.Status.State
		m.log.Debug("status", "block", e.Block, "app", app, "state", state)
		a := m.agent(e.Block)
		a.app, a.state, a.since = app, state, cmp.Or(e.Status.At, time.Now())
	}
	return m.observeTurn()
}

// member is an agent that works in the repository.
type member struct {
	block, app string
	state      rex.State
	since      time.Time // when Rex got the state
}

// members returns the agents that work in the repository, in the order
// of their blocks. As in herdr-reviewr, an agent is a member if its
// working directory is absolute and in this working tree. If the place of
// an agent is not known, members returns false, and the block of the
// agent if Rex can tell its place.
func (m *model) members() (ms []member, ask string, ok bool) {
	for _, block := range slices.Sorted(maps.Keys(m.agents)) {
		a := m.agents[block]
		if a.state == "" || a.app == appName || (m.rex != nil && block == m.rex.Block) {
			continue
		}
		if !a.hasCwd {
			if m.rex == nil {
				return nil, "", false
			}
			return nil, block, false
		}
		in, err := m.owns(a.cwd)
		if err != nil {
			m.log.Warn("agent membership failed", "err", err)
			return nil, "", false
		}
		if in {
			ms = append(ms, member{block, a.app, a.state, a.since})
		}
	}
	return ms, "", true
}

// askCwd returns a command that asks Rex for the working directory of the
// agent in block. The answer comes as a cwdMsg.
func (m *model) askCwd(block string) tea.Cmd {
	a := m.agents[block]
	if a.asking {
		return nil
	}
	a.asking = true
	c, ctx := m.rex, m.ctx
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(ctx, rexTimeout)
		defer cancel()
		cwd, err := c.Cwd(ctx, block)
		return cwdMsg{block, cwd, err}
	}
}

// onCwd keeps the working directory of an agent, and follows the turns
// with it. After an error, the next state asks again.
func (m *model) onCwd(msg cwdMsg) tea.Cmd {
	a := m.agents[msg.block]
	if a == nil {
		return nil // the block closed
	}
	a.asking = false
	if msg.err != nil {
		m.log.Debug("agent cwd failed", "block", msg.block, "err", msg.err)
		return nil
	}
	a.cwd, a.hasCwd = msg.cwd, true
	return m.observeTurn()
}

// owns tells if dir is in this working tree. It keeps the directories
// that are, because their top directory does not move.
func (m *model) owns(dir string) (bool, error) {
	if m.memberDirs[dir] {
		return true, nil
	}
	in, err := m.repo.Owns(m.ctx, dir)
	if in {
		if m.memberDirs == nil {
			m.memberDirs = map[string]bool{}
		}
		m.memberDirs[dir] = true
	}
	return in, err
}

// observeTurn gives the state of the repository to the tracker, and
// follows the start and the end of a turn.
func (m *model) observeTurn() tea.Cmd {
	if !m.sampled {
		return nil
	}
	ms, ask, ok := m.members()
	if ask != "" {
		return m.askCwd(ask)
	}
	if !ok {
		return nil // keep the last state, as herdr-reviewr does
	}
	here := len(ms) > 0
	if m.agentsHere == nil || *m.agentsHere != here {
		m.agentsHere = &here
		if m.scope == ScopeLastTurn && m.turn == nil {
			m.reload() // show the new wait message
		}
	}
	states := make([]rex.State, len(ms))
	for i, a := range ms {
		states[i] = a.state
	}
	started, ended := m.tracker.Observe(states...)
	if started {
		m.startTurn(ms)
	}
	if ended {
		m.endTurn()
	}
	return nil
}

// turnWaitMessage tells why the last-turn scope has no turn. The messages
// come from herdr-reviewr.
func (m *model) turnWaitMessage() string {
	if m.raced {
		return "the last turn started before rexdiff could snapshot it"
	}
	if m.agentsHere != nil && !*m.agentsHere {
		return "no agent works here"
	}
	return "waiting for the first turn"
}

// raceBefore is how long before Rex gets the working state the agent can
// already write, as in herdr-reviewr.
const raceBefore = time.Second

// startTurn takes a snapshot of the working tree at the start of a turn
// of the members ms. The turn stays pending until it changes a file. As in
// herdr-reviewr, a write from raceBefore the start to the snapshot can be
// in the snapshot. Then the turn is not known, and the scope says so.
func (m *model) startTurn(ms []member) {
	m.pending = nil
	tree, err := m.repo.Snapshot(m.ctx)
	if err != nil {
		m.log.Warn("turn snapshot failed", "err", err)
		return
	}
	snapped := time.Now()
	i := slices.IndexFunc(ms, func(a member) bool { return a.state == rex.StateWorking })
	if m.watch != nil {
		w := m.watch.LastWrite()
		if !w.Before(ms[i].since.Add(-raceBefore)) && !w.After(snapped) {
			m.log.Debug("turn raced", "block", ms[i].block, "write", w)
			m.dropTurn()
			return
		}
	}
	m.log.Debug("turn start", "block", ms[i].block, "tree", tree)
	m.pending = &turnState{
		block:   ms[i].block,
		app:     ms[i].app,
		tree:    tree,
		running: true,
		at:      time.Now(),
	}
}

// checkTurn makes the pending turn the last turn if the working tree
// differs from its start. Else the last turn stays.
func (m *model) checkTurn() {
	tree, err := m.repo.Snapshot(m.ctx)
	if err != nil {
		m.log.Warn("turn change check failed", "err", err)
		return
	}
	if tree != m.pending.tree {
		m.promote()
	}
}

// dropTurn forgets the last turn, because a new turn starts that is not
// known.
func (m *model) dropTurn() {
	m.turn, m.raced = nil, true
	if err := m.repo.SaveTurn(m.ctx, ""); err != nil {
		m.log.Warn("turn baseline ref delete failed", "err", err)
	}
	if m.scope == ScopeLastTurn {
		m.reload()
	}
}

// promote makes the pending turn the last turn, and saves its tree.
func (m *model) promote() {
	m.turn, m.pending, m.raced = m.pending, nil, false
	if err := m.repo.SaveTurn(m.ctx, m.turn.tree); err != nil {
		m.log.Warn("turn baseline ref write failed", "err", err)
	}
	if m.scope == ScopeLastTurn {
		m.reload()
	}
}

// endTurn ends the turn. A pending turn that changes the working tree
// becomes the last turn. After the end, a change is not from the turn.
func (m *model) endTurn() {
	if m.pending != nil {
		m.checkTurn()
		m.pending = nil
	}
	if m.turn != nil {
		m.turn.running = false
	}
	if m.scope == ScopeLastTurn {
		m.reload()
	}
}

// onAgents keeps the states of the agents. For a send, onAgents sends the
// review to the agent, or lets the user pick one.
func (m *model) onAgents(msg agentsMsg) tea.Cmd {
	for _, a := range msg.agents {
		*m.agent(a.Block) = agentState{
			app:    a.Status.App,
			state:  a.Status.State,
			since:  cmp.Or(a.Status.At, time.Now()),
			cwd:    a.Cwd,
			hasCwd: true,
		}
	}
	switch {
	case msg.err == nil:
		m.sampled = true
	case !m.sampled && !msg.send:
		// Without a list, the turns do not start. Thus list again later.
		m.log.Warn("agent list failed", "err", msg.err)
		m.sampleRetry.fail(time.Now())
	}
	// A send drops the question for a cwd. The next state asks again.
	turn := m.observeTurn()
	if !msg.send {
		return turn
	}
	m.sends--
	if msg.err != nil {
		m.flashCopy(msg.err.Error())
		return nil
	}
	// Prefer the agents in this repository. Else use all the agents of the
	// session.
	var here []rex.Agent
	for _, a := range msg.agents {
		if m.inRepo(a.Cwd) {
			here = append(here, a)
		}
	}
	if len(here) == 0 {
		here = msg.agents
	}
	switch {
	case len(here) == 0:
		m.flashCopy("no agent in this session")
		return nil
	case len(here) == 1:
		return m.deliver(here[0])
	case m.mode != modeNormal && !m.inCommentList():
		// The user can open the comment box or an overlay while the agents
		// load. A picker on top can hide the box and lose the comment. The
		// picker can take the place of the comment list, as in
		// herdr-reviewr.
		m.flash("several agents: close the box, then press s to pick one")
		return nil
	}
	m.openAgentPicker(here)
	return nil
}

// commentIDs returns the IDs of the comments.
func (m *model) commentIDs() []int {
	var ids []int
	for _, c := range m.comments.All() {
		ids = append(ids, c.ID)
	}
	return ids
}

// reviewText returns the text of the review that a send pastes.
func (m *model) reviewText() string {
	return m.comments.Review(m.cfg.ReviewPrefix)
}

// deliver returns a command that pastes the review into agent a. It checks
// the agent again before the paste, because a prompt of the agent can take
// the text.
func (m *model) deliver(a rex.Agent) tea.Cmd {
	m.sends++
	c, ctx := m.rex, m.ctx
	text, ids := m.reviewText(), m.commentIDs()
	focus := m.cfg.focusAgent()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(ctx, rexTimeout)
		defer cancel()
		status, err := c.Status(ctx, a.Block)
		if err != nil {
			return sentMsg{agent: a, err: fmt.Errorf("check %s: %w", a.Name, err)}
		}
		if status == (rex.Status{}) {
			return sentMsg{agent: a, err: fmt.Errorf("%s closed", a.Name)}
		}
		if status.State == rex.StateBlocked {
			err := fmt.Errorf("answer %s's prompt first", a.Name)
			return sentMsg{agent: a, err: err}
		}
		if err := c.Paste(ctx, a.Block, text); err != nil {
			return sentMsg{agent: a, err: err}
		}
		if focus {
			// Ignore the error, because the paste works.
			_ = c.Focus(ctx, a.Block)
		}
		return sentMsg{agent: a, ids: ids}
	}
}

// onSent removes the sent comments if the send worked. The comments that
// the user adds or changes during the send stay. A saved edit gives a
// comment a new ID, and msg does not have that ID.
func (m *model) onSent(msg sentMsg) {
	m.sends--
	if msg.err != nil {
		m.flashCopy(msg.err.Error())
		return
	}
	m.clearComments(msg.ids)
	m.lastSent = msg.agent.Block
	sent := plural(len(msg.ids), "comment")
	m.flash(fmt.Sprintf("sent %s to %s", sent, msg.agent.Name))
}

// startSend lists the agents to send the review to.
func (m *model) startSend() tea.Cmd {
	switch {
	case m.comments.Len() == 0:
		m.flash("no comments yet")
		return nil
	case len(m.reviewText()) > maxSend:
		m.flashCopy("review too large to send")
		return nil
	case m.rex == nil:
		m.flashCopy("not inside Rex")
		return nil
	}
	m.flash("sending…")
	m.sends++
	return m.listAgents(agentsMsg{send: true})
}

// copyComments copies the review to the clipboard. A good copy clears
// the comments, as a send does.
func (m *model) copyComments() {
	if m.comments.Len() == 0 {
		m.flash("no comments yet")
		return
	}
	ids := m.commentIDs()
	m.copyText(m.reviewText(), "copied "+plural(len(ids), "comment"), ids)
}

// copyTimeout is the longest time of a copy.
const copyTimeout = 2 * time.Second

// copyText writes text to the clipboard. If the copy works, it removes
// the comments ids and shows done. As in herdr-reviewr, the copy runs
// before the next frame, so two copies keep their order. A clipboard tool
// that does not answer fails after copyTimeout.
func (m *model) copyText(text, done string, ids []int) {
	ctx, cancel := context.WithTimeout(m.ctx, copyTimeout)
	defer cancel()
	err := clipboard.Write(ctx, text)
	switch {
	case errors.Is(err, clipboard.ErrNoTool):
		m.flash("copy failed: install wl-clipboard, xclip, or xsel")
	case err != nil:
		m.log.Warn("copy failed", "err", err)
		m.flash("copy failed")
	default:
		m.clearComments(ids)
		m.flash(done)
	}
}

// inCommentList tells if the comment list is open.
func (m *model) inCommentList() bool {
	return m.mode == modeOverlay && m.overlay != nil && m.overlay.kind == overlayComments
}

// clearComments removes the comments ids after a send or a copy. An open
// comment list shows the rest, or closes if no comment stays.
func (m *model) clearComments(ids []int) {
	for _, id := range ids {
		m.comments.Remove(id)
	}
	if m.inCommentList() {
		if m.comments.Len() == 0 {
			m.closeOverlay()
			return
		}
		m.fillCommentList(m.overlay)
	}
}

// flashCopy shows why the review cannot go to an agent, and tells the
// user to copy it.
func (m *model) flashCopy(why string) { m.flash(why + ", press y to copy") }

// plural returns n and word, with an s if n is not 1.
func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}
