package rex_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/rednafi/rexdiff/rex"
)

// fakeRex writes a shell script that answers like the rex tool, with
// recorded output. It logs each call to the file "calls" in its directory.
func fakeRex(t *testing.T) (*rex.Client, string) {
	t.Helper()
	dir := t.TempDir()
	script := `#!/bin/sh
printf "%s\n" "$*" >> "` + dir + `/calls"
case "$*" in
"api call -s S session.list_blocks {}")
  echo '{"blocks":[
    {"block_id":"block:self","label":"rexdiff","window_id":"w1",
     "creator_name":"com.superlogical.terminal"},
    {"block_id":"block:pi","label":"agent","window_id":"w1",
     "creator_name":"com.superlogical.terminal"},
    {"block_id":"block:web","label":"","window_id":"w1",
     "creator_name":"com.superlogical.browser"},
    {"block_id":"block:sh","label":"shell","window_id":"w2",
     "creator_name":"com.superlogical.terminal"},
    {"block_id":"block:cc","label":"claude-code","window_id":"w2",
     "creator_name":"com.superlogical.terminal"}]}' ;;
"session inspect S --json")
  echo '{"windows":[
    {"window_id":"w1","label":"main"},
    {"window_id":"w2","label":"side"}]}' ;;
"block call -s S -b block:pi program_status")
  echo '{"records":[
    {"id":"sub","state":"idle","app":"sub"},
    {"state":"blocked","kind":"permission","app":"pi","effective_app":"pi"}]}' ;;
"block call -s S -b block:cc program_status")
  echo '{"records":[{"state":"working","app":"claude-code"}]}' ;;
"block call -s S -b block:sh program_status")
  echo '{"records":[]}' ;;
"block call -s S -b "*" process")
  echo '{"foreground":{"cwd":"file:///repo"}}' ;;
"block inspect agent -s S --json")
  echo '{"block":{"block_id":"block:pi"}}' ;;
"block inspect nosuch -s S --json")
  echo 'Error: no block matches nosuch
more detail' >&2; exit 1 ;;
"events -s S --json")
  cat "` + dir + `/events" ;;
*) ;;
esac
`
	bin := filepath.Join(dir, "rex")
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	// The client comes from the environment that Rex gives a block.
	t.Setenv("REX_SESSION", "S")
	t.Setenv("REX_BLOCK", "block:self")
	t.Setenv("REX_BIN_DIR", dir)
	return rex.FromEnv(), dir
}

