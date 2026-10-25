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
