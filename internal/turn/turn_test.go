package turn

import (
	"testing"

	"github.com/rednafi/rexdiff/rex"
)

// The tests come from turn.rs of herdr-reviewr.

// TestFold checks how the states of the agents fold into one state.
func TestFold(t *testing.T) {
	tests := []struct {
		name   string
		states []rex.State
		want   state
	}{
		// No agent rests, so a fresh pane tracks the next turn it sees.
		{"no agent", nil, resting},
		{"idle and working", []rex.State{rex.StateIdle, rex.StateWorking}, working},
		{"blocked and working", []rex.State{rex.StateBlocked, rex.StateWorking}, working},
		{"idle and done", []rex.State{rex.StateIdle, rex.StateDone}, resting},
		{"blocked and idle", []rex.State{rex.StateBlocked, rex.StateIdle}, held},
		// An unknown state never rests, so it starts no turn.
		{"unknown", []rex.State{"compacting"}, held},
	}
	for _, tt := range tests {
		if got := fold(tt.states...); got != tt.want {
			t.Errorf("%s: fold(%q) = %v, want %v", tt.name, tt.states, got, tt.want)
		}
	}
}

// step is one state for a Tracker and the edges that it gives.
type step struct {
	s              state
	started, ended bool
}

// run gives the states of steps to a new Tracker, and checks the edges.
func run(t *testing.T, steps ...step) {
	t.Helper()
	var tr Tracker
	for i, st := range steps {
		started, ended := tr.observe(st.s)
		if started != st.started || ended != st.ended {
			t.Errorf("step %d (%v): started %v, ended %v; want %v, %v",
				i, st.s, started, ended, st.started, st.ended)
		}
	}
}

func TestEmptyRepoStartsOnFirstWorker(t *testing.T) {
	var tr Tracker
	tr.Observe()
	if started, _ := tr.Observe(rex.StateWorking); !started {
		t.Error("the first worker after no agent starts no turn")
	}
}

func TestStartAfterRest(t *testing.T) {
	// The first state never starts a turn.
	run(t, step{resting, false, false}, step{working, true, false})
}

func TestHeldBackToWorkContinues(t *testing.T) {
	// A prompt in the middle of a turn does not start another turn.
	run(t,
		step{resting, false, false},
		step{working, true, false},
		step{held, false, false},
		step{working, false, false},
	)
}

func TestEndOnlyAfterWork(t *testing.T) {
	run(t,
		step{resting, false, false}, // no work before, so no end
		step{working, true, false},  // a start is never an end
		step{held, false, false},    // a pause, not an end
		step{working, false, false},
		step{resting, false, true},
	)
}

func TestEndThroughHeld(t *testing.T) {
	// The prompt keeps the turn open. A rest after it ends the turn once.
	run(t,
		step{resting, false, false},
		step{working, true, false},
		step{held, false, false},
		step{resting, false, true},
		step{resting, false, false},
	)
}

func TestLoneFirstWorkingStartsNothing(t *testing.T) {
	// The start of this turn is not seen.
	run(t, step{working, false, false})
}

func TestGap(t *testing.T) {
	var tr Tracker
	tr.Observe(rex.StateIdle)
	tr.Gap()
	if started, _ := tr.Observe(rex.StateWorking); started {
		t.Error("the first state after a gap starts a turn")
	}
}