// TestAgents checks that Agents returns only the terminal blocks that send
// an app name, without the block of the client.
func TestAgents(t *testing.T) {
	c, _ := fakeRex(t)
	agents, err := c.Agents(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	want := []rex.Agent{
		{
			Block:  "block:pi",
			Name:   "agent",
			Window: "main",
			Cwd:    "/repo",
			Status: rex.Status{App: "pi", State: rex.StateBlocked},
		},
		{
			Block:  "block:cc",
			Name:   "claude-code",
			Window: "side",
			Cwd:    "/repo",
			Status: rex.Status{App: "claude-code", State: rex.StateWorking},
		},
	}
	if !slices.Equal(agents, want) {
		t.Fatalf("Agents got:\n%+v\nwant:\n%+v", agents, want)
	}
}

// TestCommands checks the rex calls of Resolve, Paste, Focus, and Split.
func TestCommands(t *testing.T) {
	c, dir := fakeRex(t)
	ctx := t.Context()
	if id, err := c.Resolve(ctx, "agent"); err != nil || id != "block:pi" {
		t.Errorf("Resolve(agent) = %q, %v, want block:pi, nil", id, err)
	}
	if id, _ := c.Resolve(ctx, "block:x"); id != "block:x" {
		t.Errorf("Resolve(block:x) = %q, want block:x", id)
	}
	_, err := c.Resolve(ctx, "nosuch")
	if err == nil || err.Error() != "no block matches nosuch" {
		t.Errorf("Resolve(nosuch): err = %v, want %q",
			err, "no block matches nosuch")
	}
	if err := c.Paste(ctx, "block:pi", "a\n\tb"); err != nil {
		t.Fatal(err)
	}
	if err := c.Focus(ctx, "block:pi"); err != nil {
		t.Fatal(err)
	}
	if err := c.Split(ctx, "/repo", "sh", "-c", "vi x"); err != nil {
		t.Fatal(err)
	}

	b, _ := os.ReadFile(filepath.Join(dir, "calls"))
	calls := string(b)
	for _, want := range []string{
		`api call -s S block.event {"block_id":"block:pi",` +
			`"event":{"type":"paste","payload":{"text":"a\n\tb"}}}`,
		`api call -s S session.focus_block {"block_id":"block:pi"}`,
		`split -s S -b block:self --cwd /repo -- sh -c vi x`,
	} {
		if !strings.Contains(calls, want) {
			t.Errorf("calls got:\n%s\nwant a call %s", calls, want)
		}
	}
}

// blockEvent returns a line of "rex events --json" for an event of block
// with the name and the payload.
func blockEvent(block, name, payload string) string {
	return `{"type":"session_event","payload":{"session_id":"S",` +
		`"event":{"type":"block_event","payload":{"block_id":"` + block +
		`","name":"` + name + `","payload":` + payload + `}}}}`
}

// TestWatch checks that Watch reports only the changes of the status of
// the program, and closed blocks.
func TestWatch(t *testing.T) {
	c, dir := fakeRex(t)
	events := strings.Join([]string{
		blockEvent("block:pi", "program_status_changed", `{"id":"",`+
			`"record":{"state":"working","kind":"","app":"x","effective_app":"pi"},`+
			`"reason":"report"}`),
		blockEvent("block:pi", "program_status_changed",
			`{"id":"sub","record":{"state":"idle"}}`),
		`not json`,
		// The removal of a sub record is not a change of the agent.
		blockEvent("block:pi", "program_status_removed",
			`{"ids":["sub"],"reason":"clear"}`),
		blockEvent("block:pi", "program_status_removed",
			`{"ids":[""],"reason":"clear"}`),
		`{"type":"session_event","payload":{"event":{"type":"block_closed",` +
			`"payload":{"block_id":"block:pi"}}}}`,
	}, "\n")
	path := filepath.Join(dir, "events")
	if err := os.WriteFile(path, []byte(events+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	want := []rex.Event{
		{Block: "block:pi", Status: rex.Status{App: "pi", State: rex.StateWorking}},
		{Block: "block:pi"},
		{Block: "block:pi", Closed: true},
	}
	// Stop Watch when it reports all the events.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	var got []rex.Event
	err := c.Watch(ctx, func(e rex.Event) {
		got = append(got, e)
		if len(got) == len(want) {
			cancel()
		}
	})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Watch = %v, want context.Canceled", err)
	}
	if !slices.Equal(got, want) {
		t.Errorf("events got:\n%+v\nwant:\n%+v", got, want)
	}
}

// TestWatchReconnect checks that Watch reports a reconnect after it starts
// rex again.
func TestWatchReconnect(t *testing.T) {
	c, dir := fakeRex(t)
	line := `{"type":"session_event","payload":{"event":{"type":"block_closed",` +
		`"payload":{"block_id":"block:pi"}}}}`
	path := filepath.Join(dir, "events")
	if err := os.WriteFile(path, []byte(line+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	closed := rex.Event{Block: "block:pi", Closed: true}
	want := []rex.Event{closed, {Reconnect: true}, closed}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	var got []rex.Event
	err := c.Watch(ctx, func(e rex.Event) {
		if got = append(got, e); len(got) == len(want) {
			cancel()
		}
	})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Watch = %v, want context.Canceled", err)
	}
	if !slices.Equal(got, want) {
		t.Errorf("events got:\n%+v\nwant:\n%+v", got, want)
	}
}

// TestFromEnv checks that FromEnv uses the environment that Rex sets.
func TestFromEnv(t *testing.T) {
	t.Setenv("REX_SESSION", "")
	if c := rex.FromEnv(); c != nil {
		t.Errorf("FromEnv outside Rex = %+v, want nil", c)
	}
	// fakeRex sets the environment of a block. The client runs the rex
	// command of REX_BIN_DIR, in the session REX_SESSION.
	c, dir := fakeRex(t)
	if c == nil || c.Block != "block:self" {
		t.Fatalf("FromEnv = %+v, want the block block:self", c)
	}
	if err := c.Focus(t.Context(), "block:pi"); err != nil {
		t.Fatal(err)
	}
	calls, err := os.ReadFile(filepath.Join(dir, "calls"))
	if err != nil || !strings.Contains(string(calls), "api call -s S ") {
		t.Errorf("calls %q, want a call to the fake rex in session S", calls)
	}
}

// TestReloadConfigOutsideRex checks that ReloadConfig runs the rex of the
// Rex server app outside a Rex terminal.
func TestReloadConfigOutsideRex(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("the Rex server app is on macOS")
	}
	// Outside a Rex terminal, for example under brew, REX_BIN_DIR is not set
	// and rex is not in PATH.
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("REX_BIN_DIR", "")
	t.Setenv("PATH", t.TempDir())
	dir := filepath.Join(home, "Library", "Application Support", "com.superlogical.rex", "RexServer", "Versions", "Current", "Rex Server.app", "Contents", "MacOS")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\necho \"reloaded: $*\"\n"
	if err := os.WriteFile(filepath.Join(dir, "rex"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	out, err := rex.ReloadConfig(t.Context())
	if err != nil || out != "reloaded: config reload\n" {
		t.Errorf("ReloadConfig = %q, %v, want the report of the app rex", out, err)
	}
}

// TestErrorKeepsCause checks that the error of a failed rex is the message
// of rex. The error also wraps the exit error of the process.
func TestErrorKeepsCause(t *testing.T) {
	dir := t.TempDir()
	script := "#!/bin/sh\necho 'Error: no Rex server' >&2\nexit 3\n"
	if err := os.WriteFile(filepath.Join(dir, "rex"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("REX_BIN_DIR", dir)
	_, err := rex.ReloadConfig(t.Context())
	ee, ok := errors.AsType[*exec.ExitError](err)
	if err == nil || err.Error() != "no Rex server" || !ok || ee.ExitCode() != 3 {
		t.Errorf("ReloadConfig error = %v, exit error %v; want %q wrapping exit 3", err, ok, "no Rex server")
	}
}
