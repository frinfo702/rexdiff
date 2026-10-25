package tui

import (
	"testing"
	"time"

	"github.com/rednafi/rexdiff/internal/testgit"
	"github.com/rednafi/rexdiff/vcs"
)

// TestPRStaleResult checks that the result of an earlier read does not
// show.
func TestPRStaleResult(t *testing.T) {
	m, _ := newTestModel(t)
	m = press(t, m, "3")
	m.loadPR()
	m.loadPR()
	m = step(t, m, prMsg{gen: 1, pr: &vcs.PullRequest{Title: "old"}})
	if m.pr.pr != nil || !m.prf.inFlight {
		t.Fatalf("a stale result shows: %+v", m.pr.pr)
	}
	m = step(t, m, prMsg{gen: 2, pr: &vcs.PullRequest{Title: "new"}})
	if m.pr.pr == nil || m.pr.pr.Title != "new" || m.prf.inFlight {
		t.Errorf("pr %+v, in flight %v, want the new one", m.pr.pr, m.prf.inFlight)
	}
}

// TestPRPoll checks that a tick reads the pull request again a minute
// after the last request, but only on the PR tab.
func TestPRPoll(t *testing.T) {
	m, _ := newTestModel(t)
	m.prf.polled = time.Now().Add(-61 * time.Second)
	if m = step(t, m, tickMsg{}); m.prf.inFlight {
		t.Error("a tick on the Changes tab reads the pull request")
	}
	m = press(t, m, "3")
	m.prf.polled = time.Now().Add(-59 * time.Second)
	if m = step(t, m, tickMsg{}); m.prf.inFlight {
		t.Error("a tick before a minute reads the pull request")
	}
	m.prf.polled = time.Now().Add(-61 * time.Second)
	if m = step(t, m, tickMsg{}); !m.prf.inFlight || m.prf.gen != 1 {
		t.Errorf("after a minute: in flight %v, gen %d, want a read", m.prf.inFlight, m.prf.gen)
	}
}

// TestPRTrailing checks that an ambient request during a read starts one
// more read after it, and that r and a hung read start one at once.
func TestPRTrailing(t *testing.T) {
	m, _ := newTestModel(t)
	m = press(t, m, "3")
	m.requestPR(true)
	if cmd := m.requestPR(false); cmd != nil || !m.prf.trailing || m.prf.gen != 1 {
		t.Fatalf("an ambient request during a read: trailing %v, gen %d, want 1 trailing read",
			m.prf.trailing, m.prf.gen)
	}
	next, cmd := m.Update(prMsg{gen: 1})
	m = *next.(*model)
	if cmd == nil || m.prf.gen != 2 || !m.prf.inFlight || m.prf.trailing {
		t.Fatalf("after the read: gen %d, in flight %v, trailing %v, want the trailing read",
			m.prf.gen, m.prf.inFlight, m.prf.trailing)
	}
	if m = step(t, m, prMsg{gen: 2}); m.prf.inFlight {
		t.Error("the trailing read starts another read")
	}

	m.requestPR(false)
	if m = press(t, m, "r"); m.prf.gen != 4 {
		t.Errorf("r during a read: gen %d, want a new read", m.prf.gen)
	}
	m.prf.started = time.Now().Add(-61 * time.Second)
	if m.requestPR(false); m.prf.gen != 5 || m.prf.trailing {
		t.Errorf("a request after a hung read: gen %d, want a new read", m.prf.gen)
	}
}

// TestPRBranchSwitch checks that a switch to another branch blanks the PR
// tab, and that a commit on the branch keeps the pull request.
func TestPRBranchSwitch(t *testing.T) {
	m, dir, pause := watched(t)
	m = press(t, m, "3")
	m = step(t, m, prMsg{gen: m.prf.gen, pr: &vcs.PullRequest{Title: "T"}})

	testgit.Git(t, dir, "commit", "-qam", "more")
	time.Sleep(pause)
	m = step(t, m, tickMsg{})
	if m.pr.pr == nil || !m.prf.inFlight {
		t.Fatalf("after a commit: pr %+v, in flight %v, want the PR kept and a read",
			m.pr.pr, m.prf.inFlight)
	}
	m = step(t, m, prMsg{gen: m.prf.gen, pr: &vcs.PullRequest{Title: "T"}})

	testgit.Git(t, dir, "checkout", "-q", "main")
	gen := m.prf.gen
	time.Sleep(pause)
	m = step(t, m, tickMsg{})
	if m.pr.pr != nil || m.prf.gen == gen || !shows(m, "loading the pull request") {
		t.Errorf("after a switch: pr %+v, gen %d, want a blank tab and a read", m.pr.pr, m.prf.gen)
	}
}
