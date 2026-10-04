package vcs_test

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/rednafi/rexdiff/vcs"
)

// commitAll commits all the changes in the working tree of dir.
func commitAll(t *testing.T, dir string) {
	t.Helper()
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-qm", "all")
}

// A rev that starts with "-" is a rev. git must not read it as an option.
func TestRevIsNotAnOption(t *testing.T) {
	r, _ := newRepo(t)
	ctx := t.Context()
	out := filepath.Join(t.TempDir(), "out")
	write(t, filepath.Dir(out), "out", "keep\n")
	for _, rev := range []string{"--output=" + out, "--independent", "--all", "-h"} {
		if id, err := r.Resolve(ctx, rev); err == nil {
			t.Errorf("Resolve(%q) = %s, want an error", rev, id)
		}
		if id, err := r.Tree(ctx, rev); err == nil {
			t.Errorf("Tree(%q) = %s, want an error", rev, id)
		}
		if id, err := r.MergeBase(ctx, rev); err == nil {
			t.Errorf("MergeBase(%q) = %s, want an error", rev, id)
		}
		if id, err := r.Parent(ctx, rev); err == nil {
			t.Errorf("Parent(%q) = %s, want an error", rev, id)
		}
		if _, err := r.Diff(ctx, rev, head(t, r)); err == nil {
			t.Errorf("Diff(%q, HEAD): no error", rev)
		}
	}
	if b, _ := os.ReadFile(out); string(b) != "keep\n" {
		t.Errorf("git changed the file of --output: %q", b)
	}
}

// TestHostilePaths checks that each part of the API keeps a path as it is.
func TestHostilePaths(t *testing.T) {
	r, dir := newRepo(t)
	ctx := t.Context()
	commitAll(t, dir)
	paths := []string{
		"-n", "--output=x", "-", "tab\tname", "new\nline", "trail\n",
		`say "hi"`, `back\slash`, "ünï cödé", " spaces ",
		"*", "[a]", ":(glob)x", "\xff\xfe",
	}
	want := map[string]string{} // the content of each path
	for i, p := range paths {
		s := fmt.Sprintf("id%02d\n", i)
		if err := os.WriteFile(filepath.Join(dir, p), []byte(s), 0o644); err != nil {
			t.Logf("skip %q: %v", p, err) // for example, macOS needs UTF-8 names
			continue
		}
		want[p] = s
	}

	got := map[string]string{}
	for _, f := range changes(t, r, head(t, r), snapshot(t, r)) {
		if f.Status != vcs.StatusAdded {
			t.Errorf("Diff: %q has status %c, want A", f.Path, f.Status)
		}
		got[f.Path] = f.New
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Diff:\n%q\nwant:\n%q", got, want)
	}

	es, err := r.Files(ctx)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]bool{}
	for _, e := range es {
		files[e.Path] = true
	}
	for p, s := range want {
		if !files[p] {
			t.Errorf("Files does not have %q", p)
		}
		if got, err := r.ReadFile(p); got != s || err != nil {
			t.Errorf("ReadFile(%q) = %q, %v; want %q", p, got, err, s)
		}
		q := strings.TrimSpace(s)
		if ms, err := r.Grep(ctx, q, 10); err != nil || len(ms) != 1 || ms[0].Path != p {
			t.Errorf("Grep(%q) = %+v, %v; want one match in %q", q, ms, err, p)
		}
	}
}

