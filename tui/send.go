package tui

import (
	"fmt"
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/rednafi/rexdiff/rex"
)

// maxSend is the largest review that rexdiff pastes into an agent. Copy a
// larger review.
const maxSend = 256 << 10

// listAgents returns a command that lists the agents of the session, other
// than rexdiff panes. The command returns msg with the agents. Set
// msg.send to send the review to them.
func (m *model) listAgents(msg agentsMsg) tea.Cmd {
	c, ctx := m.rex, m.ctx
	return func() tea.Msg {
		agents, err := c.Agents(ctx)
		isRexdiff := func(a rex.Agent) bool { return a.App == appName }
		agents = slices.DeleteFunc(agents, isRexdiff)
		msg.agents, msg.err = agents, err
		return msg
	}
}

// inRepo tells if dir is in the repository. An empty dir is unknown, and
// counts as in the repository.
func (m *model) inRepo(dir string) bool { return dir == "" || m.repo.Contains(dir) }

// agent returns the state of the agent in block. It adds the block if it is
// new.
func (m *model) agent(block string) *agentState {
	a := m.agents[block]
	if a == nil {
		a = &agentState{}
		m.agents[block] = a
	}
	return a
}

// onEvent follows the turns of the agents. When an agent in this
// repository starts to work, onEvent takes a snapshot of the working tree.
// The "last turn" scope compares with this snapshot.
func (m *model) onEvent(e rex.Event) {
	if e.Closed {
		if a := m.agents[e.Block]; a != nil {
			a.state = ""
		}
		return
	}
	app, state := e.Status.App, e.Status.State
	m.log.Debug("status", "block", e.Block, "app", app, "state", state)
	a := m.agent(e.Block)
	prev := a.state
	a.state = state
	if app == appName || (m.rex != nil && e.Block == m.rex.Block) {
		return
	}
	if state == rex.StateWorking && prev != rex.StateWorking {
		m.startTurn(e.Block, app)
		return
	}
	done := state == rex.StateIdle || state == rex.StateDone || state == ""
	ended := m.turn != nil && m.turn.block == e.Block && prev == rex.StateWorking
	if ended && done {
		m.turn.running = false
		if m.scope == ScopeLastTurn {
			m.reload()
		}
	}
}

// startTurn takes a snapshot of the working tree when the agent app in
// block starts a turn in this repository.
func (m *model) startTurn(block, app string) {
	a := m.agent(block)
	if !a.hasCwd && m.rex != nil {
		a.cwd, _ = m.rex.Cwd(m.ctx, block) // no cwd: the agent is not in the repo
		a.hasCwd = true
	}
	if !m.inRepo(a.cwd) {
		return
	}
	tree, err := m.repo.Snapshot(m.ctx)
	if err != nil {
		return
	}
	m.turn = &turnState{
		block:   block,
		app:     app,
		tree:    tree,
		running: true,
		at:      time.Now(),
	}
	if m.scope == ScopeLastTurn {
		m.reload()
	}
}

// onAgents keeps the states of the agents. For a send, it sends the
// review to the agent, or lets the user pick one.
func (m *model) onAgents(msg agentsMsg) tea.Cmd {
	for _, a := range msg.agents {
		*m.agent(a.Block) = agentState{state: a.State, cwd: a.Cwd, hasCwd: true}
	}
	if !msg.send {
		return nil
	}
	m.sends--
	if msg.err != nil {
		m.flashCopy(msg.err.Error())
		return nil
	}
	// Prefer the agents in this repository. Else use all of the session.
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
	case m.mode != modeNormal:
		// The user began a comment or an overlay while the agents loaded.
		// A picker over it would hide it, and lose a comment in writing.
		m.flash("several agents: close the box, then press s to pick one")
		return nil
	}
	m.openAgentPicker(here)
	return nil
}

// reviewText returns the text of the review that a send pastes.
func (m *model) reviewText() string {
	return m.comments.Review(m.cfg.ReviewPrefix)
}

// deliver returns a command that pastes the review into agent a. Before
// the paste, it checks the agent again, because a prompt of the
// agent would take the text.
func (m *model) deliver(a rex.Agent) tea.Cmd {
	m.sends++
	c, ctx := m.rex, m.ctx
	text := m.reviewText()
	var ids []int
	for _, cm := range m.comments.All() {
		ids = append(ids, cm.ID)
	}
	focus := m.cfg.focusAgent()
	return func() tea.Msg {
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
			_ = c.Focus(ctx, a.Block) // best effort: the review is pasted
		}
		return sentMsg{agent: a, ids: ids}
	}
}

// onSent removes the comments that were sent, but only if the send
// succeeded. The comments that the user added or changed during the send
// stay: a saved edit gives a comment a new ID, which msg does not have.
func (m *model) onSent(msg sentMsg) {
	m.sends--
	if msg.err != nil {
		m.flashCopy(msg.err.Error())
		return
	}
	for _, id := range msg.ids {
		m.comments.Remove(id)
	}
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

// copyComments copies the review to the clipboard.
func (m *model) copyComments() tea.Cmd {
	if m.comments.Len() == 0 {
		m.flash("no comments yet")
		return nil
	}
	m.flash("copied " + plural(m.comments.Len(), "comment"))
	return tea.SetClipboard(m.reviewText())
}

// flashCopy flashes why the review cannot go to an agent, and tells the
// user to copy it.
func (m *model) flashCopy(why string) { m.flash(why + ", press y to copy") }

// plural returns n and word, with an s if n is not 1.
func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}
