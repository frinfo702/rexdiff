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
		want   State
	}{
		// No agent rests, so a fresh pane tracks the next turn it sees.
		{"no agent", nil, Resting},
		{"idle and working", []rex.State{rex.StateIdle, rex.StateWorking}, Working},
		{"blocked and working", []rex.State{rex.StateBlocked, rex.StateWorking}, Working},
		{"idle and done", []rex.State{rex.StateIdle, rex.StateDone}, Resting},
		{"blocked and idle", []rex.State{rex.StateBlocked, rex.StateIdle}, Held},
		// An unknown state never rests, so it starts no turn.
		{"unknown", []rex.State{"compacting"}, Held},
	}
	for _, tt := range tests {
		if got := Fold(tt.states...); got != tt.want {
			t.Errorf("%s: Fold(%q) = %v, want %v", tt.name, tt.states, got, tt.want)
		}
	}
}

// step is one state for a Tracker and the edges that it gives.
type step struct {
	s              State
	started, ended bool
}

// run gives the states of steps to a new Tracker, and checks the edges.
func run(t *testing.T, steps ...step) {
	t.Helper()
	var tr Tracker
	for i, st := range steps {
		started, ended := tr.Observe(st.s)
		if started != st.started || ended != st.ended {
			t.Errorf("step %d (%v): started %v, ended %v; want %v, %v",
				i, st.s, started, ended, st.started, st.ended)
		}
	}
}

func TestEmptyRepoStartsOnFirstWorker(t *testing.T) {
	run(t, step{Fold(), false, false}, step{Fold(rex.StateWorking), true, false})
}

func TestStartAfterRest(t *testing.T) {
	// The first state never starts a turn.
	run(t, step{Resting, false, false}, step{Working, true, false})
}

func TestHeldBackToWorkContinues(t *testing.T) {
	// A prompt in the middle of a turn does not start another turn.
	run(t,
		step{Resting, false, false},
		step{Working, true, false},
		step{Held, false, false},
		step{Working, false, false},
	)
}

func TestEndOnlyAfterWork(t *testing.T) {
	run(t,
		step{Resting, false, false}, // no work before, so no end
		step{Working, true, false},  // a start is never an end
		step{Held, false, false},    // a pause, not an end
		step{Working, false, false},
		step{Resting, false, true},
	)
}

func TestEndThroughHeld(t *testing.T) {
	// The prompt keeps the turn open. A rest after it ends the turn once.
	run(t,
		step{Resting, false, false},
		step{Working, true, false},
		step{Held, false, false},
		step{Resting, false, true},
		step{Resting, false, false},
	)
}

func TestLoneFirstWorkingStartsNothing(t *testing.T) {
	// The start of this turn is not seen.
	run(t, step{Working, false, false})
}

func TestGap(t *testing.T) {
	var tr Tracker
	tr.Observe(Resting)
	tr.Gap()
	if started, _ := tr.Observe(Working); started {
		t.Error("the first state after a gap starts a turn")
	}
}
