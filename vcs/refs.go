package vcs

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Commit is a commit in the log.
type Commit struct {
	SHA, Short string
	Subject    string
	Author     string
	When       string // the relative date, for example "2 hours ago"
}

// LastCommits is the number of commits that [Repo.Commits] returns with no
// since.
const LastCommits = 50

// Commits returns the first-parent commits of HEAD after the commit since,
// the newest first, as herdr-reviewr lists them. If since is "", it returns
// the last [LastCommits] commits. A HEAD with no commit yet has no commits.
func (r *Repo) Commits(ctx context.Context, since string) ([]Commit, error) {
	// The subject is last, because it can have the separator.
	const (
		format  = "--format=%H%x1f%h%x1f%ar%x1f%an%x1f%s"
		nFields = 5 // the number of fields in format
	)
	_, err := r.run(ctx, nil, "rev-parse", "-q", "--verify", "HEAD")
	if exitedWith(err, 1) {
		return nil, nil
	}
	args := []string{"log", "--first-parent", format}
	if since == "" {
		args = append(args, "-n", strconv.Itoa(LastCommits), "--end-of-options", "HEAD")
	} else {
		args = append(args, "--end-of-options", since+"..HEAD")
	}
	out, err := r.run(ctx, nil, args...)
	if err != nil {
		return nil, err
	}
	var cs []Commit
	for l := range strings.SplitSeq(out, "\n") {
		f := strings.SplitN(l, "\x1f", nFields)
		if len(f) != nFields {
			continue
		}
		cs = append(cs, Commit{
			SHA:     f[0],
			Short:   f[1],
			When:    f[2],
			Author:  f[3],
			Subject: f[4],
		})
	}
	return cs, nil
}

// Branches returns the names of the local and the remote branches.
func (r *Repo) Branches(ctx context.Context) ([]string, error) {
	out, err := r.run(ctx, nil,
		"for-each-ref", "--format=%(refname:short)", "refs/heads", "refs/remotes")
	if err != nil {
		return nil, err
	}
	var bs []string
	// The short name of refs/remotes/origin/HEAD is "origin".
	for b := range strings.FieldsSeq(out) {
		if !strings.HasSuffix(b, "/HEAD") && b != "origin" {
			bs = append(bs, b)
		}
	}
	return bs, nil
}

// DefaultBase returns the default branch of the repository. This is the
// target of origin/HEAD, or the first that exists of main, master, trunk,
// origin/main, and origin/master. If there is none, it returns "HEAD".
func (r *Repo) DefaultBase(ctx context.Context) string {
	ref, err := r.run(ctx, nil, "symbolic-ref", "--short", "refs/remotes/origin/HEAD")
	if err == nil {
		return strings.TrimSpace(ref)
	}
	for _, b := range []string{"main", "master", "trunk", "origin/main", "origin/master"} {
		if _, err := r.Resolve(ctx, b); err == nil {
			return b
		}
	}
	return "HEAD"
}

// Branch returns the name of the branch that HEAD points to. With a
// detached HEAD, it returns "HEAD".
func (r *Repo) Branch(ctx context.Context) (string, error) {
	out, err := r.run(ctx, nil, "rev-parse", "--abbrev-ref", "HEAD")
	return strings.TrimSpace(out), err
}

// remoteURL returns the URL of the remote "upstream", so that forks work, or
// else of the remote "origin". If there is no remote, it returns "".
func (r *Repo) remoteURL(ctx context.Context) string {
	for _, name := range []string{"upstream", "origin"} {
		if u, err := r.run(ctx, nil, "remote", "get-url", name); err == nil {
			return strings.TrimSpace(u)
		}
	}
	return ""
}

// Editor returns the editor that git uses (core.editor), or "".
func (r *Repo) Editor(ctx context.Context) string {
	out, _ := r.run(ctx, nil, "config", "core.editor")
	return strings.TrimSpace(out)
}

// Entry is a path in the working tree.
type Entry struct {
	Path string
	// Ignored is true if git ignores the path. An ignored directory has one
	// entry, with a slash at the end of its path.
	Ignored bool
}

