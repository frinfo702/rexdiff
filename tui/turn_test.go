package tui

import (
	"slices"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/rednafi/rexdiff/rex"
)

// turnModel returns a test model in the last-turn scope, after the first
// list of the agents with pi at rest in the repository.
func turnModel(t *testing.T) (model, string) {
	t.Helper()
	m, dir := newTestModel(t)
	m = press(t, m, "t")
	return step(t, m, sample("p", "pi", dir)), dir
}

// TestTurnNeedsSample checks that a state before the first list of the
// agents starts no turn. The start of that turn is not seen.
func TestTurnNeedsSample(t *testing.T) {
	m, dir := newTestModel(t)
	m.agents["p"] = &agentState{cwd: dir, hasCwd: true}
	m = step(t, m, status("p", "pi", rex.StateIdle))
	m = step(t, m, status("p", "pi", rex.StateWorking))
	if m.pending != nil {
		t.Errorf("pending %+v before the first list, want none", m.pending)
	}
	// The first list sees pi at work. That is no start.
	working := sample("p", "pi", dir)
	working.agents[0].Status.State = rex.StateWorking
	m = step(t, m, working)
	if m.pending != nil {
		t.Errorf("pending %+v after a first list at work, want none", m.pending)
	}
}

// TestTurnReconnect checks that a reconnect of rex events forgets the
// agents and lists them again. A list at work after the gap starts no
// turn, because the start is not seen.
func TestTurnReconnect(t *testing.T) {
	m, dir := turnModel(t)
	next, cmd := m.Update(eventMsg{Reconnect: true})
	m = *next.(*model)
	if cmd == nil || m.sampled || len(m.agents) != 0 {
		t.Fatalf("after a reconnect: cmd %v, sampled %v, agents %d; want a list, no agents",
			cmd != nil, m.sampled, len(m.agents))
	}
	working := sample("p", "pi", dir)
	working.agents[0].Status.State = rex.StateWorking
	if m = step(t, m, working); m.pending != nil {
		t.Errorf("pending %+v after a list at work after a gap, want none", m.pending)
	}
}

// TestTurnWithoutChange checks that a turn that changes no file does not
// become the last turn.
func TestTurnWithoutChange(t *testing.T) {
	m, _ := turnModel(t)
	m = step(t, m, status("p", "pi", rex.StateWorking))
	if m.pending == nil {
		t.Fatal("no pending turn after the start")
	}
	m = step(t, m, status("p", "pi", rex.StateIdle))
	if m.turn != nil || m.pending != nil {
		t.Errorf("turn %+v, pending %+v, want none", m.turn, m.pending)
	}
	if m.changes.note != "waiting for the first turn" {
		t.Errorf("note %q, want \"waiting for the first turn\"", m.changes.note)
	}
}

// TestTurnKeepsOldBaseline checks that a turn with no change keeps the
// last turn that changes a file.
func TestTurnKeepsOldBaseline(t *testing.T) {
	m, dir := turnModel(t)
	m = step(t, m, status("p", "pi", rex.StateWorking))
	write(t, dir, "c.go", "package a\n")
	m = step(t, m, status("p", "pi", rex.StateIdle))
	if m.turn == nil {
		t.Fatal("no turn after a change")
	}
	first := m.turn.tree
	m = step(t, m, status("p", "pi", rex.StateWorking))
	m = step(t, m, status("p", "pi", rex.StateIdle))
	if m.turn.tree != first {
		t.Error("a turn with no change replaces the last turn")
	}
	if got := paths(m); !slices.Equal(got, []string{"c.go"}) {
		t.Errorf("last turn shows %v, want [c.go]", got)
	}
}

// TestTurnPromotesOnChange checks that the first change in a turn makes
// it the last turn, before the turn ends.
func TestTurnPromotesOnChange(t *testing.T) {
	m, dir := turnModel(t)
	m = step(t, m, status("p", "pi", rex.StateWorking))
	m = step(t, m, tickMsg{})
	if m.turn != nil {
		t.Fatal("a tick with no change promotes the turn")
	}
	write(t, dir, "c.go", "package a\n")
	m.polled = time.Time{} // without a watcher, the poll sees the change
	m = step(t, m, tickMsg{})
	if m.turn == nil || !m.turn.running || m.pending != nil {
		t.Fatalf("turn %+v, pending %+v, want a running turn", m.turn, m.pending)
	}
	if m.repo.SavedTurn(t.Context()) != m.turn.tree {
		t.Error("the promoted turn is not saved")
	}
	if got := paths(m); !slices.Equal(got, []string{"c.go"}) {
		t.Errorf("last turn shows %v, want [c.go]", got)
	}
	if !shows(m, "pi working") {
		t.Errorf("view got:\n%s\nwant the running turn in the header", view(m))
	}
}

