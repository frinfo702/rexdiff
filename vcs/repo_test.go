package vcs_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/rednafi/rexdiff/vcs"
)

// newRepo makes a repository with two commits: "init" on main and "b" on
// the branch feat. The working tree has uncommitted changes, an untracked
// file, and an ignored file.
func newRepo(t *testing.T) (*vcs.Repo, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "a repo ") // git keeps the space
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) { git(t, dir, args...) }
	run("init", "-q", "-b", "main")
	write(t, dir, "a.txt", "one\n")
	run("add", ".")
	run("commit", "-qm", "init")
	run("checkout", "-qb", "feat")
	write(t, dir, "b.txt", "bee\n")
	run("add", ".")
	run("commit", "-qm", "b")
	write(t, dir, "a.txt", "one\ntwo\n")
	write(t, dir, "new.txt", "untracked\n")
	write(t, dir, ".gitignore", "ignored.txt\nbuild/\n")
	write(t, dir, "ignored.txt", "x\n")
	write(t, dir, "build/out", "x\n")

	// Open from a subdirectory. Open must find the root.
	r, err := vcs.Open(t.Context(), filepath.Join(dir, "build"))
	if err != nil {
		t.Fatal(err)
	}
	return r, dir
}

// git runs git in dir and fails the test if git fails.
func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t",
		"GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t",
		"GIT_COMMITTER_EMAIL=t@t",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %s", args, out)
	}
}

