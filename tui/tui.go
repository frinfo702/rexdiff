// Package tui is the terminal program of rexdiff. It shows the changes in a
// repository and takes review comments on the lines. It sends the comments
// to a coding agent in Rex.
package tui

import (
	"context"
	"log/slog"

	tea "charm.land/bubbletea/v2"

	"github.com/rednafi/rexdiff/rex"
	"github.com/rednafi/rexdiff/vcs"
)

// appName is the name that rexdiff reports to Rex, and the label of its
// block.
const appName = "rexdiff"

// Options configure Run.
type Options struct {
	Repo *vcs.Repo // the repository to review; it must not be nil
	// Rex is the Rex session of the process, or nil outside Rex. Without
	// Rex, the review cannot be sent; it can only be copied.
	Rex *rex.Client
	// ConfigPath is the path of the configuration file. Run reads it first,
	// and again when it changes. An empty path means the default settings.
	ConfigPath string
	// Layout and Scope, if not empty, win over the configuration file. Use
	// them for the command-line flags.
	Layout Layout
	Scope  Scope
	// Agent is the block ID or the label of the agent that the program
	// opens beside, or "". The agent picker selects it first.
	Agent string
	// Wrap continues long lines on the next lines, instead of cutting
	// them. The key w changes it.
	Wrap bool
	// Base is the base of the branch scope. An empty value means the
	// default branch of the repository.
	Base string
	Log  *slog.Logger // nil means no log
}

// Run shows the review program in the terminal. It returns when the user
// quits or when ctx is done. If the configuration file is not valid, Run
// returns the error before it starts.
func Run(ctx context.Context, o Options) error {
	cfg, err := loadConfig(o.ConfigPath)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	m := newModel(ctx, o, cfg)
	m.reload()
	if o.Rex != nil && o.Rex.Block != "" {
		if err := o.Rex.SetLabel(ctx, o.Rex.Block, appName); err != nil {
			m.log.Debug("set label", "err", err)
		}
	}
	program := tea.NewProgram(m, tea.WithContext(ctx))
	if o.Rex != nil {
		go o.Rex.Watch(ctx, func(e rex.Event) { program.Send(eventMsg(e)) })
	}
	_, err = program.Run()
	return err
}
