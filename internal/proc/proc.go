// Package proc runs programs and makes their errors short and clear.
package proc

import (
	"cmp"
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// waitDelay is how long a stopped program can keep its output open. A
// child of the program can keep the output open after the program ends.
const waitDelay = time.Second

// Command returns the command "name args...". The command stops when ctx
// is done. Its Wait returns at most waitDelay after the program ends.
func Command(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.WaitDelay = waitDelay
	return cmd
}

// NotInstalled returns the error of the program name that PATH does not
// have. The text has the base name of the program.
func NotInstalled(name string) error {
	return errors.New(filepath.Base(name) + " is not installed")
}

// Error returns the error err of a run of the program name. If the
// program is not on PATH, the text is "<name> is not installed".
// Else the text is the first line of stderr that is not blank, without a
// leading "Error: ". If stderr has no text, the text is that of err. The
// error wraps err, so a caller can examine the exit code.
func Error(name string, err error, stderr []byte) error {
	if errors.Is(err, exec.ErrNotFound) {
		return &runError{msg: NotInstalled(name).Error(), err: err}
	}
	msg := strings.TrimPrefix(firstLine(string(stderr)), "Error: ")
	return &runError{msg: cmp.Or(msg, err.Error()), err: err}
}

// runError is a failed run of a program. It has a short text and wraps
// the error of the process.
type runError struct {
	msg string
	err error
}

func (e *runError) Error() string { return e.msg }
func (e *runError) Unwrap() error { return e.err }

// firstLine returns the first line of s that is not blank, without the
// leading and trailing space.
func firstLine(s string) string {
	for line := range strings.Lines(s) {
		if line = strings.TrimSpace(line); line != "" {
			return line
		}
	}
	return ""
}

// LastLine returns the last line of s that is not blank, without the
// leading and trailing space.
func LastLine(s string) string {
	s = strings.TrimSpace(s)
	return strings.TrimSpace(s[strings.LastIndexByte(s, '\n')+1:])
}
