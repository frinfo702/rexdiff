// Package turn finds the turns of the agents in a repository. A turn
// starts when the repository goes from rest to work. It ends when the
// repository rests again. The rules come from herdr-reviewr.
package turn

import "github.com/rednafi/rexdiff/rex"

// State is the work state of a repository. Fold makes it from the states
// of all the agents in the repository.
type State int

const (
	// Resting tells that each agent rests, or that there is no agent.
	Resting State = iota
	// Working tells that at least one agent works.
	Working
	// Held tells that no agent works, but one does not rest. For example,
	// it waits at a prompt. A turn that is open stays open.
	Held
)

// resting tells if s is a state between turns. A blocked agent and an
// unknown state are in the middle of a turn.
func resting(s rex.State) bool { return s == rex.StateIdle || s == rex.StateDone }

// Fold returns the state of a repository whose agents are in the states
// states. One working agent makes the repository work.
func Fold(states ...rex.State) State {
	held := false
	for _, s := range states {
		if s == rex.StateWorking {
			return Working
		}
		held = held || !resting(s)
	}
	if held {
		return Held
	}
	return Resting
}

// Tracker finds the start and the end of each turn in a series of states.
// The zero Tracker is ready to use.
type Tracker struct {
	// prevResting tells if the last state is Resting. It is false at
	// first, so the first state starts no turn.
	prevResting bool
	// worked tells if the repository works after its last rest. An end
	// can come through Held, but a start cannot. Thus the two edges keep
	// different memories.
	worked bool
}

// Observe takes the next state. A turn starts when s is Working after
// Resting. A turn ends when s is Resting after work, also through Held.
func (t *Tracker) Observe(s State) (started, ended bool) {
	started = s == Working && t.prevResting
	ended = t.worked && s == Resting
	t.prevResting = s == Resting
	switch s {
	case Working:
		t.worked = true
	case Resting:
		t.worked = false
	}
	return started, ended
}

// Gap tells the Tracker that nobody sees the agents for some time. Then
// the next state cannot start a turn.
func (t *Tracker) Gap() { t.prevResting = false }
