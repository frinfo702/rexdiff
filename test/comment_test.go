package test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/rednafi/rexdiff/internal/testgit"
)

// review is the text of the comment "hi" on the first change of a.go.
const review = "a.go:4 (removed)\n-\treturn 1\nhi"

// comment adds the comment text on the first changed line of a.go.
func comment(tm *term, text string) {
	tm.t.Helper()
	tm.keys("]", "c")
	tm.typed(text)
	tm.keys("enter")
	tm.waitFor("💬 " + text)
}

// TestCommentList checks that a comment shows on its line and in the
// list, that n moves to it, and that the list edits and deletes it.
func TestCommentList(t *testing.T) {
	tm := start(t, newRepo(t), opts{})
	comment(tm, "hi")
	tm.waitFor("comment added", "●1")
	tm.keys("k", "k", "n") // n stays in the file, as in herdr-reviewr
	tm.waitCursor("return 1")
	tm.keys("l")
	tm.waitFor("Comments (1)", "a.go:4 (removed)  hi")
	tm.keys("e", " ")
	tm.typed("more")
	tm.keys("enter")
	tm.waitFor("a.go:4 (removed)  hi more")
	tm.keys("d") // the list closes with its last comment
	tm.waitGone("Comments (", "💬")
}

// TestCommentBox checks that the box takes more than one line, that esc
// drops a new comment, and that v and a drag on the line numbers comment
// on a range of lines.
func TestCommentBox(t *testing.T) {
	tm := start(t, newRepo(t), opts{})
	tm.keys("]", "c", "a", "ctrl+j", "b", "enter")
	tm.waitFor("💬 a", "b")
	tm.keys("j", "c", "x", "esc")
	tm.waitGone("💬 x")
	tm.keys("v", "j", "c", "r", "enter", "l")
	tm.waitFor("a.go:4-5  r")
	tm.keys("esc")
	tm.drag(" 1  1 ", " 3  3 ")
	tm.waitFor("✎ a.go:1-3")
}

// TestSend checks that s pastes the review into the agent and gives it
// the focus, and that S does the same.
func TestSend(t *testing.T) {
	repo := newRepo(t)
	rex := newRex(t, repo)
	tm := start(t, repo, opts{rex: rex})
	tm.waitFor("send → 1 agent")
	comment(tm, "hi")
	tm.keys("s")
	tm.waitFor("sent 1 comment to agent")
	tm.waitGone("💬")
	comment(tm, "again")
	tm.keys("S")
	tm.waitFor("sent 1 comment to agent")
	calls := rex.calls(t)
	for _, want := range []string{
		`block.event {"block_id":"block:pi","event":{"type":"paste",` +
			`"payload":{"text":"a.go:4 (removed)\n-\treturn 1\nhi"}}}`,
		`session.focus_block {"block_id":"block:pi"}`,
		`"payload":{"text":"a.go:4 (removed)\n-\treturn 1\nagain"}`,
	} {
		if !strings.Contains(calls, want) {
			t.Errorf("rex calls:\n%s\nwant a call with %s", calls, want)
		}
	}
}

// TestSendWithoutAgent checks that s outside Rex tells the user to copy.
func TestSendWithoutAgent(t *testing.T) {
	tm := start(t, newRepo(t), opts{})
	comment(tm, "hi")
	tm.keys("s")
	tm.waitFor("not in Rex")
	tm.waitFor("💬 hi")
}

