package tui

import (
	"slices"
	"testing"

	"github.com/rednafi/rexdiff/internal/testgit"
	"github.com/rednafi/rexdiff/rex"
)

// reopen returns a model of the repository of m, as after a restart, with
// the base flag base.
func reopen(t *testing.T, m model, base string) model {
	t.Helper()
	return newModel(t.Context(), Options{Repo: m.repo, Base: base}, defaultConfig())
}

// pickBase picks the base ref in the base picker.
func pickBase(t *testing.T, m model, ref string) model {
	t.Helper()
	m = press(t, m, "B")
	if m.overlay == nil {
		t.Fatal("B opens no picker")
	}
	m.overlay.input.SetValue(ref)
	m.overlay.filter()
	return press(t, m, "enter")
}

// TestSavedBase checks that a restart keeps the picked base, and that the
// default branch clears it.
func TestSavedBase(t *testing.T) {
	t.Parallel()
	m, _ := newTestModel(t)
	if m = pickBase(t, m, "feat"); m.base != "feat" {
		t.Fatalf("base = %q, want feat", m.base)
	}
	if got := reopen(t, m, "").base; got != "feat" {
		t.Errorf("after a restart, base = %q, want feat", got)
	}
	m = pickBase(t, m, "main")
	if got := m.repo.SavedBase(t.Context()); got != "" {
		t.Errorf("after a pick of the default, SavedBase = %q, want \"\"", got)
	}
}

// TestBaseFlagWins checks that the flag wins over the saved base, and that
// B then opens no picker.
func TestBaseFlagWins(t *testing.T) {
	t.Parallel()
	m, _ := newTestModel(t)
	m = pickBase(t, m, "feat")
	m = reopen(t, m, "main")
	if m.base != "main" {
		t.Errorf("base = %q, want the flag, main", m.base)
	}
	if m = press(t, m, "B"); m.overlay != nil {
		t.Error("with the flag, B opens a picker")
	}
}

// TestSavedTurn checks that a restart keeps the tree at the start of the
// last turn.
func TestSavedTurn(t *testing.T) {
	t.Parallel()
	m, dir := newTestModel(t)
	m = step(t, m, sample("block:agent", "pi", dir))
	m = step(t, m, status("block:agent", "pi", rex.StateWorking))
	if m.repo.SavedTurn(t.Context()) != "" {
		t.Error("a turn with no change saves its tree")
	}
	testgit.WriteFile(t, dir, "c.go", "package a\n")
	m = step(t, m, status("block:agent", "pi", rex.StateIdle))
	if m.turn == nil {
		t.Fatal("no turn after the agent changes a file")
	}
	m2 := reopen(t, m, "")
	if m2.turn == nil || m2.turn.tree != m.turn.tree {
		t.Fatalf("after a restart, turn = %+v, want the tree %s", m2.turn, m.turn.tree)
	}
	m2.setScope(ScopeLastTurn)
	if got := paths(m2); !slices.Equal(got, []string{"c.go"}) {
		t.Errorf("the saved turn shows %v, want [c.go]", got)
	}
}
