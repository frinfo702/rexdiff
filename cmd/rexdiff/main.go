// Rexdiff shows the changes that a coding agent makes, beside the agent in
// Rex. You can comment on the changed lines, then paste the comments into the
// agent. "rexdiff install" adds a "Toggle rexdiff" action and a key to the Rex
// configuration, in rexdiff.lua beside Rex's init.lua. The settings of rexdiff
// are at the top of rexdiff.lua. The action gives them to rexdiff in the
// --config flag.
//
//	rexdiff [flags] [repo]
//	rexdiff install
//	rexdiff uninstall
package main

import (
	"cmp"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"slices"
	"strings"

	"github.com/rednafi/rexdiff/rex"
	"github.com/rednafi/rexdiff/tui"
	"github.com/rednafi/rexdiff/update"
	"github.com/rednafi/rexdiff/vcs"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	err := run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, "rexdiff:", err)
		os.Exit(1)
	}
}

const usage = `usage: rexdiff [flags] [repo]
       rexdiff install
       rexdiff uninstall

flags:
`

// run runs the subcommand in args, or else the diff viewer.
func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) > 0 {
		switch args[0] {
		case "install":
			bin, err := executable()
			if err != nil {
				return err
			}
			return install(ctx, bin, args[1:], stdout, stderr)
		case "uninstall":
			return uninstall(ctx, stdout)
		}
	}
	f, err := parseFlags(args, stderr)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return err
	}
	if f.version {
		fmt.Fprintln(stdout, cmp.Or(update.Version(version), "(devel)"))
		return nil
	}
	if f.check {
		return tui.CheckConfig(f.config)
	}

	dir := cmp.Or(f.repo, ".")
	repo, err := vcs.Open(ctx, dir)
	if err != nil {
		return fmt.Errorf("open %s: %w", dir, err)
	}
	log, closeLog, err := openLog(f.logPath)
	if err != nil {
		return err
	}
	defer closeLog()

	// In Rex, upgrade replaces the code of the action if that code differs
	// from the code of this rexdiff.
	client := rex.FromEnv()
	var notice string
	bin, binErr := executable()
	if binErr == nil && client != nil {
		notice = upgrade(ctx, bin)
	}
	var updater tui.Updater
	if u := update.Detect(bin, version); u != nil {
		updater = u // an interface with a nil *update.Updater is not nil
	}
	resume, err := tui.Run(ctx, tui.Options{
		Repo:    repo,
		Rex:     client,
		Notice:  notice,
		Updater: updater,
		Resume:  resumeFile(),
		Config:  f.config,
		Layout:  f.layout,
		Scope:   f.scope,
		Agent:   f.agent,
		Base:    f.base,
		Wrap:    f.wrap,
		Log:     log,
	})
	if err != nil || resume == "" {
		return err
	}
	// After an update, run the new rexdiff in this process with the same
	// flags. resumeEnv gives it the saved state.
	closeLog()
	if err = os.Setenv(resumeEnv, resume); err == nil {
		err = update.Restart(bin, args) // returns only if it fails
	}
	// Run keeps the alternate screen for the restart. Leave that screen and
	// show the cursor.
	fmt.Fprint(stdout, "\x1b[?1049l\x1b[?25h")
	return fmt.Errorf("start the new rexdiff: %w; your comments are saved in %s", err, resume)
}

// resumeEnv is the variable that gives a restarted rexdiff the path of the
// file with the state from before the update. rexdiff uses a variable, not a
// flag, so that the arguments of the user do not change.
const resumeEnv = "REXDIFF_RESUME"

// resumeFile returns the file in resumeEnv, or "". It unsets the variable,
// so that the programs that rexdiff starts, such as an editor, do not get
// the variable.
func resumeFile() string {
	path := os.Getenv(resumeEnv)
	// Unsetenv fails only for a bad name. resumeEnv is a good name.
	_ = os.Unsetenv(resumeEnv)
	return path
}

// version is the version of rexdiff that GoReleaser sets, for example
// "0.2.0". It is "" in other builds.
var version string

// flags are the command-line arguments of the diff viewer.
type flags struct {
	layout  tui.Layout
	scope   tui.Scope
	wrap    bool
	base    string
	agent   string
	logPath string
	config  string // the settings as JSON, or ""
	check   bool   // check the settings and exit
	version bool   // print the version and exit
	repo    string // the repo argument, or ""
}

// parseFlags parses the arguments of the diff viewer. It writes the usage
// and errors to stderr. For -h, it returns flag.ErrHelp.
func parseFlags(args []string, stderr io.Writer) (flags, error) {
	var f flags
	fs := flag.NewFlagSet("rexdiff", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Func("layout", "diff layout: unified (top to bottom) or split (side by side)",
		oneOf(&f.layout, tui.LayoutUnified, tui.LayoutSplit))
	fs.Func("scope", "start scope: uncommitted, branch, or last-turn",
		oneOf(&f.scope, tui.ScopeUncommitted, tui.ScopeBranch, tui.ScopeLastTurn))
	fs.BoolVar(&f.wrap, "wrap", true, "wrap long lines; w toggles it")
	fs.StringVar(&f.base, "base", "", "base for the branch scope (default: the picked base, else the repo's default branch)")
	fs.StringVar(&f.agent, "agent", os.Getenv("REX_AGENT_BLOCK"), "the agent's Rex block (id or label) this pane was opened beside")
	fs.StringVar(&f.logPath, "log", os.Getenv("REXDIFF_LOG"), "append a debug log to this file")
	fs.StringVar(&f.config, "config", "", "the settings as JSON; the Rex action passes them from rexdiff.lua")
	fs.BoolVar(&f.check, "check", false, "check the settings of -config and exit")
	fs.BoolVar(&f.version, "version", false, "print the version and exit")
	fs.Usage = func() {
		fmt.Fprint(stderr, usage)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return f, err
	}
	if fs.NArg() > 1 {
		return f, fmt.Errorf("unknown argument %s (flags go before the repo)", fs.Arg(1))
	}
	f.repo = fs.Arg(0)
	return f, nil
}

// oneOf returns a flag function that sets *p to a value from allowed.
// Other values are an error, as in the settings.
func oneOf[T ~string](p *T, allowed ...T) func(string) error {
	return func(v string) error {
		if !slices.Contains(allowed, T(v)) {
			var want []string
			for _, a := range allowed {
				want = append(want, string(a))
			}
			return fmt.Errorf("expected one of %s", strings.Join(want, ", "))
		}
		*p = T(v)
		return nil
	}
}

// openLog returns a logger that appends to the file at path, and a function
// that closes the file. If path is empty, the logger discards everything.
// The log is only for debugging, so the close ignores errors.
func openLog(path string) (*slog.Logger, func(), error) {
	if path == "" {
		return slog.New(slog.DiscardHandler), func() {}, nil
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, nil, err
	}
	h := slog.NewTextHandler(f, &slog.HandlerOptions{Level: slog.LevelDebug})
	return slog.New(h), func() { _ = f.Close() }, nil
}
