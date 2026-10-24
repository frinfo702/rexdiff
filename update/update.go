// Package update finds a newer release of rexdiff, installs it, and starts
// it in place of the running program.
//
// [Detect] tells how rexdiff was installed. Its [Updater] finds the latest
// [Release] and installs it the same way: with Homebrew, with go install,
// or by downloading the archive of the release from GitHub and checking it
// with the checksums of the release. [Restart] runs the new binary in the
// same process, so the terminal and the process ID stay.
package update

import (
	"archive/tar"
	"bytes"
	"cmp"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// latestURL is the GitHub API address of the latest release of rexdiff.
// The variable REXDIFF_LATEST_URL replaces it, to test a release before it
// is out.
const latestURL = "https://api.github.com/repos/rednafi/rexdiff/releases/latest"

// modulePath is the path of the main package, for go install.
const modulePath = "github.com/rednafi/rexdiff/cmd/rexdiff"

// Limits of the network and of the commands.
const (
	checkTimeout   = 5 * time.Second
	installTimeout = 5 * time.Minute
	maxArchive     = 64 << 20 // the largest archive that Download takes
	maxBinary      = 128 << 20
)

// method is how rexdiff was installed, and so how an update installs it.
type method string

const (
	methodBrew     method = "brew"     // a Homebrew cask
	methodGo       method = "go"       // go install
	methodDownload method = "download" // an archive of a release
)

// Release is a release of rexdiff on GitHub. [Updater.Latest] makes one;
// a Release made by hand has no files, so Install cannot install it.
type Release struct {
	Tag    string // for example "v0.2.0"
	assets []asset
}

// asset is a file of a release.
type asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

// Updater installs releases of a rexdiff. Use [Detect] to make one.
type Updater struct {
	bin       string // the path of rexdiff, as the user runs it; it can be a link
	version   string // the version of the running rexdiff, for example "v0.1.0"
	method    method
	brew      string // the brew of the Caskroom of bin, for methodBrew
	latestURL string
	// run runs a command and returns its output. Tests change it.
	run func(ctx context.Context, env []string, name string, args ...string) ([]byte, error)
}

// Version returns the version of rexdiff, like "v0.2.0". stamped is the
// version that the release build sets, for example "0.2.0", or "". Without
// it, Version returns the version of the module that go install records,
// or "" for a build for development.
func Version(stamped string) string {
	if stamped != "" {
		return "v" + strings.TrimPrefix(stamped, "v")
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		if _, valid := numbers(info.Main.Version); valid {
			return info.Main.Version
		}
	}
	return ""
}

// Detect returns the Updater of the rexdiff at bin. stamped is as for
// [Version]. Detect returns nil if rexdiff cannot update itself, for
// example a build for development.
//
// The Updater installs a release the way rexdiff was installed: with
// Homebrew if bin links into a Caskroom, with go install if stamped is ""
// and the version comes from go install, and else by download. The
// variable REXDIFF_LATEST_URL, if set, is the address of the latest
// release, to test a release before it is out.
func Detect(bin, stamped string) *Updater {
	return detect(bin, Version(stamped), stamped == "")
}

// detect returns the Updater of the rexdiff at bin of the version. goInstall
// tells that the version comes from go install.
func detect(bin, version string, goInstall bool) *Updater {
	if _, valid := numbers(version); bin == "" || !valid {
		return nil
	}
	u := &Updater{
		bin:       bin,
		version:   version,
		method:    methodDownload,
		latestURL: cmp.Or(os.Getenv("REXDIFF_LATEST_URL"), latestURL),
	}
	target, err := filepath.EvalSymlinks(bin)
	caskroom := string(filepath.Separator) + "Caskroom" + string(filepath.Separator)
	switch {
	case err == nil && strings.Contains(target, caskroom):
		u.method = methodBrew
		// The brew that owns the cask is in the bin directory beside the
		// Caskroom, for example /opt/homebrew/bin/brew.
		prefix, _, _ := strings.Cut(target, caskroom)
		u.brew = tool("brew", filepath.Join(prefix, "bin"))
	case goInstall:
		u.method = methodGo
	}
	return u
}

// Latest returns the latest release, and whether it is newer than the
// running rexdiff. It gives up after five seconds.
func (u *Updater) Latest(ctx context.Context) (Release, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()
	body, err := get(ctx, u.latestURL, 1<<20)
	if err != nil {
		return Release{}, false, err
	}
	var wire struct {
		Tag    string  `json:"tag_name"`
		Assets []asset `json:"assets"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		return Release{}, false, fmt.Errorf("read the latest release: %w", err)
	}
	r := Release{Tag: wire.Tag, assets: wire.Assets}
	return r, newer(r.Tag, u.version), nil
}

// Install installs the release r, and checks that the binary of rexdiff is
// then the version of r. Install does not stop the running rexdiff. The
// cancel of ctx does not stop an install that runs, because a stopped brew
// can leave a broken cask; Install gives up after five minutes.
func (u *Updater) Install(ctx context.Context, r Release) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), installTimeout)
	defer cancel()
	var err error
	switch u.method {
	case methodBrew:
		// No hints and no questions: nobody sees the output.
		env := []string{"HOMEBREW_NO_ENV_HINTS=1", "NONINTERACTIVE=1"}
		brew := cmp.Or(u.brew, "brew")
		// brew upgrade knows the casks as of the last update of the taps,
		// which can be a day old, so it would not see the release. Update
		// the taps first, and only once.
		if _, err = u.command(ctx, env, brew, "update", "--quiet"); err == nil {
			env = append(env, "HOMEBREW_NO_AUTO_UPDATE=1")
			_, err = u.command(ctx, env, brew, "upgrade", "--cask", "rexdiff")
		}
	case methodGo:
		// GOBIN puts the new binary where the old one is.
		env := []string{"GOBIN=" + filepath.Dir(u.bin)}
		goTool := "go"
		if _, err := exec.LookPath(goTool); err != nil { // not in the PATH of Rex
			goTool = tool("go", "/opt/homebrew/bin", "/usr/local/go/bin", "/usr/local/bin")
		}
		_, err = u.command(ctx, env, goTool, "install", modulePath+"@"+r.Tag)
	default:
		return u.download(ctx, r) // it checks the version before it replaces
	}
	if err != nil {
		return err
	}
	return u.checkVersion(ctx, u.bin, r.Tag)
}

// checkVersion checks that the binary at bin is the version tag, so that a
// restart does not run a broken or an old binary.
func (u *Updater) checkVersion(ctx context.Context, bin, tag string) error {
	out, err := u.command(ctx, nil, bin, "-version")
	if err != nil {
		return fmt.Errorf("check the new rexdiff: %w", err)
	}
	if got := strings.TrimSpace(string(out)); got != tag {
		return fmt.Errorf("the update left rexdiff %s, not %s", cmp.Or(got, "of no version"), tag)
	}
	return nil
}

// command runs name with args, with env added to the environment. Its
// error has the end of the output, which tells what went wrong.
func (u *Updater) command(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
	run := u.run
	if run == nil {
		run = runCommand
	}
	out, err := run(ctx, env, name, args...)
	if err == nil {
		return out, nil
	}
	if last := lastLine(out); last != "" {
		return out, fmt.Errorf("%s: %w: %s", filepath.Base(name), err, last)
	}
	return out, fmt.Errorf("%s: %w", filepath.Base(name), err)
}

// tool returns the path of the program name in dirs, or else the name, for
// a look in PATH. dirs come first because Rex starts rexdiff with the PATH
// of the Rex app, which has no directory of Homebrew or of Go.
func tool(name string, dirs ...string) string {
	for _, dir := range dirs {
		path := filepath.Join(dir, name)
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() && info.Mode()&0o111 != 0 {
			return path
		}
	}
	return name
}

// runCommand is the run of an Updater outside tests.
func runCommand(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), env...)
	return cmd.CombinedOutput()
}

// lastLine returns the last line of out that is not blank.
func lastLine(out []byte) string {
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

// download downloads the archive of r for this system, checks it with the
// checksums of r, and puts its binary in place of the real file of the
// binary.
func (u *Updater) download(ctx context.Context, r Release) error {
	name := fmt.Sprintf("rexdiff-%s-%s-%s.tar.gz", r.Tag, runtime.GOOS, runtime.GOARCH)
	archiveURL, sumsURL := assetURL(r, name), assetURL(r, "sha256sums.txt")
	if archiveURL == "" || sumsURL == "" {
		return fmt.Errorf("release %s has no %s or sha256sums.txt", r.Tag, name)
	}
	sums, err := get(ctx, sumsURL, 1<<20)
	if err != nil {
		return err
	}
	want, ok := checksum(sums, name)
	if !ok {
		return fmt.Errorf("sha256sums.txt of %s has no %s", r.Tag, name)
	}
	archive, err := get(ctx, archiveURL, maxArchive)
	if err != nil {
		return err
	}
	if got := sha256.Sum256(archive); hex.EncodeToString(got[:]) != want {
		return fmt.Errorf("%s does not match its checksum", name)
	}
	bin, err := binaryOf(archive)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	target, err := filepath.EvalSymlinks(u.bin)
	if err != nil {
		return err
	}
	check := func(tmp string) error { return u.checkVersion(ctx, tmp, r.Tag) }
	return replaceFile(target, bin, check)
}

// assetURL returns the address of the asset of r with the name, or "".
func assetURL(r Release, name string) string {
	for _, a := range r.assets {
		if a.Name == name {
			return a.URL
		}
	}
	return ""
}

// checksum returns the SHA-256 of the file name in a sha256sums.txt.
func checksum(sums []byte, name string) (string, bool) {
	for line := range strings.SplitSeq(string(sums), "\n") {
		sum, file, ok := strings.Cut(strings.TrimSpace(line), "  ")
		if ok && file == name {
			return sum, true
		}
	}
	return "", false
}

// binaryOf returns the file rexdiff in a .tar.gz archive.
func binaryOf(archive []byte) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil, errors.New("the archive has no rexdiff")
		}
		if err != nil {
			return nil, err
		}
		if h.Typeflag == tar.TypeReg && filepath.Base(h.Name) == "rexdiff" {
			b, err := io.ReadAll(io.LimitReader(tr, maxBinary+1))
			if err == nil && len(b) > maxBinary {
				err = fmt.Errorf("rexdiff is larger than %d bytes", maxBinary)
			}
			return b, err
		}
	}
}

// replaceFile puts content in place of the file at path, in one rename, so
// that path is never a half-written file. check runs on the new file first;
// if it fails, path stays as it was.
func replaceFile(path string, content []byte, check func(tmp string) error) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".rexdiff-update-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no effect after the rename
	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o755); err != nil {
		return err
	}
	if err := check(tmp.Name()); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// get returns the body at url, of at most limit bytes.
func get(ctx context.Context, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json, */*")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("GET %s: more than %d bytes", url, limit)
	}
	return body, nil
}

// Restart runs the rexdiff at bin with args in place of this process. The
// process ID and the terminal stay, so Rex sees the same program. Restart
// returns only if it fails.
func Restart(bin string, args []string) error {
	return syscall.Exec(bin, append([]string{bin}, args...), os.Environ())
}

// newer reports whether the version tag is newer than the version have.
// Both are like "v1.2.3". A version that is not "v" and three numbers is
// never newer, so a build for development does not ask to update, and a tag
// that names no archive of GoReleaser is not offered.
func newer(tag, have string) bool {
	t, ok1 := numbers(tag)
	h, ok2 := numbers(have)
	if !ok1 || !ok2 {
		return false
	}
	for i := range t {
		if t[i] != h[i] {
			return t[i] > h[i]
		}
	}
	return false
}

// numbers returns the three numbers of a version like "v1.2.3".
func numbers(v string) ([3]int, bool) {
	var n [3]int
	v, ok := strings.CutPrefix(v, "v")
	if !ok {
		return n, false
	}
	parts := strings.Split(v, ".")
	if len(parts) != len(n) {
		return n, false
	}
	for i, p := range parts {
		if p == "" || strings.Trim(p, "0123456789") != "" {
			return n, false // Atoi takes a sign, a version does not
		}
		x, err := strconv.Atoi(p)
		if err != nil {
			return n, false
		}
		n[i] = x
	}
	return n, true
}
