package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// TestNewer checks which tags are newer than the running version.
func TestNewer(t *testing.T) {
	tests := []struct {
		tag, have string
		want      bool
	}{
		{"v0.2.0", "v0.1.0", true},
		{"v0.1.1", "v0.1.0", true},
		{"v1.0.0", "v0.9.9", true},
		{"v0.10.0", "v0.9.0", true},
		{"0.2.0", "v0.1.0", false}, // GoReleaser names archives with the v
		{"v0.1.0", "v0.1.0", false},
		{"v0.1.0", "v0.2.0", false},
		{"v0.2.0-rc1", "v0.1.0", false},
		{"v0.2.0", "v0.0.0-20261024-abcdef", false},
		{"", "v0.1.0", false},
		{"v0.2.0", "", false},
		{"v+1.0.0", "v0.9.0", false}, // Atoi accepts a sign
		{"v1.-0.0", "v0.9.0", false},
	}
	for _, tc := range tests {
		if got := newer(tc.tag, tc.have); got != tc.want {
			t.Errorf("newer(%q, %q) = %v, want %v", tc.tag, tc.have, got, tc.want)
		}
	}
}

// file writes content to a new file at path with mode 0755, and returns
// path.
func file(t *testing.T, path, content string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestDetect checks that detect finds a Homebrew cask by the Caskroom of
// its link, and go install by its flag.
func TestDetect(t *testing.T) {
	dir := t.TempDir()
	cask := file(t, filepath.Join(dir, "Caskroom", "rexdiff", "0.1.0", "rexdiff"), "")
	link := filepath.Join(dir, "bin", "rexdiff")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(cask, link); err != nil {
		t.Fatal(err)
	}
	plain := file(t, filepath.Join(dir, "plain", "rexdiff"), "")
	tests := []struct {
		bin, version string
		goInstall    bool
		want         method
	}{
		{link, "v0.1.0", false, methodBrew},
		{link, "v0.1.0", true, methodBrew},
		{plain, "v0.1.0", true, methodGo},
		{plain, "v0.1.0", false, methodDownload},
	}
	for _, tc := range tests {
		u := detect(tc.bin, tc.version, tc.goInstall)
		if u == nil || u.method != tc.want {
			t.Errorf("detect(%s, %s, %v) = %+v, want %s", tc.bin, tc.version, tc.goInstall, u, tc.want)
		}
	}
	for _, v := range []string{"", "(devel)", "v0.0.0-20261024-abcdef"} {
		if u := detect(plain, v, true); u != nil {
			t.Errorf("detect of version %q = %+v, want nil", v, u)
		}
	}
	if u := detect("", "v0.1.0", false); u != nil {
		t.Errorf("detect without a path = %+v, want nil", u)
	}
}

// TestLatest checks Latest against a fake GitHub API.
func TestLatest(t *testing.T) {
	answer := func(code int, body string) *Updater {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(code)
			fmt.Fprint(w, body)
		}))
		t.Cleanup(srv.Close)
		return &Updater{version: "v0.1.0", latestURL: srv.URL}
	}
	r, isNewer, err := answer(200, `{"tag_name": "v0.2.0", "assets": [{"name": "a", "browser_download_url": "u"}], "other": 1}`).Latest(t.Context())
	if err != nil || !isNewer || r.Tag != "v0.2.0" || len(r.assets) != 1 || r.assets[0].URL != "u" {
		t.Errorf("Latest = %+v, %v, %v; want v0.2.0 with one asset, newer", r, isNewer, err)
	}
	if _, isNewer, err := answer(200, `{"tag_name": "v0.1.0"}`).Latest(t.Context()); err != nil || isNewer {
		t.Errorf("Latest of the same version: newer %v, %v; want false, nil", isNewer, err)
	}
	if _, _, err := answer(403, `rate limit`).Latest(t.Context()); err == nil || !strings.Contains(err.Error(), "403") {
		t.Errorf("Latest after 403: %v, want an error with the status", err)
	}
	if _, _, err := answer(200, `not json`).Latest(t.Context()); err == nil {
		t.Error("Latest of bad JSON: no error, want an error")
	}
}

