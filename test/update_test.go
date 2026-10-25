package test

import (
	"archive/tar"
	"bytes"
	"cmp"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"testing"
)

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
	// The two builds run at once.
	var next string
	var nextErr error
	var wg sync.WaitGroup
	nextDir := t.TempDir()
	wg.Go(func() { next, nextErr = build(nextDir, "0.2.0") })
	bin, err := build(t.TempDir(), "0.1.0")
	wg.Wait()
	if err = cmp.Or(err, nextErr); err != nil {
		t.Fatal(err)
	}
	latest := release(t, "v0.2.0", next)
	tm := startBin(t, bin, newRepo(t), opts{env: []string{"REXDIFF_LATEST_URL=" + latest}})
	tm.waitFor("rexdiff v0.2.0 is out: press U or click here to update")
	comment(tm, "hi")
	tm.keys("U")
	var version string
	eventually(t, func() bool {
		out, _ := exec.CommandContext(t.Context(), bin, "-version").Output()
		version = strings.TrimSpace(string(out))
		return version == "v0.2.0"
	}, func() string { return "rexdiff -version is " + version + ", want v0.2.0" })
	// The new rexdiff is the latest, so it shows no update. It has the
	// comment, but not the message of the old one.
	tm.waitUntil(func(s string) bool {
		return strings.Contains(s, "💬 hi") && !strings.Contains(s, "is out") &&
			!strings.Contains(s, "updating") && !strings.Contains(s, "comment added")
	}, "show the comment in the new rexdiff")
}
