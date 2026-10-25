package test

import (
	"strings"
	"testing"

	"github.com/rednafi/rexdiff/internal/testgit"
)

// TestChanges checks the Changes tab: the file list with its counts, the
// diff of a file, the next file, and a fold that opens.
func TestChanges(t *testing.T) {
	t.Parallel()
	tm := start(t, newRepo(t), opts{})
	tm.waitFor("Changes (2)", "M a.go", "+2 -1", "M long.txt", "+2 -2",
		"return 1", "x := 2", "not in Rex")
	tm.keys("f")
	tm.waitFor("first", "last", "⋯ 32 unmodified lines")
	tm.keys("j", "j", "j", "j", "j", "j", "right")
	tm.waitFor("line 20")
	tm.waitGone("unmodified lines")
	tm.keys("F")
	tm.waitFor("x := 2")
}

// TestScopes checks that each scope shows its changes.
func TestScopes(t *testing.T) {
	t.Parallel()
	tm := start(t, newRepo(t), opts{})
	tm.keys("b")
	tm.waitFor("vs main", "Changes (3)", "A b.go")
	tm.keys("u")
	tm.waitFor("Changes (2)")
	tm.waitGone("b.go")
	tm.keys("t")
	tm.waitFor("waiting for the first turn")
	tm.keys("g")
	tm.waitFor("commits · 1 over main", "add b")
	tm.keys("enter")
	tm.waitFor("Changes (1)", "A b.go", "var B = 2")
	tm.keys("u", "B")
	tm.waitFor("base branch for the branch scope")
	tm.keys("f", "e", "a", "t", "enter")
	tm.waitFor("vs feat", "Changes (2)")
}

// TestSplit checks that | shows the old and the new lines side by side
// on a wide screen, and tells why it does not on a narrow one.
func TestSplit(t *testing.T) {
	t.Parallel()
	tm := start(t, newRepo(t), opts{width: 220})
	tm.keys("|")
	tm.waitUntil(func(s string) bool {
		for _, l := range lines(s, "return 1") {
			if strings.Contains(l, "x := 2") {
				return true
			}
		}
		return false
	}, "show return 1 beside x := 2")

	narrow := start(t, newRepo(t), opts{width: 120})
	narrow.keys("|")
	narrow.waitFor("too narrow for split")
}

// TestWrap checks that rexdiff wraps a long line, that w cuts it, and
// that → scrolls to its end.
func TestWrap(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	long := "var long = \"" + strings.Repeat("ab", 60) + "end\"\n"
	testgit.WriteFile(t, repo, "b.go", "package a\n\n"+long)
	tm := start(t, repo, opts{width: 120})
	tm.keys("f")
	tm.waitFor("end\"")
	tm.keys("w")
	tm.waitGone("end\"")
	for i := 0; !strings.Contains(tm.screen(), "end\""); i++ {
		if i == 30 {
			t.Fatalf("→ does not scroll to the end. The screen shows:\n%s", tm.screen())
		}
		tm.keys("right")
	}
}

// TestFileList checks that p moves the file list, z hides it, and > and
// < change its width.
func TestFileList(t *testing.T) {
	t.Parallel()
	tm := start(t, newRepo(t), opts{})
	col := func() int {
		ls := lines(tm.screen(), "Changes (2)")
		if len(ls) == 0 {
			return -1
		}
		return strings.Index(ls[0], "Changes (2)")
	}
	right := col()
	tm.keys(">")
	if c := col(); c <= right {
		t.Errorf("after >, the list starts at column %d, want after %d", c, right)
	}
	tm.keys("<", "<")
	if c := col(); c >= right {
		t.Errorf("after <, the list starts at column %d, want before %d", c, right)
	}
	tm.keys("p") // bottom
	tm.waitUntil(func(s string) bool {
		ls := strings.Split(s, "\n")
		for i, l := range ls {
			if strings.Contains(l, "Changes (2)") {
				return i > 10
			}
		}
		return false
	}, "show the list at the bottom")
	tm.keys("z")
	tm.waitGone("Changes (2)")
	tm.keys("z")
	tm.waitFor("Changes (2)")
}

// TestKeys checks that ? shows all the keys.
func TestKeys(t *testing.T) {
	t.Parallel()
	tm := start(t, newRepo(t), opts{})
	tm.keys("?")
	tm.waitFor("search files & code", "pick commits", "move file list")
	tm.keys("esc")
	tm.waitGone("pick commits")
}

// TestGotoLine checks that : moves the cursor to a line of the new file.
func TestGotoLine(t *testing.T) {
	t.Parallel()
	tm := start(t, newRepo(t), opts{})
	tm.keys("f", ":", "3", "3", "enter")
	tm.waitUntil(func(s string) bool {
		for _, l := range lines(s, "▶") {
			if strings.Contains(l, "line 33") {
				return true
			}
		}
		return false
	}, "put the cursor on line 33")
}

// TestFind checks that ctrl+f counts the lines with a text, and that
// enter moves to a hit.
func TestFind(t *testing.T) {
	t.Parallel()
	tm := start(t, newRepo(t), opts{})
	tm.keys("f", "ctrl+f", "l", "i", "n", "e", " ", "3")
	tm.waitFor("find line 3", " 11") // line 3, and line 30 to line 39
	tm.keys("enter")
	tm.waitUntil(func(s string) bool {
		for _, l := range lines(s, "▶") {
			if strings.Contains(l, "line 3") {
				return true
			}
		}
		return false
	}, "put the cursor on a hit")
}

// TestSettings checks that the settings change the layout, the marks of
// the changed lines, the place of the file list, and the keys.
func TestSettings(t *testing.T) {
	t.Parallel()
	tm := start(t, newRepo(t), opts{width: 220, args: []string{
		"-config",
		`{"layout": "split", "diff_signs": true, "file_list": "left",` +
			` "keybindings": {"next-file": ["x"]}}`,
	}})
	s := tm.waitFor("4 -     return 1", "4 +     x := 2", "x next/prev file")
	for _, l := range lines(s, "return 1") {
		if !strings.Contains(l, "x := 2") {
			t.Errorf("line %q, want the old and the new line side by side", l)
		}
	}
	if ls := lines(s, "Changes (2)"); len(ls) == 0 || strings.Index(ls[0], "Changes (2)") > 5 {
		t.Errorf("the list starts at %q, want it at the left", ls)
	}
	tm.keys("x")
	tm.waitFor("first")
}

// TestTinyScreen checks that rexdiff draws and takes keys in a small
// terminal.
func TestTinyScreen(t *testing.T) {
	t.Parallel()
	tm := start(t, newRepo(t), opts{width: 40, height: 8})
	tm.keys("f", "j", "2", "/", "esc", "?", "esc", "1")
	tm.waitFor("1 Changes")
}

// TestHostileText checks that the escape codes in a file show as text, so
// a file cannot change the terminal.
func TestHostileText(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	testgit.WriteFile(t, repo, "evil.txt", "a\x1b[2Jb\x1b]0;title\x07c\n")
	tm := start(t, repo, opts{})
	tm.keys("f")
	tm.waitFor("a␛[2Jb␛]0;title␇c")
}