// fakeRun records the commands, and answers -version with version.
type fakeRun struct {
	calls   []string
	version string
	fail    string // a command name that fails
}

func (f *fakeRun) run(_ context.Context, env []string, name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, strings.TrimSpace(strings.Join(env, " ")+" "+name+" "+strings.Join(args, " ")))
	if name == f.fail {
		return []byte("Updating Homebrew...\nError: another brew is running\n"), errors.New("exit status 1")
	}
	if len(args) == 1 && args[0] == "-version" {
		return []byte(f.version + "\n"), nil
	}
	return nil, nil
}

// TestInstallCommands checks the commands of Homebrew and go install, and
// the check of the new version.
func TestInstallCommands(t *testing.T) {
	r := Release{Tag: "v0.2.0"}
	brew := &fakeRun{version: "v0.2.0"}
	u := &Updater{bin: "/opt/homebrew/bin/rexdiff", version: "v0.1.0", method: methodBrew, run: brew.run}
	if err := u.Install(t.Context(), r); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"HOMEBREW_NO_ENV_HINTS=1 NONINTERACTIVE=1 brew update --quiet",
		"HOMEBREW_NO_ENV_HINTS=1 NONINTERACTIVE=1 HOMEBREW_NO_AUTO_UPDATE=1 brew upgrade --cask rexdiff",
		"/opt/homebrew/bin/rexdiff -version",
	}
	if !slices.Equal(brew.calls, want) {
		t.Errorf("brew calls got:\n%s\nwant:\n%s", strings.Join(brew.calls, "\n"), strings.Join(want, "\n"))
	}

	gorun := &fakeRun{version: "v0.2.0"}
	u = &Updater{bin: "/home/u/go/bin/rexdiff", version: "v0.1.0", method: methodGo, run: gorun.run}
	if err := u.Install(t.Context(), r); err != nil {
		t.Fatal(err)
	}
	env, rest, _ := strings.Cut(gorun.calls[0], " ")
	prog, args, _ := strings.Cut(rest, " ")
	if env != "GOBIN=/home/u/go/bin" || filepath.Base(prog) != "go" || args != "install github.com/rednafi/rexdiff/cmd/rexdiff@v0.2.0" {
		t.Errorf("go call = %q, want GOBIN=/home/u/go/bin go install github.com/rednafi/rexdiff/cmd/rexdiff@v0.2.0", gorun.calls[0])
	}

	// The error of a failed command has the last line of the output.
	failing := &fakeRun{fail: "brew"}
	u = &Updater{bin: "/b/rexdiff", method: methodBrew, run: failing.run}
	err := u.Install(t.Context(), r)
	if err == nil || err.Error() != "brew: exit status 1: Error: another brew is running" {
		t.Errorf("Install with a failing brew: %v, want %q",
			err, "brew: exit status 1: Error: another brew is running")
	}

	// A new binary of another version is an error, so rexdiff does not
	// restart into it.
	stale := &fakeRun{version: "v0.1.0"}
	u = &Updater{bin: "/b/rexdiff", method: methodBrew, run: stale.run}
	if err := u.Install(t.Context(), r); err == nil || !strings.Contains(err.Error(), "left rexdiff v0.1.0, not v0.2.0") {
		t.Errorf("Install that leaves the old version: %v, want an error",
			err)
	}
}

