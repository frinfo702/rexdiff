package test

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestUpgradeAtStart checks that rexdiff in Rex replaces old code in
// rexdiff.lua, keeps the settings, and reloads Rex. Without the line above
// the code, it changes nothing and tells why.
func TestUpgradeAtStart(t *testing.T) {
	repo := newRepo(t)
	rex := newRex(t, repo)
	config := filepath.Join(t.TempDir(), "init.lua")
	env := []string{"REX_CONFIG=" + config}
	install := exec.CommandContext(t.Context(), binary(t), "install")
	install.Env = slices.Concat(os.Environ(), env, []string{"REX_BIN_DIR=" + rex.dir})
	if out, err := install.CombinedOutput(); err != nil {
		t.Fatalf("rexdiff install: %v\n%s", err, out)
	}
	lua := filepath.Join(filepath.Dir(config), "rexdiff.lua")
	installed := read(t, lua)
	edited := strings.Replace(installed, "local settings = {", "local settings = { layout = \"split\",", 1)
	write(t, lua, strings.Replace(edited, "local function must", "local function old_must", 1))

	tm := start(t, repo, opts{rex: rex, env: env})
	tm.keys("q")
	if err := tm.exited(); err != nil {
		t.Fatal(err)
	}
	if got := read(t, lua); got != edited {
		t.Errorf("rexdiff.lua after the start:\n%s\nwant the new code under the settings:\n%s", got, edited)
	}
	if calls := rex.calls(t); !strings.Contains(calls, "config reload") {
		t.Errorf("rex calls:\n%s\nwant a config reload", calls)
	}

	const mark = "-- rexdiff code below. Install and upgrades replace it. Do not edit this line.\n"
	noMark := strings.Replace(edited, mark, "", 1)
	write(t, lua, noMark)
	tm = start(t, repo, opts{rex: rex, env: env})
	tm.waitFor("cannot upgrade rexdiff.lua")
	if got := read(t, lua); got != noMark {
		t.Errorf("rexdiff.lua without the line changes to:\n%s", got)
	}
}

// read returns the content of the file at path.
func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// write writes content to the file at path.
func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
