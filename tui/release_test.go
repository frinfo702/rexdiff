package tui

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/rednafi/rexdiff/diff"
	"github.com/rednafi/rexdiff/rex"
	"github.com/rednafi/rexdiff/update"
	"github.com/rednafi/rexdiff/vcs"
)

// fakeUpdater is an Updater with a fixed latest release.
type fakeUpdater struct {
	latest     update.Release
	newer      bool
	err        error    // the error of Latest
	installErr error    // the error of Install
	installed  []string // the tags that Install got
}

func (f *fakeUpdater) Latest(context.Context) (update.Release, bool, error) {
	return f.latest, f.newer, f.err
}

func (f *fakeUpdater) Install(_ context.Context, r update.Release) error {
	f.installed = append(f.installed, r.Tag)
	return f.installErr
}

// updaterModel returns a model with u, after the release check.
func updaterModel(t *testing.T, u *fakeUpdater) model {
	t.Helper()
	m, _ := newTestModel(t)
	m = newModel(t.Context(), Options{Repo: m.repo, Updater: u}, defaultConfig())
	m.reload()
	m = step(t, m, tea.WindowSizeMsg{Width: 120, Height: 30})
	return step(t, m, m.checkRelease()())
}

// lastLine returns the last line of the view, without colours.
func lastLine(m model) string {
	lines := strings.Split(view(m), "\n")
	return lines[len(lines)-1]
}

// run runs cmd and gives its message to m.
func run(t *testing.T, m model, cmd tea.Cmd) (model, tea.Cmd) {
	t.Helper()
	if cmd == nil {
		t.Fatal("no command")
	}
	next, cmd := m.Update(cmd())
	return *next.(*model), cmd
}

// TestUpdate checks the prompt, the install, and the restart of an update
// from the footer.
func TestUpdate(t *testing.T) {
	u := &fakeUpdater{latest: update.Release{Tag: "v0.2.0"}, newer: true}
	m := updaterModel(t, u)
	const prompt = "rexdiff v0.2.0 is out: press U or click here to update"
	if got := lastLine(m); !strings.Contains(got, prompt) {
		t.Fatalf("footer = %q, want %q", got, prompt)
	}

	// The key U starts the install, and the footer tells about it.
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'U', Text: "U"})
	m = *next.(*model)
	if got := lastLine(m); !strings.Contains(got, "updating rexdiff to v0.2.0…") {
		t.Errorf("footer during the install = %q, want updating rexdiff to v0.2.0…", got)
	}
	// The user starts a comment while the release installs.
	m = press(t, m, "j", "j", "j", "j", "c")
	m, restart := run(t, m, cmd)
	if len(u.installed) != 1 || u.installed[0] != "v0.2.0" {
		t.Fatalf("Install got %v, want v0.2.0", u.installed)
	}
	if restart != nil || m.restartState != "" {
		t.Fatal("rexdiff restarts while the user writes a comment")
	}
	// The restart waits until the user saves the comment.
	m = press(t, m, "f", "i", "x", "enter")
	if m.restartState == "" {
		t.Fatal("no restart after the comment")
	}
	t.Cleanup(func() { os.Remove(m.restartState) })

	// The new rexdiff reads back the comment and the place.
	saved, err := loadState(m.restartState)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(m.restartState); !errors.Is(err, fs.ErrNotExist) {
		t.Error("loadState did not remove the file")
	}
	m2 := newModel(t.Context(), Options{Repo: m.repo}, defaultConfig())
	m2.restoreSettings(saved)
	m2.reload()
	m2.restorePlaces(saved)
	if got, want := comments(m2), comments(m); got != want {
		t.Errorf("comments after the restart = %q, want %q", got, want)
	}
	if m2.changes.cur != m.changes.cur || m2.scope != m.scope || m2.tab != m.tab {
		t.Errorf("after the restart: cursor %d, scope %q, tab %d; want %d, %q, %d",
			m2.changes.cur, m2.scope, m2.tab, m.changes.cur, m.scope, m.tab)
	}
	if got, want := m2.comments.Review(""), m.comments.Review(""); got != want {
		t.Errorf("the review after the restart:\n%s\nwant:\n%s", got, want)
	}
}