// archive returns a .tar.gz with a README.md and a file rexdiff that has
// content.
func archive(t *testing.T, content string) []byte {
	t.Helper()
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	tw := tar.NewWriter(gz)
	add := func(name, content string, mode int64) {
		hdr := &tar.Header{Name: name, Mode: mode, Size: int64(len(content)), Typeflag: tar.TypeReg}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	add("README.md", "hi", 0o644)
	add("rexdiff", content, 0o755)
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// TestDownload checks the download of a release: the checksum, the binary
// in the archive, and the replacement of the target file of a link.
func TestDownload(t *testing.T) {
	name := fmt.Sprintf("rexdiff-v0.2.0-%s-%s.tar.gz", runtime.GOOS, runtime.GOARCH)
	good := archive(t, "new binary")
	sum := sha256.Sum256(good)
	files := map[string][]byte{
		"/" + name:            good,
		"/sha256sums.txt":     []byte(hex.EncodeToString(sum[:]) + "  " + name + "\nabc  other.tar.gz\n"),
		"/bad/" + name:        []byte("tampered"),
		"/bad/sha256sums.txt": []byte(hex.EncodeToString(sum[:]) + "  " + name + "\n"),
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		b, ok := files[req.URL.Path]
		if !ok {
			http.NotFound(w, req)
			return
		}
		w.Write(b)
	}))
	t.Cleanup(srv.Close)
	release := func(prefix string) Release {
		return Release{Tag: "v0.2.0", assets: []asset{
			{Name: name, URL: srv.URL + prefix + "/" + name},
			{Name: "sha256sums.txt", URL: srv.URL + prefix + "/sha256sums.txt"},
		}}
	}

	dir := t.TempDir()
	target := file(t, filepath.Join(dir, "real", "rexdiff"), "old binary")
	link := filepath.Join(dir, "rexdiff")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	fake := &fakeRun{version: "v0.2.0"}
	u := &Updater{bin: link, version: "v0.1.0", method: methodDownload, run: fake.run}
	if err := u.Install(t.Context(), release("")); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(target); string(b) != "new binary" {
		t.Errorf("target file = %q, want the new binary", b)
	}
	if info, _ := os.Lstat(link); info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("after Install: %s is not a symlink, want a symlink", link)
	}
	if info, _ := os.Stat(target); info.Mode().Perm() != 0o755 {
		t.Errorf("mode = %v, want 0755", info.Mode().Perm())
	}
	if entries, _ := os.ReadDir(filepath.Dir(target)); len(entries) != 1 {
		t.Errorf("the directory has %d files, want only rexdiff", len(entries))
	}

	// A file that does not match its checksum is not installed.
	file(t, target, "old binary")
	if err := u.Install(t.Context(), release("/bad")); err == nil || !strings.Contains(err.Error(), "does not match its checksum") {
		t.Errorf("Install of a tampered archive: %v, want a checksum error",
			err)
	}
	if b, _ := os.ReadFile(target); string(b) != "old binary" {
		t.Errorf("target file after a bad download = %q, want the old binary", b)
	}

	// download does not replace the old binary with a binary of another
	// version, so the next start does not run it.
	stale := &fakeRun{version: "v0.1.9"}
	u.run = stale.run
	if err := u.Install(t.Context(), release("")); err == nil || !strings.Contains(err.Error(), "left rexdiff v0.1.9, not v0.2.0") {
		t.Errorf("Install of a binary of another version: %v, "+
			"want a version error", err)
	}
	if b, _ := os.ReadFile(target); string(b) != "old binary" {
		t.Errorf("target file after a failed check = %q, want the old binary", b)
	}
	if entries, _ := os.ReadDir(filepath.Dir(target)); len(entries) != 1 {
		t.Errorf("after a failed check: %d files, want only rexdiff",
			len(entries))
	}
	u.run = fake.run

	// A release without the archive of this system is an error.
	if err := u.Install(t.Context(), Release{Tag: "v0.2.0"}); err == nil || !strings.Contains(err.Error(), "has no "+name) {
		t.Errorf("Install of a release without assets: %v, want an error",
			err)
	}
}

