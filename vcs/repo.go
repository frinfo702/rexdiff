// Package vcs reads the changes in a git repository and gets the pull
// request of its branch.
//
// A [Repo] compares trees. A tree is the ID of a git tree object.
// [Repo.Snapshot] gets a tree of the working tree. [Repo.Diff] gets the files
// that differ between two trees.
package vcs

import (
	"bufio"
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

// emptyTree is the ID of the tree that has no files.
const emptyTree = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"

// The size limits are the same as in herdr-reviewr. [Repo.Diff] applies
// them to the sum of the two versions of a file. [Repo.ReadFile] applies
// them to one file.
const (
	maxFileSize = 2_000_000 // bytes
	maxLines    = 50_000
)

var (
	// ErrBinary means that a file is binary.
	ErrBinary = errors.New("binary file")
	// ErrTooBig means that a file is larger than 2,000,000 bytes or has
	// more than 50,000 lines.
	ErrTooBig = errors.New("file too big")
)

// Repo is a git repository.
type Repo struct {
	root  string
	index string // the path of the real index
}

// Open returns the repository that contains dir.
func Open(ctx context.Context, dir string) (*Repo, error) {
	r := &Repo{root: dir}
	// Get each path with its own git process, because a path can have a
	// newline. A path can also end with a space, so remove only the last
	// newline.
	root, err := r.run(ctx, nil, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, err
	}
	r.root = strings.TrimSuffix(root, "\n")
	index, err := r.run(ctx, nil,
		"rev-parse", "--path-format=absolute", "--git-path", "index")
	if err != nil {
		return nil, err
	}
	r.index = strings.TrimSuffix(index, "\n")
	return r, nil
}

// Root returns the top directory of the working tree.
func (r *Repo) Root() string { return r.root }

// run runs git in the root directory and returns its standard output. env
// adds to the environment of the process.
func (r *Repo) run(ctx context.Context, env []string, args ...string) (string, error) {
	return r.runInput(ctx, env, nil, args...)
}

// runInput is like run. git reads its standard input from in.
func (r *Repo) runInput(
	ctx context.Context,
	env []string,
	in io.Reader,
	args ...string,
) (string, error) {
	out, err := command(ctx, r.root, env, in, "git", args...)
	if err != nil {
		return out, fmt.Errorf("git %s: %w", args[0], err)
	}
	return out, nil
}

// command runs the program name in dir and returns its standard output.
// env adds to the environment of the process, and stdin is its standard
// input, or nil for none. The error has the first line
// of the standard error. It wraps the error of the process, so callers can
// check its exit code.
func command(
	ctx context.Context,
	dir string,
	env []string,
	stdin io.Reader,
	name string,
	args ...string,
) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir, cmd.Stdin = dir, stdin
	if env != nil {
		cmd.Env = append(os.Environ(), env...)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if errors.Is(err, exec.ErrNotFound) {
		return "", fmt.Errorf("%s isn't installed", name)
	}
	if err != nil {
		msg, _, _ := strings.Cut(strings.TrimSpace(stderr.String()), "\n")
		return string(out), &runError{cmp.Or(msg, err.Error()), err}
	}
	return string(out), nil
}

// runError shows the message of the program, but keeps the error of the
// process.
type runError struct {
	msg string
	err error
}

func (e *runError) Error() string { return e.msg }
func (e *runError) Unwrap() error { return e.err }

// exitedWith reports whether err is from a process that exited with code.
func exitedWith(err error, code int) bool {
	ee, ok := errors.AsType[*exec.ExitError](err)
	return ok && ee.ExitCode() == code
}

// Resolve returns the object ID of rev, for example "HEAD" or "main^{tree}".
func (r *Repo) Resolve(ctx context.Context, rev string) (string, error) {
	out, err := r.run(ctx, nil, "rev-parse", "--verify", "--quiet", rev)
	if err != nil {
		return "", fmt.Errorf("git: unknown revision %s", rev)
	}
	return strings.TrimSpace(out), nil
}

// Tree returns the tree of the commit rev.
func (r *Repo) Tree(ctx context.Context, rev string) (string, error) {
	return r.Resolve(ctx, rev+"^{tree}")
}

// Head returns the tree of HEAD. If HEAD is a branch with no commits yet,
// as in a new repository, it returns the empty tree.
func (r *Repo) Head(ctx context.Context) (string, error) {
	t, err := r.Tree(ctx, "HEAD")
	if err == nil {
		return t, nil
	}
	// rev-parse exits with 1 if HEAD names no commit yet. A broken HEAD,
	// for example one whose commit is missing, is an error.
	_, rerr := r.run(ctx, nil, "rev-parse", "-q", "--verify", "HEAD")
	if exitedWith(rerr, 1) {
		return emptyTree, nil
	}
	return "", err
}

// Parent returns the tree of the first parent of the commit rev. If rev has
// no parent, it returns the empty tree.
func (r *Repo) Parent(ctx context.Context, rev string) (string, error) {
	c, err := r.Resolve(ctx, rev+"^{commit}")
	if err != nil {
		return "", err
	}
	// The output has the parents, one on each line.
	out, err := r.run(ctx, nil, "rev-parse", c+"^@")
	if err != nil {
		return "", err
	}
	first, _, _ := strings.Cut(out, "\n")
	if first == "" {
		return emptyTree, nil
	}
	return r.Tree(ctx, first)
}

// MergeBase returns the tree of the best common ancestor of base and HEAD.
func (r *Repo) MergeBase(ctx context.Context, base string) (string, error) {
	c, err := r.ForkPoint(ctx, base)
	if err != nil {
		return "", err
	}
	return r.Tree(ctx, c)
}

// ForkPoint returns the best common ancestor commit of base and HEAD.
func (r *Repo) ForkPoint(ctx context.Context, base string) (string, error) {
	out, err := r.run(ctx, nil, "merge-base", "--end-of-options", base, "HEAD")
	if err != nil {
		return "", fmt.Errorf("no merge base with %s: %w", base, err)
	}
	return strings.TrimSpace(out), nil
}

// Snapshot writes the working tree as a tree object and returns its ID.
// The tree has the tracked and the untracked files. It does not have
// ignored files, or files that git cannot add, for example a git repository
// with no commit. Snapshot uses a temporary index, so the real
// index does not change.
func (r *Repo) Snapshot(ctx context.Context) (string, error) {
	// git does not read an empty index file, but it reads a missing one
	// as empty. A new repository has no index.
	dir, err := os.MkdirTemp("", "rexdiff-index-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	idx := filepath.Join(dir, "index")
	if err := r.copyIndex(idx); err != nil {
		return "", err
	}
	env := []string{"GIT_INDEX_FILE=" + idx}
	_, err = r.run(ctx, env, "add", "-A", "--ignore-errors")
	if exitedWith(err, 1) {
		err = nil // git cannot add some files, but it adds the others
	}
	if err != nil {
		return "", fmt.Errorf("snapshot: %w", err)
	}
	out, err := r.run(ctx, env, "write-tree")
	if err != nil {
		return "", fmt.Errorf("snapshot: %w", err)
	}
	return strings.TrimSpace(out), nil
}

// copyIndex copies the real index to dst. If there is no real index, it
// does nothing. With the copy, "add -A" hashes only the changed files.
// The copy keeps the modification time of the index. git uses this time
// to find files that changed in the same second as the index.
func (r *Repo) copyIndex(dst string) error {
	st, err := os.Stat(r.index)
	if errors.Is(err, fs.ErrNotExist) {
		return nil // a new repository has no index
	}
	if err != nil {
		return err
	}
	b, err := os.ReadFile(r.index)
	if err != nil {
		return err
	}
	if err := os.WriteFile(dst, b, 0o600); err != nil {
		return err
	}
	return os.Chtimes(dst, time.Time{}, st.ModTime())
}

// Status is the type of change to a file. Its value is the letter that git
// uses, for example 'A'.
type Status byte

// The values of Status that [Repo.Diff] returns.
const (
	StatusAdded    Status = 'A'
	StatusModified Status = 'M'
	StatusDeleted  Status = 'D'
	StatusRenamed  Status = 'R'

	// statusTypeChange is a change of type, for example from a file to a
	// symbolic link. [Repo.Diff] shows it as [StatusModified].
	statusTypeChange Status = 'T'
)

// File is a file that is different between two trees. Old and New are the
// two versions of its content. They are empty if the file is binary, too
// big, or not in the tree. For a submodule, each version is one line,
// "Subproject commit <id>", as git diff shows it.
type File struct {
	Path     string
	From     string // for a rename, the old path
	Status   Status
	Old, New string
	// Binary is true if a version of the file has a NUL byte.
	Binary bool
	// TooBig is true if the two versions together are larger than
	// 2,000,000 bytes or have more than 50,000 lines.
	TooBig bool
}

// gitlink is the mode of a submodule in a tree.
const gitlink = "160000"

// Diff returns the files that are different between the trees from and to,
// sorted by path.
func (r *Repo) Diff(ctx context.Context, from, to string) ([]File, error) {
	out, err := r.run(ctx, nil,
		"diff", "--raw", "-z", "--no-abbrev", "--find-renames",
		"--end-of-options", from, to)
	if err != nil {
		return nil, err
	}
	// Each file is ":<old mode> <new mode> <old id> <new id> <status>",
	// then its path. A rename has its old and new paths.
	fields := strings.Split(strings.TrimSuffix(out, "\x00"), "\x00")
	var files []File
	var ids []string // the old and the new object of each file, or "" for none
	var blobs []blob // the content of each ID
	for len(fields) >= 2 {
		meta := strings.Fields(strings.TrimPrefix(fields[0], ":"))
		if len(meta) != 5 {
			return nil, fmt.Errorf("git diff: bad output %q", fields[0])
		}
		f := File{Status: Status(meta[4][0]), Path: fields[1]}
		if f.Status == statusTypeChange {
			f.Status = StatusModified
		}
		fields = fields[2:]
		if f.Status == StatusRenamed && len(fields) > 0 {
			f.From, f.Path, fields = f.Path, fields[0], fields[1:]
		}
		files = append(files, f)
		for k := range 2 { // the old side, then the new side
			id, b := diffSide(meta[k], meta[2+k])
			ids = append(ids, id)
			blobs = append(blobs, b)
		}
	}
	if err := r.readBlobs(ctx, ids, blobs); err != nil {
		return nil, err
	}
	for i := range files {
		f := &files[i]
		f.Old, f.New = f.use(blobs[2*i]), f.use(blobs[2*i+1])
		f.TooBig = f.TooBig || tooBig(f.Old, f.New)
		if f.Binary || f.TooBig {
			f.Old, f.New = "", ""
		}
	}
	slices.SortFunc(files, func(a, b File) int { return strings.Compare(a.Path, b.Path) })
	return files, nil
}

// diffSide returns the object to read for one side of a diff, from its
// mode and its ID. It returns "" if there is no object to read. Then the
// blob has the content, if any.
func diffSide(mode, id string) (string, blob) {
	switch {
	case strings.Trim(id, "0") == "": // this side has no file
		return "", blob{}
	case mode == gitlink:
		return "", blob{data: "Subproject commit " + id + "\n"}
	}
	return id, blob{}
}

// use returns the content of b, and marks f if b is binary or too big.
func (f *File) use(b blob) string {
	f.Binary = f.Binary || errors.Is(b.err, ErrBinary)
	f.TooBig = f.TooBig || errors.Is(b.err, ErrTooBig)
	return b.data
}

// blob is the content of an object. If the object is binary or too big,
// data is empty and err is ErrBinary or ErrTooBig.
type blob struct {
	data string
	err  error
}

// readBlobs reads the objects with the given IDs into blobs[i] for each
// ids[i], with one git process. It skips an empty ID. It does not keep the
// content of an object that is larger than maxFileSize.
func (r *Repo) readBlobs(ctx context.Context, ids []string, blobs []blob) error {
	var batch []int // the indices of the objects to read
	var in strings.Builder
	for i, id := range ids {
		if id != "" {
			batch = append(batch, i)
			in.WriteString(id + "\n")
		}
	}
	if len(batch) == 0 {
		return nil
	}
	cmd := exec.CommandContext(ctx, "git", "cat-file", "--batch")
	cmd.Dir = r.root
	cmd.Stdin = strings.NewReader(in.String())
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("git cat-file: %w", err)
	}
	err = readBatch(bufio.NewReader(out), batch, blobs)
	if err != nil {
		cmd.Process.Kill()
	}
	if werr := cmd.Wait(); err == nil {
		err = werr
	}
	if err != nil {
		return fmt.Errorf("git cat-file: %w", err)
	}
	return nil
}

// readBatch reads the output of "git cat-file --batch" into blobs[i] for
// each i in batch. For each object, the output has
// "<oid> <type> <size>\n<content>\n", or one line that ends with "missing"
// or "ambiguous".
func readBatch(r *bufio.Reader, batch []int, blobs []blob) error {
	errShort := errors.New("short output")
	for _, i := range batch {
		hdr, err := r.ReadString('\n')
		if err != nil {
			return errShort
		}
		f := strings.Fields(hdr)
		if len(f) != 3 { // "<id> missing"
			continue
		}
		n, err := strconv.Atoi(f[2])
		if err != nil {
			return fmt.Errorf("bad size %q", f[2])
		}
		if f[1] != "blob" || n > maxFileSize {
			if _, err := r.Discard(n + 1); err != nil {
				return errShort
			}
			if f[1] == "blob" {
				blobs[i].err = ErrTooBig
			}
			continue
		}
		b := make([]byte, n+1)
		if _, err := io.ReadFull(r, b); err != nil {
			return errShort
		}
		if s := string(b[:n]); isBinary(s) {
			blobs[i].err = ErrBinary
		} else {
			blobs[i].data = s
		}
	}
	return nil
}

// isBinary reports whether s has a NUL byte.
func isBinary(s string) bool { return strings.Contains(s, "\x00") }

// tooBig reports whether the versions together are larger than
// maxFileSize bytes or have more than maxLines lines. A last line without
// a newline counts as a line, as in diff.Lines.
func tooBig(versions ...string) bool {
	size, lines := 0, 0
	for _, s := range versions {
		size += len(s)
		lines += strings.Count(s, "\n")
		if s != "" && !strings.HasSuffix(s, "\n") {
			lines++
		}
	}
	return size > maxFileSize || lines > maxLines
}

// ReadFile reads the file at path in the working tree. The path is relative
// to the root. ReadFile reads only a regular file in the root, also through
// a symbolic link. If the file is binary or too big, ReadFile returns
// ErrBinary or ErrTooBig.
func (r *Repo) ReadFile(path string) (string, error) {
	root, err := os.OpenRoot(r.root)
	if err != nil {
		return "", err
	}
	defer root.Close()
	st, err := root.Stat(path)
	if err != nil {
		return "", err
	}
	// Do not read a device or a named pipe. The read can block or not end.
	if !st.Mode().IsRegular() {
		return "", fmt.Errorf("%s is not a regular file", path)
	}
	if st.Size() > maxFileSize {
		return "", ErrTooBig
	}
	b, err := root.ReadFile(path)
	if err != nil {
		return "", err
	}
	switch s := string(b); {
	case isBinary(s):
		return "", ErrBinary
	case tooBig(s):
		return "", ErrTooBig
	default:
		return s, nil
	}
}

// Contains reports whether dir is the root or a directory below it. It
// follows symbolic links, so /tmp and /private/tmp on macOS are the same.
func (r *Repo) Contains(dir string) bool {
	root := evalSymlinks(r.root)
	dir = evalSymlinks(dir)
	return dir == root || strings.HasPrefix(dir, root+string(filepath.Separator))
}

// Owns reports whether dir is in the working tree of r, and not in another
// repository or worktree below the root. As in herdr-reviewr, only an
// absolute dir can be in it. Owns runs git only for a dir below the root.
// An error tells that git cannot run, so the answer is not known.
func (r *Repo) Owns(ctx context.Context, dir string) (bool, error) {
	if !filepath.IsAbs(dir) || !r.Contains(dir) {
		return false, nil
	}
	out, err := command(ctx, r.root, nil, nil, "git", "-C", dir, "rev-parse", "--show-toplevel")
	if _, ok := errors.AsType[*exec.ExitError](err); ok {
		return false, nil // dir is in no working tree
	}
	if err != nil {
		return false, err
	}
	top := strings.TrimSuffix(out, "\n")
	return top != "" && evalSymlinks(top) == evalSymlinks(r.root), nil
}

// evalSymlinks returns p with its symbolic links resolved, or p if that
// fails.
func evalSymlinks(p string) string {
	if q, err := filepath.EvalSymlinks(p); err == nil {
		return q
	}
	return p
}