// comments returns the locations and the texts of the comments of m.
func comments(m model) string {
	var b strings.Builder
	for _, c := range m.comments.All() {
		b.WriteString(c.Location() + " " + c.Text + "\n")
	}
	return b.String()
}

// TestUpdateClick checks that a click on the footer starts the install.
func TestUpdateClick(t *testing.T) {
	u := &fakeUpdater{latest: update.Release{Tag: "v0.2.0"}, newer: true}
	m := updaterModel(t, u)
	next, cmd := m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: 3, Y: m.h - 1})
	m = *next.(*model)
	if cmd == nil || m.phase != phaseInstalling {
		t.Fatal("a click on the footer did not start the install")
	}
}

// TestUpdateFails checks that a failed install shows the reason, does not
// restart, and lets the user try again.
func TestUpdateFails(t *testing.T) {
	u := &fakeUpdater{
		latest:     update.Release{Tag: "v0.2.0"},
		newer:      true,
		installErr: errors.New("brew: exit status 1: Error: another brew is running"),
	}
	m := updaterModel(t, u)
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'U', Text: "U"})
	m, restart := run(t, *next.(*model), cmd)
	if restart != nil || m.restartState != "" {
		t.Fatal("rexdiff restarts after a failed install")
	}
	if !strings.Contains(lastLine(m), "update failed: brew: exit status 1: Error: another brew is running") {
		t.Errorf("footer = %q, want the reason", lastLine(m))
	}
	if _, cmd := m.Update(tea.KeyPressMsg{Code: 'U', Text: "U"}); cmd == nil {
		t.Error("U does not try again")
	}
}

// TestNoUpdate checks that the footer shows no release and U does nothing
// without a newer release, after a failed check, or without an updater.
func TestNoUpdate(t *testing.T) {
	for _, u := range []*fakeUpdater{
		{latest: update.Release{Tag: "v0.1.0"}},
		{err: errors.New("no network")},
	} {
		m := updaterModel(t, u)
		if m.release != nil || strings.Contains(lastLine(m), "is out") {
			t.Errorf("updater %+v: footer %q, want no release", u, lastLine(m))
		}
		if _, cmd := m.Update(tea.KeyPressMsg{Code: 'U', Text: "U"}); cmd != nil {
			t.Errorf("updater %+v: U started a command", u)
		}
	}
	m, _ := newTestModel(t)
	if cmd := m.Init(); m.updater != nil || cmd == nil {
		t.Error("the test model has an updater")
	}
}

// TestCommentsJSON checks that the comments give the same review after
// JSON encoding and decoding.
func TestCommentsJSON(t *testing.T) {
	m, _ := newTestModel(t)
	m = press(t, m, "j", "j", "j", "j", "c", "x", "enter")
	b, err := m.comments.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	var back diff.Comments
	if err := back.UnmarshalJSON(b); err != nil {
		t.Fatal(err)
	}
	if back.Review("p") != m.comments.Review("p") || back.Len() != 1 {
		t.Errorf("review after JSON = %q, want %q", back.Review("p"), m.comments.Review("p"))
	}
	// After the restart, a new comment also gets a new ID.
	if c := back.Add(diff.Comment{Path: "a.go", Start: 1, End: 1}); c.ID != 2 {
		t.Errorf("ID of a new comment = %d, want 2", c.ID)
	}
}

// TestRestartWaits checks that the restart waits for a running send and for
// a message whose command is not done.
func TestRestartWaits(t *testing.T) {
	u := &fakeUpdater{latest: update.Release{Tag: "v0.2.0"}, newer: true}
	m := updaterModel(t, u)
	m.sends = 1 // a send runs while the install ends
	m = step(t, m, installedMsg{})
	if m.restartState != "" {
		t.Fatal("restart while a send runs")
	}
	m = step(t, m, sentMsg{agent: rex.Agent{Name: "pi"}})
	if m.restartState == "" {
		t.Fatal("no restart after the send")
	}
	os.Remove(m.restartState)

	// In the PR tab, the key r returns the command that loads the PR. Thus
	// r does not restart, but a message without a command does.
	m = updaterModel(t, u)
	m.tab = tabPR
	m.phase = phaseInstalled
	if next, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"}); cmd == nil || next.(*model).restartState != "" {
		t.Fatal("restart on a message with a command")
	}
	m = step(t, m, tea.FocusMsg{})
	if m.restartState == "" {
		t.Fatal("no restart on a message without a command")
	}
	os.Remove(m.restartState)
}

