package tui

import (
	"os"
	"strings"
	"testing"

	"github.com/rednafi/rexdiff/rex"
)

// agentPicker returns a test model with a comment and the agent picker
// open on three agents. The agent beside the pane is b2. It also returns
// the log of the fake rex.
func agentPicker(t *testing.T) (model, string) {
	t.Helper()
	m, dir := newTestModel(t)
	log := logRex(t, &m)
	m.beside = "b2"
	m = press(t, m, "]", "c", "h", "i", "enter", "s")
	m = step(t, m, agentsMsg{send: true, agents: []rex.Agent{
		{Block: "b1", Name: "pi", Window: "main", Cwd: dir, Status: rex.Status{App: "pi", State: rex.StateIdle}},
		{Block: "b2", Name: "agent", Cwd: dir, Status: rex.Status{App: "claude-code", State: rex.StateBlocked}},
		{Block: "b3", Name: "cc", Cwd: dir, Status: rex.Status{App: "codex", State: rex.StateWorking}},
	}})
	if m.overlay == nil || m.overlay.kind != overlayAgents {
		t.Fatalf("a send to three agents: overlay %v, want the agent picker", m.overlay)
	}
	return m, log
}

// TestAgentPicker checks the rows of the agent picker, and that a digit or
// enter sends to the agent of the row.
func TestAgentPicker(t *testing.T) {
	m, _ := agentPicker(t)
	for _, want := range []string{
		"send to which agent?",
		"1  pi", "pi · idle · main",
		"2  agent", "claude-code · at a prompt", "opened beside",
		"3  cc", "codex · working",
	} {
		if !shows(m, want) {
			t.Errorf("the picker got:\n%s\nwant %q", view(m), want)
		}
	}
	if m.overlay.cur != 1 {
		t.Errorf("the cursor is on row %d, want row 1, the agent beside", m.overlay.cur)
	}
	if m = press(t, m, "9"); m.overlay == nil {
		t.Error("9 with three agents closes the picker, want no pick")
	}

	tests := []struct{ name, keys, block string }{
		{"digit", "3", "b3"},
		{"enter", "enter", "b2"},
		{"move and enter", "j enter", "b3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, log := agentPicker(t)
			keys := strings.Fields(tt.keys)
			m = press(t, m, keys[:len(keys)-1]...)
			next, cmd := m.Update(keyMsg(t, keys[len(keys)-1]))
			if m = *next.(*model); m.overlay != nil || cmd == nil {
				t.Fatalf("after %s: overlay %v, command %v; want a send", tt.keys, m.overlay, cmd != nil)
			}
			cmd() // the send checks the agent first
			b, err := os.ReadFile(log)
			want := "block call -s S -b " + tt.block + " program_status"
			if err != nil || !strings.Contains(string(b), want) {
				t.Errorf("rex calls %q (%v), want %s", b, err, want)
			}
		})
	}
}
