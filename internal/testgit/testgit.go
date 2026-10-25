// Package testgit has the test helpers that run git. The settings, hooks,
// and templates of the user do not change what git does in a test.
package testgit

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// set is true after Main sets the environment.
var set bool

// Main runs the tests of m in a git environment that ignores the setup of
// the user. Each package that runs git calls it from its TestMain.
//
// Git reads no global or system config, so no hooksPath, signing,
// fsmonitor, or diff setting applies. A new repository gets no hooks from
// a template. The variables that point git to another repository, or that
// add config, are unset. The commits have a fixed author.
func Main(m *testing.M) {
	for _, kv := range Env() {
		k, v, _ := strings.Cut(kv, "=")
		if err := os.Setenv(k, v); err != nil {
			panic(err)
		}
	}
	for _, k := range unset {
		if err := os.Unsetenv(k); err != nil {
			panic(err)
		}
	}
	set = true
	m.Run()
}

// Env returns the variables that Main sets, as "key=value" pairs. A test
// that runs a program with an environment of its own adds them. An empty
// GIT_TEMPLATE_DIR copies no template.
func Env() []string {
	return []string{
		"GIT_CONFIG_GLOBAL=" + os.DevNull,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_TEMPLATE_DIR=",
		"GIT_AUTHOR_NAME=t",
		"GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t",
		"GIT_COMMITTER_EMAIL=t@t",
	}
}

// unset are the variables that make git use another repository, more
// config, or another diff program.
var unset = []string{
	"GIT_DIR",
	"GIT_WORK_TREE",
	"GIT_INDEX_FILE",
	"GIT_OBJECT_DIRECTORY",
	"GIT_ALTERNATE_OBJECT_DIRECTORIES",
	"GIT_COMMON_DIR",
	"GIT_NAMESPACE",
	"GIT_CONFIG",
	"GIT_CONFIG_COUNT",
	"GIT_CONFIG_PARAMETERS",
	"GIT_EXTERNAL_DIFF",
	"GIT_DIFF_OPTS",
}

// ready fails tb if Main does not run.
func ready(tb testing.TB) {
	tb.Helper()
	if !set {
		tb.Fatal("testgit: TestMain does not call testgit.Main")
	}
}

// Git runs git in dir and returns its standard output. It fails tb if
// git fails.
func Git(tb testing.TB, dir string, args ...string) string {
	tb.Helper()
	ready(tb)
	cmd := exec.CommandContext(tb.Context(), "git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		var stderr []byte
		if ee, ok := errors.AsType[*exec.ExitError](err); ok {
			stderr = ee.Stderr
		}
		tb.Fatalf("git %v: %v: %s", args, err, stderr)
	}
	return string(out)
}

// NewRepo makes a repository in dir with the branch main. It makes dir
// if it does not exist.
func NewRepo(tb testing.TB, dir string) {
	tb.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		tb.Fatal(err)
	}
	Git(tb, dir, "init", "-q", "-b", "main")
}

// WriteFile writes content to the file name in dir. It makes the parent
// directories.
func WriteFile(tb testing.TB, dir, name, content string) {
	tb.Helper()
	p := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		tb.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		tb.Fatal(err)
	}
}

// Script writes an executable sh script with body to the file name in
// dir, and returns its path. A test puts dir on PATH to fake a tool.
func Script(tb testing.TB, dir, name, body string) string {
	tb.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		tb.Fatal(err)
	}
	return p
}