// TestRestorePRPlace checks that the place in the PR tab comes back after
// a restart, although the PR loads after the restore.
func TestRestorePRPlace(t *testing.T) {
	m, _ := newTestModel(t)
	saved := &savedState{PR: savedPane{Cur: 7, Top: 3}, Comments: new(diff.Comments)}
	m.restoreSettings(saved)
	m.restorePlaces(saved)
	// A failed first load keeps the place for the next load.
	m = step(t, m, prMsg{err: errors.New("no network")})
	body := strings.Repeat("line\n\n", 40)
	m = step(t, m, prMsg{pr: &vcs.PullRequest{Title: "t", Body: body}})
	if m.prPane.cur != 7 || m.prPane.top != 3 {
		t.Errorf("PR place = %d, %d; want 7, 3", m.prPane.cur, m.prPane.top)
	}
}

// TestUpdateClickRule checks that a click on the footer starts the install,
// and a click on the rule above it does nothing.
func TestUpdateClickRule(t *testing.T) {
	u := &fakeUpdater{latest: update.Release{Tag: "v0.2.0"}, newer: true}
	m := updaterModel(t, u)
	rule := bodyY + m.bodyHeight()
	if got := step(t, m, tea.MouseClickMsg{Button: tea.MouseLeft, X: 3, Y: rule}); got.phase == phaseInstalling {
		t.Error("a click on the rule started the install")
	}
	if got := step(t, m, tea.MouseClickMsg{Button: tea.MouseLeft, X: 3, Y: rule + 1}); got.phase != phaseInstalling {
		t.Error("a click on the footer did not start the install")
	}
}

// TestRestartSaveFails checks that a failed save of the state tells the
// user and stops the restart, so that rexdiff continues with the old version.
func TestRestartSaveFails(t *testing.T) {
	u := &fakeUpdater{latest: update.Release{Tag: "v0.2.0"}, newer: true}
	m := updaterModel(t, u)
	m.mode = modeComment // the install ends while the user writes
	m = step(t, m, installedMsg{})
	if got := lastLine(m); !strings.Contains(got, "rexdiff v0.2.0 is installed; it starts when you are done") {
		t.Errorf("footer while the restart waits = %q, want that v0.2.0 is installed", got)
	}
	m.mode = modeNormal
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "missing"))
	m = step(t, m, tea.FocusMsg{})
	if m.restartState != "" || m.phase != phaseIdle || m.release != nil {
		t.Errorf("after a failed save: state %q, phase %d, release %v; want none, idle, nil",
			m.restartState, m.phase, m.release)
	}
	if !strings.Contains(m.status, "cannot save your comments for the restart") {
		t.Errorf("status = %q, want the reason", m.status)
	}
}

// TestRestartOnTick checks that a tick can start the restart, though its
// command schedules the next tick.
func TestRestartOnTick(t *testing.T) {
	u := &fakeUpdater{latest: update.Release{Tag: "v0.2.0"}, newer: true}
	m := updaterModel(t, u)
	m.phase = phaseInstalled
	next, cmd := m.Update(tickMsg{})
	m = *next.(*model)
	if m.restartState == "" || cmd == nil {
		t.Fatal("no restart on a tick")
	}
	os.Remove(m.restartState)
}

// TestSavePendingPRPlace checks that a restart before the PR loads saves
// the PR place of the previous restart.
func TestSavePendingPRPlace(t *testing.T) {
	m, _ := newTestModel(t)
	m.prPlace = &savedPane{Cur: 7, Top: 3}
	path, err := m.saveState()
	if err != nil {
		t.Fatal(err)
	}
	s, err := loadState(path)
	if err != nil || s.PR != (savedPane{Cur: 7, Top: 3}) {
		t.Errorf("saved PR place = %+v, %v; want 7, 3", s.PR, err)
	}
}

