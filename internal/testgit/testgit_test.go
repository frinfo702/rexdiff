package testgit_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rednafi/rexdiff/internal/testgit"
)

func TestMain(m *testing.M) { testgit.Main(m) }

// TestRepoIgnoresUser checks that a new repository reads only its own
// config and gets no hooks from a template.
func TestRepoIgnoresUser(t *testing.T) {
	dir := t.TempDir()
	testgit.NewRepo(t, dir)
	for l := range strings.Lines(testgit.Git(t, dir, "config", "--list", "--show-origin")) {
		if !strings.HasPrefix(l, "file:.git/config\t") {
			t.Errorf("git config has %q, want only the config of the repository", l)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, ".git", "hooks")); !os.IsNotExist(err) {
		t.Errorf("stat .git/hooks: %v, want no hooks directory", err)
	}
}

// TestHelpers checks the files that WriteFile and Script make, and that
// a commit gets the fixed author.
func TestHelpers(t *testing.T) {
	dir := t.TempDir()
	testgit.NewRepo(t, dir)
	testgit.WriteFile(t, dir, "a/b/c.txt", "x\n")
	testgit.Git(t, dir, "add", ".")
	testgit.Git(t, dir, "commit", "-qm", "c")
	if got := testgit.Git(t, dir, "log", "-1", "--format=%an <%ae>"); got != "t <t@t>\n" {
		t.Errorf("author %q, want t <t@t>", got)
	}
	p := testgit.Script(t, t.TempDir(), "tool", "echo hi\n")
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm()&0o100 == 0 {
		t.Errorf("%s has mode %v, want an executable", p, fi.Mode())
	}
}

// TestFixture checks that a fixture builds once, and that each copy is a
// repository of its own.
func TestFixture(t *testing.T) {
	builds := 0
	f := testgit.NewFixture(func(tb testing.TB, dir string) {
		tb.Helper()
		builds++
		testgit.NewRepo(tb, dir)
		testgit.WriteFile(tb, dir, "a.txt", "a\n")
		testgit.Git(tb, dir, "add", ".")
		testgit.Git(tb, dir, "commit", "-qm", "a")
	})
	one, two := t.TempDir(), t.TempDir()
	f.Copy(t, one)
	f.Copy(t, two)
	testgit.WriteFile(t, one, "a.txt", "changed\n")
	if builds != 1 {
		t.Errorf("%d builds, want 1", builds)
	}
	if got := testgit.Git(t, one, "status", "--porcelain"); got != " M a.txt\n" {
		t.Errorf("status of the changed copy = %q, want  M a.txt", got)
	}
	if got := testgit.Git(t, two, "status", "--porcelain"); got != "" {
		t.Errorf("status of the other copy = %q, want clean", got)
	}
}
