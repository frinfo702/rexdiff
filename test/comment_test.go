package test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rednafi/rexdiff/internal/testgit"
)

// comment adds the comment text on the first changed line of a.go.
func comment(tm *term, text string) {
	tm.t.Helper()
	tm.keys("]", "c")
	tm.keys(strings.Split(text, "")...)
	tm.keys("enter")
	tm.waitFor("💬 " + text)
}

// TestCommentList checks that a comment shows on its line and in the
// list, and that the list edits and deletes it.
func TestCommentList(t *testing.T) {
	t.Parallel()
	tm := start(t, newRepo(t), opts{})
	comment(tm, "hi")
	tm.waitFor("comment added", "●1")
	tm.keys("l")
	tm.waitFor("Comments (1)", "a.go:4 (removed)  hi")
	tm.keys("e", " ", "m", "o", "r", "e", "enter")
	tm.waitFor("a.go:4 (removed)  hi more")
	tm.keys("d") // the list closes with its last comment
	tm.waitGone("Comments (", "💬")
}

// TestRangeComment checks that v selects lines and c comments on all of
// them.
func TestRangeComment(t *testing.T) {
	t.Parallel()
	tm := start(t, newRepo(t), opts{})
	tm.keys("]", "v", "j", "j", "c", "r", "enter")
	tm.keys("l")
	tm.waitFor("a.go:4-5  r")
}

// TestSend checks that s pastes the review into the agent and gives it
// the focus.
func TestSend(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	rex := newRex(t, repo)
	tm := start(t, repo, opts{rex: rex})
	tm.waitFor("send → 1 agent")
	comment(tm, "hi")
	tm.keys("s")
	tm.waitFor("sent 1 comment to agent")
	tm.waitGone("💬")
	calls := rex.calls(t)
	for _, want := range []string{
		`block.event {"block_id":"block:pi","event":{"type":"paste",` +
			`"payload":{"text":"a.go:4 (removed)\n-\treturn 1\nhi"}}}`,
		`session.focus_block {"block_id":"block:pi"}`,
	} {
		if !strings.Contains(calls, want) {
			t.Errorf("rex calls:\n%s\nwant a call with %s", calls, want)
		}
	}
}

// TestCopy checks that y copies the review and clears the comments.
func TestCopy(t *testing.T) {
	t.Parallel()
	bin := tool(t, t.TempDir(), "pbcopy", `cat > "${0%/*}/clip"`+"\n")
	tool(t, bin, "wl-copy", `cat > "${0%/*}/clip"`+"\n")
	tm := start(t, newRepo(t), opts{path: bin, env: []string{"WAYLAND_DISPLAY=w"}})
	comment(tm, "hi")
	tm.keys("y")
	tm.waitFor("copied 1 comment")
	tm.waitGone("💬")
	b, err := os.ReadFile(filepath.Join(bin, "clip"))
	if want := "a.go:4 (removed)\n-\treturn 1\nhi"; err != nil || string(b) != want {
		t.Errorf("the clipboard has %q (%v), want %q", b, err, want)
	}
}

// TestQuit checks that q quits at once with no comment, and that with a
// comment it asks first.
func TestQuit(t *testing.T) {
	t.Parallel()
	tm := start(t, newRepo(t), opts{})
	tm.keys("q")
	if err := tm.exited(); err != nil {
		t.Errorf("q exits with %v, want no error", err)
	}

	tm = start(t, newRepo(t), opts{})
	comment(tm, "hi")
	tm.keys("Q")
	time.Sleep(200 * time.Millisecond)
	tm.waitFor("💬 hi") // Q does nothing outside the prompt
	tm.keys("q")
	tm.waitFor("1 unsent comment will be lost",
		"Q quit (1 pending) · esc cancel · s send 1 · y copy")
	tm.keys("esc")
	tm.waitGone("unsent comment")
	tm.keys("q", "Q")
	if err := tm.exited(); err != nil {
		t.Errorf("q, Q exits with %v, want no error", err)
	}
}

