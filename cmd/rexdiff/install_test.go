package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeRexDir returns a directory with a rex that writes its arguments to
// log and exits 0. Set REX_BIN_DIR to it, so that no test reloads the real
// Rex.
func fakeRexDir(t *testing.T, log string) string {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\necho \"$@\" >> " + log + "\n"
	if err := os.WriteFile(filepath.Join(dir, "rex"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestInstallWithoutRex checks that install stops before it writes a file
// when rex is not installed.
func TestInstallWithoutRex(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("REX_CONFIG", "")
	t.Setenv("REX_BIN_DIR", "")
	t.Setenv("PATH", t.TempDir())
	err := install(t.Context(), "/opt/rexdiff", nil, io.Discard, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "rex is not installed") {
		t.Errorf("install without rex = %v, want the rex error", err)
	}
	if _, err := os.Stat(filepath.Dir(luaPath())); !os.IsNotExist(err) {
		t.Errorf("install without rex made %s", filepath.Dir(luaPath()))
	}
}

// TestUpgrade checks that upgrade replaces the code of rexdiff.lua if it
// differs from the code of the binary, and keeps the settings.
func TestUpgrade(t *testing.T) {
	dir := t.TempDir()
	reloads := filepath.Join(dir, "reloads")
	t.Setenv("REX_CONFIG", filepath.Join(dir, "init.lua"))
	t.Setenv("REX_BIN_DIR", fakeRexDir(t, reloads))
	lua := filepath.Join(dir, luaFile)
	const bin = "/opt/rexdiff"
	const settings = "local settings = { layout = \"split\" }\n\n"
	write := func(content string) {
		t.Helper()
		if err := os.WriteFile(lua, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	read := func() string {
		t.Helper()
		b, err := os.ReadFile(lua)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	ctx := t.Context()

	// Without rexdiff.lua, upgrade does nothing.
	if n := upgrade(ctx, bin); n != "" {
		t.Fatalf("upgrade without rexdiff.lua = %q, want no notice", n)
	}
	if _, err := os.Stat(lua); err == nil {
		t.Fatal("upgrade made a rexdiff.lua")
	}

	// If the file has old code for the same binary, upgrade replaces the
	// code, keeps the settings, and reloads Rex.
	old := strings.Replace(luaCode(bin), "local function must", "local function old_must", 1)
	write(settings + old)
	if n := upgrade(ctx, bin); n != "" {
		t.Fatalf("upgrade = %q, want no notice", n)
	}
	if got := read(); got != settings+luaCode(bin) {
		t.Errorf("rexdiff.lua after upgrade:\n%s\nwant the settings and the new code", got)
	}
	if b, _ := os.ReadFile(reloads); string(b) != "config reload\n" {
		t.Errorf("rex calls = %q, want one config reload", b)
	}

	// If the code is the same, upgrade does not write the file or reload Rex.
	if n := upgrade(ctx, bin); n != "" {
		t.Fatalf("second upgrade = %q, want no notice", n)
	}
	if b, _ := os.ReadFile(reloads); string(b) != "config reload\n" {
		t.Errorf("rex calls = %q, want no second reload", b)
	}

	// If the file runs another rexdiff, for example a build for
	// development, upgrade does not change it.
	other := settings + old
	other = strings.Replace(other, luaQuote(bin), luaQuote("/tmp/dev/rexdiff"), 1)
	write(other)
	if n := upgrade(ctx, bin); n != "" || read() != other {
		t.Errorf("upgrade of the file of another rexdiff = %q; want no notice, no change", n)
	}

	// Without the line above the code, upgrade changes nothing. Its notice
	// tells what is wrong and what to do.
	noMark := strings.Replace(settings+old, codeMark+"\n", "", 1)
	if !strings.Contains(noMark, luaQuote(bin)) {
		t.Fatal("the test file does not run bin")
	}
	write(noMark)
	n := upgrade(ctx, bin)
	for _, want := range []string{"cannot upgrade rexdiff.lua", lua, codeMark,
		"cannot tell your settings from its code", "Put that line back"} {
		if !strings.Contains(n, want) {
			t.Errorf("notice %q does not have %q", n, want)
		}
	}
	if read() != noMark {
		t.Error("upgrade changed rexdiff.lua without the line above the code")
	}
}

// TestLuaQuote checks that a path that Go would escape stays the same in
// Lua 5.1, which knows only a few escapes.
func TestLuaQuote(t *testing.T) {
	got := luaQuote("/a\u200bb/\"x\"\\y\nz")
	want := "\"/a\u200bb/\\\"x\\\"\\\\y\\nz\""
	if got != want {
		t.Errorf("luaQuote = %s, want %s", got, want)
	}
}