// write writes s to the file name in dir, and makes its parent directories.
func write(t *testing.T, dir, name, s string) {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

// snapshot returns the tree of the working tree.
func snapshot(t *testing.T, r *vcs.Repo) string {
	t.Helper()
	tree, err := r.Snapshot(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return tree
}

// head returns the tree of HEAD.
func head(t *testing.T, r *vcs.Repo) string {
	t.Helper()
	tree, err := r.Head(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return tree
}

// parent returns the tree of the parent of rev.
func parent(t *testing.T, r *vcs.Repo, rev string) string {
	t.Helper()
	tree, err := r.Parent(t.Context(), rev)
	if err != nil {
		t.Fatal(err)
	}
	return tree
}

// changes returns the files that differ from the tree from to the tree to.
func changes(t *testing.T, r *vcs.Repo, from, to string) []vcs.File {
	t.Helper()
	fs, err := r.Diff(t.Context(), from, to)
	if err != nil {
		t.Fatal(err)
	}
	return fs
}

// names returns the status and the path of each file, for example
// "Aa.txt Mb.txt".
func names(fs []vcs.File) string {
	var n []string
	for _, f := range fs {
		n = append(n, string(f.Status)+f.Path)
	}
	return strings.Join(n, " ")
}

// TestScopes checks the trees that each scope of a review compares.
func TestScopes(t *testing.T) {
	r, dir := newRepo(t)
	ctx := t.Context()

	now := snapshot(t, r)
	unc := changes(t, r, head(t, r), now)
	if got := names(unc); got != "A.gitignore Ma.txt Anew.txt" {
		t.Errorf("uncommitted: %s, want A.gitignore Ma.txt Anew.txt", got)
	}
	if unc[1].Old != "one\n" || unc[1].New != "one\ntwo\n" {
		t.Errorf("a.txt: Old %q, New %q; want %q, %q",
			unc[1].Old, unc[1].New, "one\n", "one\ntwo\n")
	}

	mb, err := r.MergeBase(ctx, "main")
	if err != nil {
		t.Fatal(err)
	}
	if got := names(changes(t, r, mb, now)); got != "A.gitignore Ma.txt Ab.txt Anew.txt" {
		t.Errorf("branch: %s, want A.gitignore Ma.txt Ab.txt Anew.txt", got)
	}
	if _, err := r.MergeBase(ctx, "nosuchbranch"); err == nil {
		t.Error("MergeBase(nosuchbranch): no error, want an error")
	}

	// In a turn, rexdiff takes a snapshot and then the agent edits a file.
	write(t, dir, "new.txt", "untracked\nmore\n")
	tf := changes(t, r, now, snapshot(t, r))
	if got := names(tf); got != "Mnew.txt" || tf[0].New != "untracked\nmore\n" {
		t.Errorf("turn: %s, want Mnew.txt with the new line", got)
	}

	// Snapshot must not change the real index.
	st, _ := exec.Command("git", "-C", dir, "status", "--porcelain").Output()
	if !strings.Contains(string(st), "?? new.txt") {
		t.Errorf("git status after Snapshot = %q, want ?? new.txt", st)
	}

	cs, err := r.Commits(ctx, 5)
	if err != nil || len(cs) != 2 || cs[0].Subject != "b" || cs[1].Subject != "init" {
		t.Fatalf("Commits = %+v, %v, want b and init", cs, err)
	}
	tree, err := r.Tree(ctx, cs[0].SHA)
	if err != nil {
		t.Fatal(err)
	}
	if got := names(changes(t, r, parent(t, r, cs[0].SHA), tree)); got != "Ab.txt" {
		t.Errorf("commit b: %s, want Ab.txt", got)
	}
	// emptyTree is the ID of the tree that has no files.
	const emptyTree = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"
	if got := parent(t, r, cs[1].SHA); got != emptyTree {
		t.Errorf("parent of the root commit = %s, want the empty tree", got)
	}
}

// TestMissingCommit checks that Head and Parent fail when a commit object
// is missing.
func TestMissingCommit(t *testing.T) {
	// The empty tree is not a correct result. It shows every file as added.
	r, dir := newRepo(t)
	ctx := t.Context()
	cs, err := r.Commits(ctx, 2)
	if err != nil || len(cs) != 2 {
		t.Fatalf("Commits = %+v, %v, want two commits", cs, err)
	}
	remove := func(sha string) {
		t.Helper()
		obj := filepath.Join(dir, ".git", "objects", sha[:2], sha[2:])
		if err := os.Remove(obj); err != nil {
			t.Fatal(err)
		}
	}
	remove(cs[1].SHA)
	if id, err := r.Parent(ctx, "HEAD"); err == nil {
		t.Errorf("Parent(HEAD) = %s with no parent commit, want an error", id)
	}
	if id, err := r.Parent(ctx, "nope"); err == nil {
		t.Errorf("Parent(nope) = %s, want an error", id)
	}
	remove(cs[0].SHA)
	if id, err := r.Head(ctx); err == nil {
		t.Errorf("Head = %s with no HEAD commit, want an error", id)
	}
}

// TestDiffSkipsBinaryAndBigFiles checks that Diff and ReadFile mark binary
// and big files, and do not read their content.
func TestDiffSkipsBinaryAndBigFiles(t *testing.T) {
	r, dir := newRepo(t)
	write(t, dir, "bin", "a\x00b")
	write(t, dir, "big", strings.Repeat("x\n", 50_001))
	write(t, dir, "huge", strings.Repeat("x", 2<<20+1)) // over 2 MiB
	// Diff reads this file after the skipped content.
	write(t, dir, "small", "after\n")
	got := map[string]vcs.File{}
	for _, f := range changes(t, r, head(t, r), snapshot(t, r)) {
		got[f.Path] = f
	}
	if f := got["bin"]; !f.Binary || f.New != "" {
		t.Errorf("bin: %+v, want Binary with no New", f)
	}
	for _, p := range []string{"big", "huge"} {
		if f := got[p]; !f.TooBig || f.New != "" {
			t.Errorf("%s: TooBig = %v, New has %d bytes; want true, 0",
				p, f.TooBig, len(f.New))
		}
	}
	if f := got["small"]; f.New != "after\n" {
		t.Errorf("small: New = %q, want %q", f.New, "after\n")
	}
	if _, err := r.ReadFile("bin"); !errors.Is(err, vcs.ErrBinary) {
		t.Errorf("ReadFile(bin): err = %v, want ErrBinary", err)
	}
	if _, err := r.ReadFile("huge"); !errors.Is(err, vcs.ErrTooBig) {
		t.Errorf("ReadFile(huge): err = %v, want ErrTooBig", err)
	}
}

// TestFilesInConflict checks that Files lists a path in a merge conflict
// once.
func TestFilesInConflict(t *testing.T) {
	dir := t.TempDir()
	run := func(args ...string) { git(t, dir, args...) }
	run("init", "-q", "-b", "main")
	write(t, dir, "f.txt", "base\n")
	run("add", ".")
	run("commit", "-qm", "base")
	run("checkout", "-qb", "other")
	write(t, dir, "f.txt", "other\n")
	run("commit", "-qam", "other")
	run("checkout", "-q", "main")
	write(t, dir, "f.txt", "main\n")
	run("commit", "-qam", "main")
	cmd := exec.Command("git", "merge", "other")
	cmd.Dir = dir
	if err := cmd.Run(); err == nil {
		t.Fatal("git merge other: no error, want a conflict")
	}
	r, err := vcs.Open(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	es, err := r.Files(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range es {
		got = append(got, e.Path)
	}
	if want := []string{"f.txt"}; !slices.Equal(got, want) {
		t.Errorf("Files = %q, want %q", got, want)
	}
}

// TestWorktree checks Files, Grep, ReadFile, and Contains in the working
// tree.
func TestWorktree(t *testing.T) {
	r, dir := newRepo(t)
	ctx := t.Context()

	es, err := r.Files(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range es {
		if e.Ignored {
			got = append(got, "!"+e.Path)
		} else {
			got = append(got, e.Path)
		}
	}
	want := ".gitignore a.txt b.txt !build/ !ignored.txt new.txt"
	if got := strings.Join(got, " "); got != want {
		t.Errorf("Files = %s, want %s", got, want)
	}

	ms, err := r.Grep(ctx, "TWO", 10)
	match := vcs.Match{Path: "a.txt", Line: 2, Text: "two"}
	if err != nil || len(ms) != 1 || ms[0] != match {
		t.Errorf("Grep(TWO) = %+v, %v, want [%+v], nil", ms, err, match)
	}
	write(t, dir, "a:é\n.txt", "x: two\n") // git quotes this name unless -z
	ms, err = r.Grep(ctx, "x: two", 10)
	match = vcs.Match{Path: "a:é\n.txt", Line: 1, Text: "x: two"}
	if err != nil || len(ms) != 1 || ms[0] != match {
		t.Errorf("Grep(x: two) = %+v, %v, want [%+v], nil", ms, err, match)
	}
	if ms, err := r.Grep(ctx, "nothing has this", 10); err != nil || len(ms) != 0 {
		t.Errorf("Grep with no match = %+v, %v, want none, nil", ms, err)
	}

	if s, err := r.ReadFile("a.txt"); err != nil || s != "one\ntwo\n" {
		t.Errorf("ReadFile(a.txt) = %q, %v, want %q, nil",
			s, err, "one\ntwo\n")
	}
	contains := []struct {
		path string
		want bool
	}{
		{filepath.Join(dir, "build"), true},
		{filepath.Dir(dir), false},
		{dir + "x", false}, // a sibling with the same prefix
	}
	for _, tt := range contains {
		if got := r.Contains(tt.path); got != tt.want {
			t.Errorf("Contains(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

// TestRefs checks Branches, DefaultBase, and Resolve.
func TestRefs(t *testing.T) {
	r, _ := newRepo(t)
	ctx := t.Context()
	if bs, err := r.Branches(ctx); err != nil || strings.Join(bs, " ") != "feat main" {
		t.Errorf("Branches = %v, %v, want [feat main], nil", bs, err)
	}
	if b := r.DefaultBase(ctx); b != "main" {
		t.Errorf("DefaultBase = %q, want main", b)
	}
	if _, err := r.Resolve(ctx, "nosuchref"); err == nil {
		t.Error("Resolve(nosuchref): no error, want an error")
	}
}

// TestEditor checks that Editor returns core.editor, or "" if it is not set.
func TestEditor(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	r, dir := newRepo(t)
	if got := r.Editor(t.Context()); got != "" {
		t.Errorf("Editor with no core.editor = %q, want \"\"", got)
	}
	git(t, dir, "config", "core.editor", "vim -f")
	if got := r.Editor(t.Context()); got != "vim -f" {
		t.Errorf("Editor = %q, want \"vim -f\"", got)
	}
}

// TestDiffRenameAndSubmodule checks the old path of a rename and the
// content that Diff gives for a submodule.
func TestDiffRenameAndSubmodule(t *testing.T) {
	dir := t.TempDir()
	git(t, dir, "init", "-q")
	write(t, dir, "old.txt", "one\ntwo\nthree\n")
	const (
		a = "1111111111111111111111111111111111111111"
		b = "2222222222222222222222222222222222222222"
	)
	git(t, dir, "add", ".")
	git(t, dir, "update-index", "--add", "--cacheinfo", "160000,"+a+",sub")
	git(t, dir, "commit", "-qm", "one")
	git(t, dir, "mv", "old.txt", "new.txt")
	git(t, dir, "update-index", "--cacheinfo", "160000,"+b+",sub")
	git(t, dir, "commit", "-qm", "two")

	ctx := t.Context()
	r, err := vcs.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	from, to := parent(t, r, "HEAD"), head(t, r)
	got, err := r.Diff(ctx, from, to)
	if err != nil {
		t.Fatal(err)
	}
	want := []vcs.File{
		{
			Path:   "new.txt",
			From:   "old.txt",
			Status: vcs.StatusRenamed,
			Old:    "one\ntwo\nthree\n",
			New:    "one\ntwo\nthree\n",
		},
		{
			Path:   "sub",
			Status: vcs.StatusModified,
			Old:    "Subproject commit " + a + "\n",
			New:    "Subproject commit " + b + "\n",
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Diff got:\n%+v\nwant:\n%+v", got, want)
	}
}

// TestOpenOutsideARepo checks that Open fails outside a repository.
func TestOpenOutsideARepo(t *testing.T) {
	if _, err := vcs.Open(t.Context(), t.TempDir()); err == nil {
		t.Error("Open outside a repository: no error, want an error")
	}
}

// TestNewRepo checks Head and Snapshot in a repository with no commit and
// no index.
func TestNewRepo(t *testing.T) {
	dir := t.TempDir()
	git(t, dir, "init", "-q")
	write(t, dir, "a.txt", "one\n")
	r, err := vcs.Open(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := names(changes(t, r, head(t, r), snapshot(t, r))); got != "Aa.txt" {
		t.Errorf("Diff(Head, Snapshot) = %s, want Aa.txt", got)
	}
}

// TestSnapshotRacyEdit checks that Snapshot sees a same-size edit in the
// same second as the last write of the index.
func TestSnapshotRacyEdit(t *testing.T) {
	// git checks such "racily clean" files by their content. It finds them by
	// the modification time of the index. The copy of the index must keep
	// that time.
	r, dir := newRepo(t)
	git(t, dir, "config", "core.trustctime", "false")
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-qm", "all")
	then := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	touch := func(name string) {
		t.Helper()
		if err := os.Chtimes(filepath.Join(dir, name), then, then); err != nil {
			t.Fatal(err)
		}
	}
	write(t, dir, "b.txt", "bee\n")
	touch("b.txt")
	// The index entry gets the time of b.txt.
	git(t, dir, "update-index", "--really-refresh")
	touch(".git/index")
	write(t, dir, "b.txt", "BEE\n") // the same size and time
	touch("b.txt")

	if got := names(changes(t, r, head(t, r), snapshot(t, r))); got != "Mb.txt" {
		t.Errorf("Diff(HEAD, Snapshot) = %s, want Mb.txt", got)
	}
}
