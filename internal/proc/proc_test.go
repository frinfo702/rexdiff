package proc_test

import (
	"errors"
	"os/exec"
	"testing"

	"github.com/rednafi/rexdiff/internal/proc"
)

// TestError checks the text of the error of a run, and that the error
// keeps the error of the process.
func TestError(t *testing.T) {
	exit := exec.CommandContext(t.Context(), "sh", "-c", "exit 3").Run()
	tests := []struct {
		name   string
		err    error
		stderr string
		want   string
	}{
		{"/usr/bin/gh", exec.ErrNotFound, "", "gh is not installed"},
		{"git", exit, "\n  fatal: bad\nmore\n", "fatal: bad"},
		{"rex", exit, "Error: no Rex server\n", "no Rex server"},
		{"git", exit, " \n", exit.Error()},
	}
	for _, tt := range tests {
		err := proc.Error(tt.name, tt.err, []byte(tt.stderr))
		if err.Error() != tt.want {
			t.Errorf("Error(%q, %v, %q) = %q, want %q", tt.name, tt.err, tt.stderr, err, tt.want)
		}
		if !errors.Is(err, tt.err) {
			t.Errorf("Error(%q, %v, %q) does not wrap %v", tt.name, tt.err, tt.stderr, tt.err)
		}
	}
}

// TestLastLine checks that LastLine skips the blank lines at the end.
func TestLastLine(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", ""},
		{"one", "one"},
		{"one\n two \n\n", "two"},
		{"one\r\ntwo\r\n", "two"},
	}
	for _, tt := range tests {
		if got := proc.LastLine(tt.in); got != tt.want {
			t.Errorf("LastLine(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// TestCommand checks that Command limits the wait for the output.
func TestCommand(t *testing.T) {
	cmd := proc.Command(t.Context(), "true")
	if cmd.WaitDelay <= 0 {
		t.Errorf("WaitDelay = %v, want more than 0", cmd.WaitDelay)
	}
}
