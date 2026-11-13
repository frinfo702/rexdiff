package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/frinfo702/rexdiff/rex"
)

// TestUnflash checks that the end of a flash removes that flash only, and
// that a newer flash stays.
func TestUnflash(t *testing.T) {
	m, _ := newTestModel(t)
	m.flashDone("copied")
	old := m.statusTill
	m.flash("open: no such file") // a newer flash before the first one ends
	m.statusTill = old.Add(time.Minute)
	m = step(t, m, unflashMsg(old))
	if !strings.Contains(m.status, "open:") {
		t.Fatalf("status %q after the end of the older flash, want the newer flash", m.status)
	}
	m.flashDone("copied")
	m = step(t, m, unflashMsg(m.statusTill))
	if m.status != "" || !m.statusTill.IsZero() || m.statusDone {
		t.Errorf("status %q, till %v, done %v after the end of the flash; want all clear",
			m.status, m.statusTill, m.statusDone)
	}
}

// TestLateTick checks that a tick after the end of a flash removes it.
func TestLateTick(t *testing.T) {
	m, _ := newTestModel(t)
	m.flashDone("copied")
	m.statusTill = time.Now().Add(-time.Second)
	m.onTick()
	if m.status != "" || m.statusDone {
		t.Errorf("status %q, done %v after a late tick; want all clear", m.status, m.statusDone)
	}
}

// TestSendingBadge checks that the footer shows the send for as long as it
// runs, on the path of one agent and on the path of the picker.
func TestSendingBadge(t *testing.T) {
	m, _ := newTestModel(t)
	m = press(t, m, "j", "j", "j", "j", "c", "h", "i", "enter")
	m.rex = rex.FromEnv()
	m.flash("line 40 isn't shown") // an older flash must not hide the send
	if m.startSend() == nil {
		t.Fatalf("startSend did not start: status %q", m.status)
	}
	if got := view(m); !strings.Contains(got, "sending…") {
		t.Fatalf("no sending… in the footer after the start of the send:\n%s", got)
	}
	m.onTick()
	if got := view(m); !strings.Contains(got, "sending…") {
		t.Errorf("a tick removed sending… while the send runs")
	}
	m.flash("open: no such file") // a flash during the send shows first
	if got := view(m); strings.Contains(got, "sending…") || !strings.Contains(got, "open:") {
		t.Errorf("want the newer flash in the footer during the send:\n%s", got)
	}
	m = step(t, m, unflashMsg(m.statusTill))
	if got := view(m); !strings.Contains(got, "sending…") {
		t.Errorf("sending… did not come back after the flash ended")
	}

	// Several agents: the picker asks, so no send runs until the user picks.
	two := []rex.Agent{{Block: "a", Name: "pi"}, {Block: "b", Name: "cc"}}
	m.flash("open: no such file")
	m = step(t, m, agentsMsg{send: true, agents: two})
	if m.overlay == nil || m.sends != 0 {
		t.Fatalf("picker open %v, sends %d; want the picker and no send", m.overlay != nil, m.sends)
	}
	if !strings.Contains(m.status, "open:") {
		t.Errorf("status %q; the picker must not remove a flash of another action", m.status)
	}
	m.clearFlash()
	if got := view(m); strings.Contains(got, "sending…") {
		t.Errorf("sending… shows while the picker asks")
	}
	m = press(t, m, "enter") // pick the first agent
	if got := view(m); m.sends != 1 || !strings.Contains(got, "sending…") {
		t.Errorf("sends %d; want sending… in the footer after the user picked an agent:\n%s", m.sends, got)
	}
	m = step(t, m, sentMsg{agent: two[0]})
	if got := view(m); strings.Contains(got, "sending…") || !strings.Contains(got, "✓ sent") {
		t.Errorf("want the confirmation in the footer after the send:\n%s", got)
	}
}

// TestUnflashOnce checks that the ticks start one unflash command for each
// flash, also when the end of the flash is more than one tick away.
func TestUnflashOnce(t *testing.T) {
	m, _ := newTestModel(t)
	m.flash("open: no such file")
	m.onTick()
	if !m.unflashAt.Equal(m.statusTill) {
		t.Fatalf("the first tick of a flash started no unflash command")
	}
	first := m.unflashAt
	m.onTick()
	if !m.unflashAt.Equal(first) {
		t.Errorf("the second tick started another unflash command")
	}
	m.flashDone("copied")
	m.onTick()
	if !m.unflashAt.Equal(m.statusTill) || m.unflashAt.Equal(first) {
		t.Errorf("a newer flash got no unflash command of its own")
	}
}