// TestWrapWordsNarrow checks that a width of 0, or a character wider than
// the width, does not loop forever.
func TestWrapWordsNarrow(t *testing.T) {
	tests := []struct {
		s    string
		w    int
		want string
	}{
		{"ab c", 0, "a\nb\nc"},
		{"中 a", 1, "中\na"},
		{"a中b", 2, "a\n中\nb"},
	}
	for _, tc := range tests {
		if got := wrapWords(tc.s, tc.w); got != tc.want {
			t.Errorf("wrapWords(%q, %d) = %q, want %q", tc.s, tc.w, got, tc.want)
		}
	}
}

// TestRunChecksOptions checks that Run refuses options that it cannot use,
// before it starts the program.
func TestRunChecksOptions(t *testing.T) {
	m, _ := newTestModel(t)
	tests := []struct {
		o    Options
		want string
	}{
		{Options{}, "no repository"},
		{Options{Repo: m.repo, Layout: "sideways"}, `invalid value for layout: expected one of unified, split, not "sideways"`},
		{Options{Repo: m.repo, Scope: "commits"}, `invalid value for scope: expected one of uncommitted, branch, last-turn, not "commits"`},
	}
	for _, tc := range tests {
		if _, err := Run(t.Context(), tc.o); err == nil || err.Error() != tc.want {
			t.Errorf("Run(%+v) error = %v, want %s", tc.o, err, tc.want)
		}
	}
}

// TestSendKeepsEdits checks that a comment edited while its send runs
// stays after the send, whether the user saved the edit before the send
// ended or after it.
func TestSendKeepsEdits(t *testing.T) {
	for _, saveFirst := range []bool{true, false} {
		m, _ := newTestModel(t)
		m = press(t, m, "j", "j", "j", "j", "c", "o", "l", "d", "enter")
		sent := sentMsg{agent: rex.Agent{Name: "pi"}, ids: []int{m.comments.All()[0].ID}}
		m.sends = 1               // the send runs
		m = press(t, m, "e", "!") // edit the comment
		if saveFirst {
			m = press(t, m, "enter")
			m = step(t, m, sent)
		} else {
			m = step(t, m, sent)
			m = press(t, m, "enter")
		}
		if got := comments(m); !strings.HasSuffix(strings.TrimSpace(got), "old!") {
			t.Errorf("save first %v: comments after the send = %q, want the edited comment", saveFirst, got)
		}
	}
	// The send removes a comment that the user did not change.
	m, _ := newTestModel(t)
	m = press(t, m, "j", "j", "j", "j", "c", "x", "enter")
	m.sends = 1
	m = step(t, m, sentMsg{agent: rex.Agent{Name: "pi"}, ids: []int{m.comments.All()[0].ID}})
	if m.comments.Len() != 0 {
		t.Errorf("comments after the send = %q, want none", comments(m))
	}
}

// TestPickerWaitsForComment checks that the agent picker does not open over
// a comment that the user starts while the agents load.
func TestPickerWaitsForComment(t *testing.T) {
	m, _ := newTestModel(t)
	m = press(t, m, "j", "j", "j", "j", "c", "h", "i")
	m.sends = 1
	agents := agentsMsg{send: true, agents: []rex.Agent{{Block: "a", Name: "pi"}, {Block: "b", Name: "cc"}}}
	m = step(t, m, agents)
	if m.mode != modeComment || m.overlay != nil {
		t.Fatalf("mode %d, picker open %v; want the comment box, no picker", m.mode, m.overlay != nil)
	}
	if !strings.Contains(m.status, "then press s to pick one") || m.sends != 0 {
		t.Errorf("status %q, sends %d; want the hint and no send", m.status, m.sends)
	}
	m = press(t, m, "enter")
	if got := comments(m); !strings.Contains(got, "hi") {
		t.Errorf("comments = %q, want the comment that the user wrote", got)
	}
}
