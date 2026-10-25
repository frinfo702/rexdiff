package test

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/rednafi/rexdiff/internal/testgit"
)

// repoFixture is a repository with a commit on main and on the branch
// feat. It has uncommitted edits of a.go, and of the first and the last
// line of long.txt.
var repoFixture = testgit.NewFixture(func(tb testing.TB, dir string) {
	tb.Helper()
	testgit.NewRepo(tb, dir)
	testgit.WriteFile(tb, dir, "a.go", "package a\n\nfunc A() int {\n\treturn 1\n}\n")
	testgit.WriteFile(tb, dir, "README.md", "# Title\n\nSome text.\n")
	testgit.WriteFile(tb, dir, "docs/guide.txt", "guide\n")
	testgit.WriteFile(tb, dir, ".gitignore", "out/\n")
	testgit.WriteFile(tb, dir, "long.txt", numbered("line", 1, 40))
	testgit.Git(tb, dir, "add", ".")
	testgit.Git(tb, dir, "commit", "-qm", "first")
	testgit.Git(tb, dir, "checkout", "-qb", "feat")
	testgit.WriteFile(tb, dir, "b.go", "package a\n\nvar B = 2\n")
	testgit.Git(tb, dir, "add", ".")
	testgit.Git(tb, dir, "commit", "-qm", "add b")
	testgit.WriteFile(tb, dir, "a.go", "package a\n\nfunc A() int {\n\tx := 2\n\treturn x\n}\n")
	testgit.WriteFile(tb, dir, "out/build.log", "built\n")
	testgit.WriteFile(tb, dir, "long.txt",
		"first\n"+numbered("line", 2, 39)+"last\n")
})

// numbered returns the lines "<word> <n>" for n from first to last.
func numbered(word string, first, last int) string {
	var b strings.Builder
	for n := first; n <= last; n++ {
		b.WriteString(word + " " + strconv.Itoa(n) + "\n")
	}
	return b.String()
}

// newRepo returns a copy of the fixture repository. The path has no
// symbolic link, as the root that git gives.
func newRepo(t *testing.T) string {
	t.Helper()
	tmp, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(tmp, "repo")
	repoFixture.Copy(t, dir)
	return dir
}

// fakeRex is a rex for the session S. rexdiff runs in the block
// block:self. The agents run in repo. The fake logs each call and prints
// the output of a real rex from testdata/rex, with the values of the test.
// "rex events" prints the lines that the test adds.
type fakeRex struct {
	dir    string
	blocks []string // the JSON of each block
}

// newRex returns a fake rex with the agent pi in the block block:pi.
func newRex(t *testing.T, repo string) *fakeRex {
	t.Helper()
	r := &fakeRex{dir: t.TempDir()}
	testgit.Script(t, r.dir, "rex", `d="${0%/*}"
printf '%s\n' "$*" >> "$d/calls"
case "$*" in
"api call -s S session.list_blocks {}") cat "$d/blocks" ;;
"session inspect S --json") cat "$d/session" ;;
"block call -s S -b "*" program_status") cat "$d/status.$6" ;;
"block call -s S -b "*" process") cat "$d/process" ;;
"block inspect "*) cat "$d/inspect.$3" ;;
"events -s S --json") touch "$d/events"; exec tail -n +1 -f "$d/events" ;;
esac
`)
	r.write(t, "session", recorded(t, "session.json"))
	r.write(t, "process", recorded(t, "process.json", "{{CWD}}", repo))
	r.addBlock(t, "block:self", "rexdiff")
	r.addAgent(t, "block:pi", "agent", "pi")
	return r
}

// recorded returns the file name of testdata/rex, with each pair of
// old and new values replaced.
func recorded(t *testing.T, name string, oldNew ...string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "rex", name))
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(strings.NewReplacer(oldNew...).Replace(string(b)))
}

// addBlock adds a terminal block with the label.
func (r *fakeRex) addBlock(t *testing.T, block, label string) {
	t.Helper()
	r.blocks = append(r.blocks, recorded(t, "block.json", "{{ID}}", block, "{{LABEL}}", label))
	r.write(t, "blocks", `{"blocks":[`+strings.Join(r.blocks, ",")+`]}`)
	r.write(t, "inspect."+label, recorded(t, "inspect.json", "{{ID}}", block, "{{LABEL}}", label))
}

// addAgent adds a block with the label, and an idle agent app in it.
func (r *fakeRex) addAgent(t *testing.T, block, label, app string) {
	t.Helper()
	r.addBlock(t, block, label)
	r.write(t, "status."+block, recorded(t, "status.json", "{{STATE}}", "idle", "{{APP}}", app))
}

// write writes content to the file name of the fake.
func (r *fakeRex) write(t *testing.T, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(r.dir, name), []byte(content+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// calls returns the calls of rex, one a line.
func (r *fakeRex) calls(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(r.dir, "calls"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return string(b)
}

// setState sets the state of the agent pi and sends the event of the
// change.
func (r *fakeRex) setState(t *testing.T, state string) {
	t.Helper()
	values := []string{"{{ID}}", "block:pi", "{{STATE}}", state, "{{APP}}", "pi"}
	r.write(t, "status.block:pi", recorded(t, "status.json", values...))
	f, err := os.OpenFile(filepath.Join(r.dir, "events"), os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(recorded(t, "event.json", values...) + "\n"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

// tool writes a fake program name with body to dir, and returns dir.
func tool(t *testing.T, dir, name, body string) string {
	t.Helper()
	testgit.Script(t, dir, name, body)
	return dir
}

// lines returns the lines of s that contain text.
func lines(s, text string) []string {
	var out []string
	for l := range strings.Lines(s) {
		if strings.Contains(l, text) {
			out = append(out, strings.TrimRight(l, "\n"))
		}
	}
	return out
}
