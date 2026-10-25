package test

import (
	"encoding/json"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

// realRex runs rex in the session of a test with a real Rex.
type realRex struct {
	t       *testing.T
	bin     string // the path of rex
	session string
}

// run runs rex with args and returns its output. It fails the test if rex
// fails.
func (r *realRex) run(args ...string) string {
	r.t.Helper()
	out, err := exec.CommandContext(r.t.Context(), r.bin, args...).Output()
	if err != nil {
		r.t.Fatalf("rex %s: %v", strings.Join(args, " "), err)
	}
	return string(out)
}

// blocks returns the IDs of the blocks of the session, by label.
func (r *realRex) blocks() map[string]string {
	r.t.Helper()
	var reply struct {
		Blocks []struct {
			ID    string `json:"block_id"`
			Label string `json:"label"`
		} `json:"blocks"`
	}
	out := r.run("api", "call", "-s", r.session, "session.list_blocks", "{}")
	if err := json.Unmarshal([]byte(out), &reply); err != nil {
		r.t.Fatal(err)
	}
	ids := map[string]string{}
	for _, b := range reply.Blocks {
		ids[b.Label] = b.ID
	}
	return ids
}

// write types data into block.
func (r *realRex) write(block, data string) {
	r.t.Helper()
	args, err := json.Marshal(map[string]string{"data": data})
	if err != nil {
		r.t.Fatal(err)
	}
	r.run("block", "call", "-s", r.session, "-b", block, "write", string(args))
}

// waitFor waits until the screen of block shows each of texts.
func (r *realRex) waitFor(block string, texts ...string) {
	r.t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		s := r.run("capture", "-s", r.session, "-b", block)
		ok := true
		for _, text := range texts {
			ok = ok && strings.Contains(s, text)
		}
		if ok {
			return
		}
		if time.Now().After(deadline) {
			r.t.Fatalf("block %s does not show %q. It shows:\n%s", block, texts, s)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// TestRealRex runs rexdiff in a pane of a real Rex, beside a shell that
// says it is the agent pi. It checks that rexdiff finds the agent, and
// that a send pastes the review into the shell. It runs only if
// REXDIFF_REX is the path of rex and Rex runs. It makes a session of its
// own, and does not change the config of Rex.
func TestRealRex(t *testing.T) {
	bin := os.Getenv("REXDIFF_REX")
	if bin == "" {
		t.Skip("set REXDIFF_REX to the path of rex to run rexdiff in a real Rex")
	}
	repo := newRepo(t)
	r := &realRex{t: t, bin: bin, session: "rexdiff-test-" + strconv.Itoa(os.Getpid())}
	r.run("new", r.session, "--cwd", repo, "--", "sh")
	t.Cleanup(func() {
		_ = exec.Command(bin, "kill", r.session).Run() //nolint:noctx // t.Context is done
	})
	shell := r.blocks()["shell"]
	r.run("block", "rename", shell, "agent", "-s", r.session)
	r.write(shell, `printf '\033]7501;state=idle:app=pi\007'`+"\n")

	r.run("split", "-s", r.session, "-b", shell, "--cwd", repo, "--shell=none",
		"--", binary(t), "-config", `{"focus_agent_after_send": false}`, repo)
	var pane string
	for deadline := time.Now().Add(15 * time.Second); pane == ""; {
		for label, id := range r.blocks() {
			if label != "agent" {
				pane = id
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("rexdiff does not start in a pane")
		}
	}
	r.waitFor(pane, "Changes (2)")
	r.write(pane, "]")
	r.write(pane, "c")
	r.write(pane, "hi from rexdiff")
	r.write(pane, "\r")
	r.waitFor(pane, "💬 hi from rexdiff")
	r.write(pane, "s")
	r.waitFor(pane, "sent 1 comment to agent")
	r.waitFor(shell, "a.go:4 (removed)", "hi from rexdiff")
	if got := r.blocks(); got["rexdiff"] == "" {
		t.Errorf("blocks %v, want the label rexdiff on the pane", got)
	}
}
