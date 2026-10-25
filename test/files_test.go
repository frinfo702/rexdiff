package test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rednafi/rexdiff/internal/testgit"
)

// TestFilesTab checks that the Files tab lists all the files, opens a
// file, expands an ignored directory, and renders markdown.
func TestFilesTab(t *testing.T) {
	t.Parallel()
	tm := start(t, newRepo(t), opts{})
	tm.keys("2")
	tm.waitFor("Files (6)", "docs/guide.txt", "▸ out/", "long.txt")
	tm.keys("tab", "j", "right")
	tm.waitFor("▾ out/", "build.log")
	tm.keys("j", "j", "j")
	tm.waitFor("# Title")
	tm.keys("m")
	tm.waitGone("# Title")
	tm.waitFor("Title", "Some text.")
}

// TestSearch checks that / finds files by name and lines by code, shows a
// preview, and opens the pick. A changed file opens in the Changes tab.
func TestSearch(t *testing.T) {
	t.Parallel()
	tm := start(t, newRepo(t), opts{})
	tm.keys("/", "l", "o", "n", "g")
	tm.waitFor("files 1", "▶ long.txt", "preview · long.txt", "first")
	tm.keys("enter")
	tm.waitGone("preview ·")
	tm.waitFor("first", "⋯ 32 unmodified lines")

	tm.keys("/", "tab", "l", "i", "n", "e", " ", "7")
	tm.waitFor("code 1", "7: line 7", "▶   7  line 7")
	tm.keys("enter")
	tm.waitUntil(func(s string) bool {
		for _, l := range lines(s, "▶") {
			if strings.Contains(l, "line 7") {
				return true
			}
		}
		return false
	}, "put the cursor on line 7")
}

// TestLiveReload checks that the Changes tab shows an edit and a new file
// without a key.
func TestLiveReload(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	tm := start(t, repo, opts{})
	tm.waitFor("Changes (2)")
	testgit.WriteFile(t, repo, "new/c.go", "package c\n")
	tm.waitFor("Changes (3)", "c.go")
	testgit.WriteFile(t, repo, "a.go", "package a\n\nfunc A() int { return 3 }\n")
	tm.waitFor("return 3 }")
	if err := os.Remove(filepath.Join(repo, "new", "c.go")); err != nil {
		t.Fatal(err)
	}
	tm.waitFor("Changes (2)")
}

// TestLastTurn checks that the last-turn scope shows what the agent
// changes in its last turn.
func TestLastTurn(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	rex := newRex(t, repo)
	tm := start(t, repo, opts{rex: rex})
	tm.keys("t")
	// rexdiff takes the states of an agent after its first list of them.
	tm.waitFor("waiting for the first turn", "send → 1 agent")
	rex.setState(t, "working")
	time.Sleep(500 * time.Millisecond) // the turn starts with a snapshot
	testgit.WriteFile(t, repo, "turn.go", "package a\n")
	rex.setState(t, "idle")
	tm.waitFor("Changes (1)", "turn.go", "pi")
}

// TestPR checks that the PR tab shows the pull request of the branch from
// gh, with its checks and comments, and that o opens it.
func TestPR(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	testgit.Git(t, repo, "remote", "add", "origin", "https://github.com/o/r.git")
	bin := t.TempDir()
	tool(t, bin, "gh", `cat <<'EOF'
{"number": 7, "title": "Add b", "state": "OPEN", "isDraft": false,
 "url": "https://github.com/o/r/pull/7", "body": "The body of the PR.",
 "author": {"login": "ann"}, "headRefName": "feat", "baseRefName": "main",
 "comments": [{"author": {"login": "bob"}, "body": "Looks good.",
   "createdAt": "2026-01-02T03:04:05Z"}],
 "statusCheckRollup": [{"name": "test", "conclusion": "SUCCESS"},
   {"name": "lint", "conclusion": "FAILURE"}]}
EOF
`)
	tool(t, bin, "opener", `printf '%s\n' "$@" > "${0%/*}/opened"`+"\n")
	tm := start(t, repo, opts{path: bin, args: []string{
		"-config",
		`{"url_opener": "opener {url}"}`,
	}})
	tm.keys("3")
	tm.waitFor("Add b #7", "● open", "feat → main", "by ann",
		"checks: ✓ 1  ✗ 1", "✗ lint", "The body of the PR.", "bob",
		"Looks good.")
	tm.keys("o")
	path := filepath.Join(bin, "opened")
	deadline := time.Now().Add(10 * time.Second)
	for {
		b, _ := os.ReadFile(path)
		if string(b) == "https://github.com/o/r/pull/7\n" {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the opener got %q, want the URL of the PR", b)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
