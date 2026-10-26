// Package clipboard writes text to the system clipboard through the first
// clipboard tool on PATH, as herdr-reviewr does.
package clipboard

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// ErrNoTool tells that PATH has no clipboard tool. The user can fix it.
var ErrNoTool = errors.New("no clipboard tool is installed (pbcopy, wl-clipboard, xclip, or xsel)")

// tools are the clipboard tools that read stdin. The first on PATH wins.
var tools = [][]string{
	{"pbcopy"},
	{"wl-copy"},
	{"xclip", "-selection", "clipboard"},
	{"xsel", "--clipboard", "--input"},
}

// Write pipes text into the first clipboard tool on PATH. It fails if the
// tool exits with a status that is not zero.
func Write(ctx context.Context, text string) error {
	for _, t := range tools {
		path, err := exec.LookPath(t[0])
		if err != nil {
			continue
		}
		cmd := exec.CommandContext(ctx, path, t[1:]...)
		cmd.Stdin = strings.NewReader(text)
		// The output goes to the null device. xclip and wl-copy leave a
		// process that keeps the clipboard, and a pipe stays open while it
		// runs.
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("%s: %w", t[0], err)
		}
		return nil
	}
	return ErrNoTool
}