// Files returns the tracked and the untracked files, and the ignored paths,
// sorted by path.
func (r *Repo) Files(ctx context.Context) ([]Entry, error) {
	lists := []struct {
		args    []string
		ignored bool
	}{
		// In a merge conflict, git lists a path once for each stage.
		// --deduplicate lists it once.
		{[]string{"-co", "--deduplicate", "--exclude-standard"}, false},
		{[]string{"-oi", "--exclude-standard", "--directory"}, true},
	}
	var es []Entry
	for _, l := range lists {
		ps, err := r.lsFiles(ctx, l.args...)
		if err != nil {
			return nil, err
		}
		for _, p := range ps {
			es = append(es, Entry{Path: p, Ignored: l.ignored})
		}
	}
	slices.SortStableFunc(es, func(a, b Entry) int {
		return strings.Compare(a.Path, b.Path)
	})
	return es, nil
}

// Match is a line that [Repo.Grep] finds.
type Match struct {
	Path string
	Line int    // the line number, from 1
	Text string // the line, without the leading and trailing space
}

// Grep returns at most limit lines of the tracked and the untracked files
// that contain the text q. The search ignores case and binary files.
func (r *Repo) Grep(ctx context.Context, q string, limit int) ([]Match, error) {
	if strings.Contains(q, "\n") {
		// git reads each line of q as a pattern. A match is one line, so no
		// match can have the newline of q.
		return nil, nil
	}
	// --no-column overrides the git setting grep.column. That setting adds a
	// field to each match.
	out, err := r.run(ctx, nil,
		"grep", "-n", "--no-column", "-z", "-I", "-i", "-F", "--untracked", "--no-color",
		"-e", q)
	if exitedWith(err, 1) {
		return nil, nil // no match
	}
	if err != nil {
		return nil, err
	}
	// Each match is "path NUL line NUL text LF". Only the text ends with a
	// newline, because the path can have a newline.
	var ms []Match
	for len(ms) < limit && out != "" {
		var path, num, text string
		path, out, _ = strings.Cut(out, "\x00")
		num, out, _ = strings.Cut(out, "\x00")
		text, out, _ = strings.Cut(out, "\n")
		n, _ := strconv.Atoi(num)
		ms = append(ms, Match{Path: path, Line: n, Text: strings.TrimSpace(text)})
	}
	return ms, nil
}

// The refs that keep the choices of rexdiff. git keeps a ref under
// refs/worktree/ for each worktree.
const (
	baseRef = "refs/worktree/rexdiff/base-pick"
	turnRef = "refs/worktree/rexdiff/turn-base"
)

// SavedBase returns the base that [Repo.SaveBase] keeps for this worktree,
// or "" if there is none.
func (r *Repo) SavedBase(ctx context.Context) string {
	out, err := r.run(ctx, nil, "cat-file", "blob", baseRef)
	name := strings.TrimSpace(out)
	if err != nil || !refShaped(name) {
		return ""
	}
	return name
}

// refShaped reports whether s is one line of printable text that is not a
// git option, as herdr-reviewr checks a saved base.
func refShaped(s string) bool {
	return s != "" && !strings.HasPrefix(s, "-") &&
		!strings.ContainsFunc(s, func(c rune) bool { return c <= ' ' || c == 0x7f })
}

// SaveBase keeps name as the base of this worktree. For the default branch
// ([Repo.DefaultBase]), it deletes the saved base.
func (r *Repo) SaveBase(ctx context.Context, name string) error {
	if name == r.DefaultBase(ctx) {
		return r.deleteRef(ctx, baseRef)
	}
	if !refShaped(name) {
		return fmt.Errorf("bad base %q", name)
	}
	blob, err := r.runInput(ctx, nil, strings.NewReader(name),
		"hash-object", "-w", "--stdin")
	if err != nil {
		return err
	}
	_, err = r.run(ctx, nil, "update-ref", baseRef, strings.TrimSpace(blob))
	return err
}

// SavedTurn returns the tree that [Repo.SaveTurn] keeps for this worktree,
// or "" if there is none.
func (r *Repo) SavedTurn(ctx context.Context) string {
	tree, err := r.Resolve(ctx, turnRef)
	if err != nil {
		return ""
	}
	return tree
}

// SaveTurn keeps tree as the tree at the start of the last turn in this
// worktree. The ref keeps the tree from git gc. An empty tree deletes the
// saved tree.
func (r *Repo) SaveTurn(ctx context.Context, tree string) error {
	if tree == "" {
		return r.deleteRef(ctx, turnRef)
	}
	_, err := r.run(ctx, nil, "update-ref", turnRef, tree)
	return err
}

// deleteRef deletes ref. A missing ref is no error.
func (r *Repo) deleteRef(ctx context.Context, ref string) error {
	_, err := r.run(ctx, nil, "update-ref", "-d", ref)
	return err
}
