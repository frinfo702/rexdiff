package test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// build builds rexdiff with version into dir, and returns its path.
func build(t *testing.T, dir, version string) string {
	t.Helper()
	bin := filepath.Join(dir, "rexdiff")
	cmd := exec.CommandContext(t.Context(), "go", "build", "-o", bin,
		"-ldflags", "-X main.version="+version, "../cmd/rexdiff")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return bin
}

// release serves the latest release tag with the binary at bin, as GitHub
// does. It returns the address of the latest release.
func release(t *testing.T, tag, bin string) string {
	t.Helper()
	content := read(t, bin)
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	tw := tar.NewWriter(gz)
	hdr := &tar.Header{Name: "rexdiff", Mode: 0o755, Size: int64(len(content)), Typeflag: tar.TypeReg}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	name := "rexdiff-" + tag + "-" + runtime.GOOS + "-" + runtime.GOARCH + ".tar.gz"
	sum := sha256.Sum256(b.Bytes())
	files := map[string][]byte{
		"/" + name:        b.Bytes(),
		"/sha256sums.txt": []byte(hex.EncodeToString(sum[:]) + "  " + name + "\n"),
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, ok := files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(b)
	}))
	t.Cleanup(srv.Close)
	files["/latest"] = []byte(`{"tag_name": "` + tag + `", "assets": [` +
		`{"name": "` + name + `", "browser_download_url": "` + srv.URL + "/" + name + `"},` +
		`{"name": "sha256sums.txt", "browser_download_url": "` + srv.URL + `/sha256sums.txt"}]}`)
	return srv.URL + "/latest"
}

// TestUpdate checks that U installs the latest release and starts it in
// the same terminal, with the comments and the place of the user.
func TestUpdate(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	bin := build(t, dir, "0.1.0")
	latest := release(t, "v0.2.0", build(t, t.TempDir(), "0.2.0"))
	tm := startBin(t, bin, newRepo(t), opts{env: []string{"REXDIFF_LATEST_URL=" + latest}})
	tm.waitFor("rexdiff v0.2.0 is out: press U or click here to update")
	comment(tm, "hi")
	tm.keys("U")
	deadline := time.Now().Add(10 * time.Second)
	for {
		out, _ := exec.CommandContext(t.Context(), bin, "-version").Output()
		if strings.TrimSpace(string(out)) == "v0.2.0" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("rexdiff -version = %q, want v0.2.0. The screen shows:\n%s", out, tm.screen())
		}
		time.Sleep(50 * time.Millisecond)
	}
	// The new rexdiff is the latest, so it shows no update. It has the
	// comment, but not the message of the old one.
	tm.waitUntil(func(s string) bool {
		return strings.Contains(s, "💬 hi") && !strings.Contains(s, "is out") &&
			!strings.Contains(s, "updating") && !strings.Contains(s, "comment added")
	}, "show the comment in the new rexdiff")
}