// TestInstallOutlivesCancel checks that the cancel of the context of the
// program does not stop a brew that runs. A stopped brew can leave a broken
// cask.
func TestInstallOutlivesCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	var sawCancel bool
	run := func(ctx context.Context, _ []string, name string, _ ...string) ([]byte, error) {
		if name == "brew" {
			cancel() // the user quits rexdiff while brew runs
			sawCancel = ctx.Err() != nil
			return nil, nil
		}
		return []byte("v0.2.0\n"), nil
	}
	u := &Updater{bin: "/b/rexdiff", method: methodBrew, run: run}
	if err := u.Install(ctx, Release{Tag: "v0.2.0"}); err != nil || sawCancel {
		t.Errorf("Install after a cancel = %v, brew sees the cancel %v; "+
			"want nil, false", err, sawCancel)
	}
}

// TestBinaryTooLarge checks that a binary larger than the limit is an
// error, not a binary that is cut short.
func TestBinaryTooLarge(t *testing.T) {
	big := strings.Repeat("x", maxBinary+1)
	if _, err := binaryOf(archive(t, big)); err == nil || !strings.Contains(err.Error(), "larger than") {
		t.Errorf("binaryOf of a binary over the limit: %v, want an error",
			err)
	}
	if b, err := binaryOf(archive(t, "ok")); err != nil || string(b) != "ok" {
		t.Errorf("binaryOf = %q, %v, want \"ok\", nil", b, err)
	}
}

// TestVersion checks the version from the release build. A test binary has
// no version, like a build for development.
func TestVersion(t *testing.T) {
	for stamped, want := range map[string]string{"0.2.0": "v0.2.0", "v0.2.0": "v0.2.0", "": ""} {
		if got := Version(stamped); got != want {
			t.Errorf("Version(%q) = %q, want %q", stamped, got, want)
		}
	}
	if u := Detect("/bin/rexdiff", ""); u != nil {
		t.Errorf("Detect of a build for development = %+v, want nil", u)
	}
	t.Setenv("REXDIFF_LATEST_URL", "http://127.0.0.1:1/latest")
	if u := Detect("/bin/rexdiff", "0.2.0"); u == nil || u.latestURL != "http://127.0.0.1:1/latest" || u.method != methodDownload {
		t.Errorf("Detect of a release build = %+v, "+
			"want a download from the test URL", u)
	}
}

// TestDetectBrew checks that a Homebrew install uses the brew beside its
// Caskroom. Rex starts rexdiff with a PATH without Homebrew.
func TestDetectBrew(t *testing.T) {
	prefix := t.TempDir()
	cask := file(t, filepath.Join(prefix, "Caskroom", "rexdiff", "0.2.0", "rexdiff"), "")
	brew := file(t, filepath.Join(prefix, "bin", "brew"), "#!/bin/sh\n")
	brew, _ = filepath.EvalSymlinks(brew) // detect follows /var on macOS
	link := filepath.Join(prefix, "bin", "rexdiff")
	if err := os.Symlink(cask, link); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", "/usr/bin:/bin") // the PATH of the Rex app
	u := detect(link, "v0.2.0", false)
	if u == nil || u.method != methodBrew || u.brew != brew {
		t.Fatalf("detect = %+v, want brew %s", u, brew)
	}
	fake := &fakeRun{version: "v0.2.1"}
	u.run = fake.run
	if err := u.Install(t.Context(), Release{Tag: "v0.2.1"}); err != nil {
		t.Fatal(err)
	}
	if want := "HOMEBREW_NO_ENV_HINTS=1 NONINTERACTIVE=1 " + brew + " update --quiet"; fake.calls[0] != want {
		t.Errorf("first brew call = %q, want %q", fake.calls[0], want)
	}
}

// TestErrorWithoutOutput checks that a command that fails with no output
// has no empty part at the end of its error.
func TestErrorWithoutOutput(t *testing.T) {
	u := &Updater{run: func(context.Context, []string, string, ...string) ([]byte, error) {
		return nil, errors.New(`exec: "brew": executable file not found in $PATH`)
	}}
	_, err := u.command(t.Context(), nil, "/x/brew", "upgrade")
	if want := `brew: exec: "brew": executable file not found in $PATH`; err == nil || err.Error() != want {
		t.Errorf("error = %v, want %s", err, want)
	}
}