// TestAgentPicker checks that a send with two agents asks which one, and
// that a digit picks it.
func TestAgentPicker(t *testing.T) {
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

// TestCopy checks that y and Y copy the review and clear the comments.
func TestCopy(t *testing.T) {
	for _, key := range []string{"y", "Y"} {
		t.Run(key, func(t *testing.T) {
			bin := clipboard(t)
			tm := start(t, newRepo(t), opts{path: bin, env: clipEnv})
			comment(tm, "hi")
			tm.keys(key)
			tm.waitFor("copied 1 comment")
			tm.waitGone("💬")
			waitFile(t, filepath.Join(bin, "clip"), func(s string) bool { return s == review })
		})
	}
}

// TestMouse checks that a click picks a file, a triple click copies a
// line, a drag on the code copies the lines, and the wheel scrolls.
func TestMouse(t *testing.T) {
	bin := clipboard(t)
	clip := filepath.Join(bin, "clip")
	tm := start(t, newRepo(t), opts{path: bin, env: clipEnv, height: 12})
	tm.click("M long.txt", 1)
	tm.waitFor("⋯ 32 unmodified lines")
	tm.click("M a.go", 1)
	tm.click("func A() int {", 3)
	tm.waitFor("copied 14 chars")
	waitFile(t, clip, func(s string) bool { return s == "func A() int {" })
	tm.drag("package a", "func A()")
	waitFile(t, clip, func(s string) bool { return s == "package a\n\nfunc A() int {" })

	tm.click("M long.txt", 1)
	tm.keys(":", "2", "0", "enter") // opens the fold
	x, y := tm.at("line 20")
	for range 10 {
		tm.mouse(65, x, y, "M") // the wheel down
	}
	tm.waitGone(" first")
}

// TestQuit checks that q quits at once with no comment, and that with a
// comment it asks first.
func TestQuit(t *testing.T) {
	tm := start(t, newRepo(t), opts{})
	tm.keys("q")
	if err := tm.exited(); err != nil {
		t.Errorf("q exits with %v, want no error", err)
	}

	tm = start(t, newRepo(t), opts{})
	comment(tm, "hi")
	tm.keys("Q", "q") // Q does nothing outside the prompt
	tm.waitFor("1 unsent comment will be lost",
		"Q quit (1 pending) · esc cancel · s send 1 · y copy")
	tm.keys("esc")
	tm.waitGone("unsent comment")
	tm.waitFor("💬 hi")
	tm.keys("q", "Q")
	if err := tm.exited(); err != nil {
		t.Errorf("q, Q exits with %v, want no error", err)
	}
}

// TestCommentStays checks that a comment stays when the agent edits the
// file. As in herdr-reviewr, it keeps its line number.
func TestCommentStays(t *testing.T) {
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

// TestEditorInPane checks that e opens a terminal editor at the line, in
// a new Rex pane.
func TestEditorInPane(t *testing.T) {
	repo := newRepo(t)
	rex := newRex(t, repo)
	bin := tools(t, map[string]string{"nvim": ""})
	tm := start(t, repo, opts{rex: rex, path: bin, env: []string{"EDITOR=nvim"}})
	tm.keys("]", "j", "e")
	want := "split -s S -b block:self --cwd " + repo + " -- sh -c nvim +4 '" +
		filepath.Join(repo, "a.go") + "'"
	eventually(t, func() bool { return strings.Contains(rex.calls(t), want) }, func() string {
		return "rex calls:\n" + rex.calls(t) + "\nwant " + want
	})
}

// TestWindowEditor checks that e starts a window editor at the line, and
// that the editor setting picks the editor.
func TestWindowEditor(t *testing.T) {
	bin := tools(t, map[string]string{
		"code": `printf '%s\n' "$@" > "${0%/*}/code"` + "\n",
		"mine": `printf '%s\n' "$@" > "${0%/*}/mine"` + "\n",
	})
	tm := start(t, newRepo(t), opts{path: bin, env: []string{"VISUAL=code --wait"}})
	tm.keys("]", "j", "e")
	waitFile(t, filepath.Join(bin, "code"), func(s string) bool {
		return strings.HasPrefix(s, "--wait\n-g\n") && strings.HasSuffix(s, "a.go:4\n")
	})

	tm = start(t, newRepo(t), opts{path: bin, args: []string{
		"-config",
		`{"editor": "mine --line {line} {file}"}`,
	}})
	tm.keys("]", "j", "e")
	waitFile(t, filepath.Join(bin, "mine"), func(s string) bool {
		return strings.HasPrefix(s, "--line\n4\n") && strings.HasSuffix(s, "a.go\n")
	})
}

// TestNoEditor checks that e tells how to set an editor when none is set.
func TestNoEditor(t *testing.T) {
	tm := start(t, newRepo(t), opts{})
	tm.keys("]", "j", "e")
	tm.waitFor("no editor: set `editor` in rexdiff.lua, $EDITOR, or git's core.editor")
}

// TestCopyHangs checks that a clipboard tool that does not answer fails
// the copy, and rexdiff still takes keys.
func TestCopyHangs(t *testing.T) {
	hang := "exec sleep 60\n"
	bin := tools(t, map[string]string{"pbcopy": hang, "wl-copy": hang})
	tm := start(t, newRepo(t), opts{path: bin, env: clipEnv})
	comment(tm, "hi")
	tm.keys("y")
	tm.waitFor("copy failed")
	tm.waitFor("💬 hi")
	tm.keys("f")
	tm.waitFor("⋯ 32 unmodified lines")
}