// TestEditorInPane checks that e opens a terminal editor at the line, in
// a new Rex pane.
func TestEditorInPane(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	rex := newRex(t, repo)
	bin := tool(t, t.TempDir(), "nvim", "")
	tm := start(t, repo, opts{rex: rex, path: bin, env: []string{"EDITOR=nvim"}})
	tm.keys("]", "j", "e")
	want := "split -s S -b block:self --cwd " + repo + " -- "
	deadline := time.Now().Add(10 * time.Second)
	for !strings.Contains(rex.calls(t), want) {
		if time.Now().After(deadline) {
			t.Fatalf("rex calls:\n%s\nwant a call with %s", rex.calls(t), want)
		}
		time.Sleep(20 * time.Millisecond)
	}
	var split string
	for _, l := range lines(rex.calls(t), want) {
		split = l
	}
	if !strings.Contains(split, "nvim +4 ") || !strings.Contains(split, "a.go") {
		t.Errorf("the split runs %q, want nvim +4 on a.go", split)
	}
}

// TestWindowEditor checks that e starts a window editor at the line.
func TestWindowEditor(t *testing.T) {
	t.Parallel()
	bin := t.TempDir()
	tool(t, bin, "code", `printf '%s\n' "$@" > "${0%/*}/args"`+"\n")
	tm := start(t, newRepo(t), opts{path: bin, env: []string{"VISUAL=code --wait"}})
	tm.keys("]", "j", "e")
	path := filepath.Join(bin, "args")
	deadline := time.Now().Add(10 * time.Second)
	for {
		b, _ := os.ReadFile(path)
		if strings.Contains(string(b), "a.go:4") {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("code got %q, want a.go:4. The screen shows:\n%s", b, tm.screen())
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestNoEditor checks that e tells how to set an editor when none is set.
func TestNoEditor(t *testing.T) {
	t.Parallel()
	tm := start(t, newRepo(t), opts{})
	tm.keys("]", "j", "e")
	tm.waitFor("no editor: set `editor` in rexdiff.lua, $EDITOR, or git's core.editor")
}

// TestMouse checks that a click picks a file, a click on a line number
// opens the comment box, and a triple click copies a line.
func TestMouse(t *testing.T) {
	t.Parallel()
	bin := tool(t, t.TempDir(), "pbcopy", `cat > "${0%/*}/clip"`+"\n")
	tool(t, bin, "wl-copy", `cat > "${0%/*}/clip"`+"\n")
	tm := start(t, newRepo(t), opts{path: bin, env: []string{"WAYLAND_DISPLAY=w"}})
	tm.click("M long.txt", 1)
	tm.waitFor("⋯ 32 unmodified lines")
	tm.click("M a.go", 1)
	tm.click("func A() int {", 3)
	tm.waitFor("copied 14 chars")
	b, err := os.ReadFile(filepath.Join(bin, "clip"))
	if err != nil || string(b) != "func A() int {" {
		t.Errorf("the clipboard has %q (%v), want the line", b, err)
	}
	tm.click(" 5 ▌", 1) // the number of the new line 5
	tm.waitFor("✎ a.go:5")
}

// TestAgentPicker checks that a send with two agents asks which one, and
// that a digit picks it.
func TestAgentPicker(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	rex := newRex(t, repo)
	rex.addAgent(t, "block:cc", "cc", "claude-code")
	tm := start(t, repo, opts{rex: rex})
	tm.waitFor("send → 2 agents")
	comment(tm, "hi")
	tm.keys("s")
	tm.waitFor("send to which agent?", "1  agent", "2  cc")
	tm.keys("2")
	tm.waitFor("sent 1 comment to cc")
	if calls := rex.calls(t); !strings.Contains(calls, `block.event {"block_id":"block:cc"`) {
		t.Errorf("rex calls:\n%s\nwant a paste to block:cc", calls)
	}
}

// TestCommentStays checks that a comment stays when the agent edits the
// file. As in herdr-reviewr, it keeps its line number.
func TestCommentStays(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	tm := start(t, repo, opts{})
	tm.keys("]", "j", "c", "h", "i", "enter") // on x := 2
	tm.waitFor("💬 hi")
	testgit.WriteFile(t, repo, "a.go",
		"package a\n\n// A adds.\n// It is new.\nfunc A() int {\n\tx := 2\n\treturn x\n}\n")
	tm.waitFor("It is new.")
	tm.keys("l")
	tm.waitFor("a.go:4  hi")
}