// TestTurnTwoAgents checks that a turn ends only when all the agents in
// the repository rest.
func TestTurnTwoAgents(t *testing.T) {
	m, dir := turnModel(t)
	m = step(t, m, agentsMsg{agents: []rex.Agent{
		{Block: "p", Cwd: dir, Status: rex.Status{App: "pi", State: rex.StateIdle}},
		{Block: "q", Cwd: dir, Status: rex.Status{App: "cc", State: rex.StateIdle}},
	}})
	m = step(t, m, status("p", "pi", rex.StateWorking))
	m = step(t, m, status("q", "cc", rex.StateWorking))
	write(t, dir, "c.go", "package a\n")
	m = step(t, m, status("p", "pi", rex.StateIdle))
	if m.pending == nil || m.turn != nil {
		t.Fatalf("turn %+v, pending %+v, want the turn still pending", m.turn, m.pending)
	}
	m = step(t, m, status("q", "cc", rex.StateBlocked))
	if m.pending == nil {
		t.Fatal("a blocked agent ends the turn")
	}
	m = step(t, m, status("q", "cc", rex.StateDone))
	if m.turn == nil || m.turn.app != "pi" || m.turn.running {
		t.Errorf("turn %+v, want the ended turn of pi", m.turn)
	}
}

// TestTurnOutsideAgent checks that an agent outside the repository starts
// no turn, and that the scope tells that no agent works here.
func TestTurnOutsideAgent(t *testing.T) {
	m, dir := newTestModel(t)
	m = press(t, m, "t")
	m = step(t, m, sample("p", "pi", t.TempDir()))
	if m.changes.note != "no agent works here" {
		t.Errorf("note %q, want \"no agent works here\"", m.changes.note)
	}
	m = step(t, m, status("p", "pi", rex.StateWorking))
	write(t, dir, "c.go", "package a\n")
	m = step(t, m, status("p", "pi", rex.StateIdle))
	if m.turn != nil || m.pending != nil {
		t.Errorf("turn %+v, pending %+v, want none", m.turn, m.pending)
	}
	// A relative or unknown directory is not in the repository.
	m = step(t, m, sample("p", "pi", "."))
	if m.changes.note != "no agent works here" {
		t.Errorf("relative cwd: note %q, want \"no agent works here\"", m.changes.note)
	}
}

// TestTurnUnknownCwdHolds checks that an agent whose directory is not
// known holds the state, so it does not end a turn.
func TestTurnUnknownCwdHolds(t *testing.T) {
	m, dir := turnModel(t)
	m = step(t, m, status("p", "pi", rex.StateWorking))
	write(t, dir, "c.go", "package a\n")
	// Outside Rex, the program cannot ask for the directory of block q.
	m = step(t, m, status("q", "cc", rex.StateIdle))
	m = step(t, m, status("p", "pi", rex.StateIdle))
	if m.pending == nil || m.turn != nil {
		t.Fatalf("turn %+v, pending %+v, want the turn still pending", m.turn, m.pending)
	}
	m = step(t, m, eventMsg{Block: "q", Closed: true})
	if m.turn == nil {
		t.Error("the turn does not end after the unknown agent closes")
	}
}

// tickAgents runs a tick of m and returns the agent lists that its
// commands give.
func tickAgents(t *testing.T, m *model) []agentsMsg {
	t.Helper()
	batch, _ := m.onTick()().(tea.BatchMsg)
	var out []agentsMsg
	for _, c := range batch {
		got := make(chan tea.Msg, 1)
		go func() { got <- c() }()
		select {
		case msg := <-got:
			if a, ok := msg.(agentsMsg); ok {
				out = append(out, a)
			}
		case <-time.After(300 * time.Millisecond): // the next tick
		}
	}
	return out
}

// TestSampleRetry checks that a failed first list of the agents is
// tried again after a wait that doubles, as in herdr-reviewr. Without
// the list, the turns do not start.
func TestSampleRetry(t *testing.T) {
	m, dir := newTestModel(t)
	logRex(t, &m) // its output is no list
	m = step(t, m, m.listAgents(agentsMsg{})())
	if m.sampled || m.sampleAt.IsZero() || m.sampleWait != 2*time.Second {
		t.Fatalf("after a failed list: sampled %v, at %v, wait %v; want a retry",
			m.sampled, m.sampleAt, m.sampleWait)
	}
	if got := tickAgents(t, &m); len(got) != 0 {
		t.Errorf("a tick before the wait lists %d times, want 0", len(got))
	}
	m.sampleAt = time.Now().Add(-time.Millisecond)
	got := tickAgents(t, &m)
	if len(got) != 1 || got[0].err == nil {
		t.Fatalf("a tick after the wait lists %d times, want 1 that fails", len(got))
	}
	if m = step(t, m, got[0]); m.sampleWait != 4*time.Second {
		t.Errorf("after a second failure: wait %v, want 4s", m.sampleWait)
	}
	m = step(t, m, sample("p", "pi", dir))
	m.sampleAt = time.Now().Add(-time.Millisecond)
	if !m.sampled || len(tickAgents(t, &m)) != 0 {
		t.Errorf("after a good list: sampled %v, want true and no more lists", m.sampled)
	}
}