func TestDiffFileKinds(t *testing.T) {
	r, dir := newRepo(t)
	git(t, dir, "config", "core.autocrlf", "false")
	write(t, dir, "mode.sh", "echo\n")
	write(t, dir, "dir", "a file\n")
	write(t, dir, "type", "a file\n")
	commitAll(t, dir)

	outside := t.TempDir()
	link := func(target, name string) {
		t.Helper()
		if err := os.Symlink(target, filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}
	link("a.txt", "link")
	link(outside, "out")
	if err := os.Chmod(filepath.Join(dir, "mode.sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "dir")); err != nil {
		t.Fatal(err)
	}
	write(t, dir, "dir/in", "in\n")
	if err := os.Remove(filepath.Join(dir, "type")); err != nil {
		t.Fatal(err)
	}
	link("a.txt", "type")
	write(t, dir, "crlf", "a\r\nb\r\n")
	write(t, dir, "empty", "")
	write(t, dir, "no-newline", "end")

	want := []vcs.File{
		{Path: "crlf", Status: vcs.StatusAdded, New: "a\r\nb\r\n"},
		{Path: "dir", Status: vcs.StatusDeleted, Old: "a file\n"},
		{Path: "dir/in", Status: vcs.StatusAdded, New: "in\n"},
		{Path: "empty", Status: vcs.StatusAdded},
		// git keeps the target of a link.
		{Path: "link", Status: vcs.StatusAdded, New: "a.txt"},
		{Path: "mode.sh", Status: vcs.StatusModified, Old: "echo\n", New: "echo\n"},
		{Path: "no-newline", Status: vcs.StatusAdded, New: "end"},
		{Path: "out", Status: vcs.StatusAdded, New: outside},
		{Path: "type", Status: vcs.StatusModified, Old: "a file\n", New: "a.txt"},
	}
	if got := changes(t, r, head(t, r), snapshot(t, r)); !reflect.DeepEqual(got, want) {
		t.Errorf("Diff:\n%+v\nwant:\n%+v", got, want)
	}
}

// A git repository in the working tree that has no commit cannot be added.
// Snapshot must add the other files.
func TestSnapshotNestedRepo(t *testing.T) {
	r, dir := newRepo(t)
	commitAll(t, dir)
	git(t, dir, "init", "-q", "empty")
	write(t, dir, "empty/x", "x\n")
	write(t, dir, "full/y", "y\n")
	git(t, filepath.Join(dir, "full"), "init", "-q")
	commitAll(t, filepath.Join(dir, "full"))
	write(t, dir, "z.txt", "z\n")

	got := changes(t, r, head(t, r), snapshot(t, r))
	isSubmodule := strings.HasPrefix(got[0].New, "Subproject commit ")
	if names(got) != "Afull Az.txt" || !isSubmodule {
		t.Errorf("Diff = %+v, want full as a submodule and z.txt", got)
	}
}

// TestReadFileStaysInRoot checks that ReadFile reads only the regular files
// in the root. A repository can link to a secret file, or to a device that
// does not end, for example /dev/zero.
func TestReadFileStaysInRoot(t *testing.T) {
	r, dir := newRepo(t)
	secret := filepath.Join(filepath.Dir(dir), "secret")
	write(t, filepath.Dir(dir), "secret", "secret\n")
	links := map[string]string{
		"to-secret": secret,
		"to-zero":   "/dev/zero",
		"to-dir":    "build",
	}
	for name, target := range links {
		if err := os.Symlink(target, filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}
	if err := exec.Command("mkfifo", filepath.Join(dir, "fifo")).Run(); err != nil {
		t.Fatal(err)
	}
	bad := []string{"../secret", secret, "to-secret", "to-zero", "to-dir", "fifo"}
	for _, p := range bad {
		if s, err := r.ReadFile(p); err == nil {
			t.Errorf("ReadFile(%q) = %q, want an error", p, s)
		}
	}
	if err := os.Symlink("a.txt", filepath.Join(dir, "to-a")); err != nil {
		t.Fatal(err)
	}
	if s, err := r.ReadFile("to-a"); s != "one\ntwo\n" || err != nil {
		t.Errorf("ReadFile(to-a) = %q, %v; want the content of a.txt", s, err)
	}
}

func TestOpenRootWithNewline(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "a\nb")
	git(t, filepath.Dir(dir), "init", "-q", dir)
	write(t, dir, "a.txt", "one\n")
	r, err := vcs.Open(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if want, _ := filepath.EvalSymlinks(dir); r.Root() != want {
		t.Errorf("Root = %q, want %q", r.Root(), want)
	}
	if got := names(changes(t, r, head(t, r), snapshot(t, r))); got != "Aa.txt" {
		t.Errorf("Diff(Head, Snapshot) = %s, want Aa.txt", got)
	}
}

// TestGitWorktree checks a working tree that "git worktree add" made. It
// has its own index.
func TestGitWorktree(t *testing.T) {
	_, dir := newRepo(t)
	ctx := t.Context()
	wt := filepath.Join(t.TempDir(), "wt")
	git(t, dir, "worktree", "add", "-q", "-b", "other", wt, "main")
	write(t, wt, "a.txt", "changed\n")
	write(t, wt, "staged.txt", "s\n")
	git(t, wt, "add", "staged.txt")

	r, err := vcs.Open(ctx, wt)
	if err != nil {
		t.Fatal(err)
	}
	got := names(changes(t, r, head(t, r), snapshot(t, r)))
	if got != "Ma.txt Astaged.txt" {
		t.Errorf("Diff(Head, Snapshot) = %s, want Ma.txt Astaged.txt", got)
	}
	st, _ := exec.Command("git", "-C", wt, "status", "--porcelain").Output()
	if string(st) != " M a.txt\nA  staged.txt\n" {
		t.Errorf("the index of the worktree changed:\n%s", st)
	}
}

func TestDetachedHead(t *testing.T) {
	r, dir := newRepo(t)
	ctx := t.Context()
	git(t, dir, "checkout", "-q", "--detach", "main")
	got := names(changes(t, r, head(t, r), snapshot(t, r)))
	if got != "A.gitignore Ma.txt Anew.txt" {
		t.Errorf("Diff(Head, Snapshot) = %s", got)
	}
	if cs, err := r.Commits(ctx, 5); err != nil || len(cs) != 1 || cs[0].Subject != "init" {
		t.Errorf("Commits = %+v, %v; want init", cs, err)
	}
}

// TestGrepIsLiteral checks that Grep finds the text, and does not read it
// as a pattern or as an option.
func TestGrepIsLiteral(t *testing.T) {
	r, dir := newRepo(t)
	ctx := t.Context()
	write(t, dir, "g.txt", "abc\na.c\n-v\n--help\n[x\n(y\n\\d\n*\n^a\nx$\n")
	git(t, dir, "config", "grep.column", "true") // adds a field to the output
	tests := []struct {
		q    string
		want []string
	}{
		{"a.c", []string{"a.c"}},
		{"-v", []string{"-v"}},
		{"--help", []string{"--help"}},
		{"[x", []string{"[x"}},
		{"(y", []string{"(y"}},
		{`\d`, []string{`\d`}},
		{"*", []string{"*"}},
		{"^a", []string{"^a"}},
		{"x$", []string{"x$"}},
		{"abc\na.c", nil}, // no line has a newline
	}
	for _, tt := range tests {
		ms, err := r.Grep(ctx, tt.q, 10)
		if err != nil {
			t.Errorf("Grep(%q): %v", tt.q, err)
			continue
		}
		var got []string
		for _, m := range ms {
			got = append(got, m.Text)
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("Grep(%q) = %q, want %q", tt.q, got, tt.want)
		}
	}
}

// A subject can have any byte, for example the separator of the fields of
// the log.
func TestCommitsHostileSubject(t *testing.T) {
	r, dir := newRepo(t)
	subjects := []string{"a\x1fb", "\x1b[31mred\x1b[0m", "tab\there"}
	for _, s := range subjects {
		git(t, dir, "commit", "-q", "--allow-empty", "-m", s)
	}
	cs, err := r.Commits(t.Context(), 3)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, c := range cs {
		got = append(got, c.Subject)
		if c.Author != "t" {
			t.Errorf("Author of %q = %q, want t", c.Subject, c.Author)
		}
	}
	want := []string{subjects[2], subjects[1], subjects[0]}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Commits subjects = %q, want %q", got, want)
	}
}

// TestPullRequestBadJSON checks the output of gh that is not a normal pull
// request.
func TestPullRequestBadJSON(t *testing.T) {
	tests := []struct {
		name, out string
		want      error // nil for an error of any type
		ok        bool  // true if PullRequest must not fail
	}{
		{name: "empty object", out: `{}`, want: vcs.ErrNotFound},
		{name: "not JSON", out: `warning: no pull request`},
		{name: "huge number", out: `{"title": "T", "number": 1e30}`},
		{
			name: "null fields",
			out: `{"title": "T", "author": null, "comments": null, ` +
				`"statusCheckRollup": null}`,
			ok: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bin := t.TempDir()
			fakePath(t, bin)
			fakeTool(t, bin, "gh", map[string]string{ghArgs: tt.out})
			r, _ := repoAt(t, "https://github.com/o/r")
			pr, err := r.PullRequest(t.Context(), vcs.Hosts{})
			switch {
			case tt.ok && (err != nil || pr.Title != "T"):
				t.Errorf("PullRequest = %+v, %v; want the title T", pr, err)
			case !tt.ok && err == nil:
				t.Errorf("PullRequest = %+v, want an error", pr)
			case tt.want != nil && !errors.Is(err, tt.want):
				t.Errorf("PullRequest: err = %v, want %v", err, tt.want)
			}
		})
	}
}
