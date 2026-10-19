// Rexdiff shows the changes that a coding agent makes, beside the agent in
// Rex. You can comment on the changed lines, then paste the comments into the
// agent. "rexdiff install" adds a "Toggle rexdiff" action and a key to the Rex
// configuration. "rexdiff -h" shows the path of the rexdiff configuration file.
//
//	rexdiff [flags] [repo]
//	rexdiff install [--key=shift+cmd+r] [--placement=split|zoomed|tab|overlay]
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
	"github.com/rednafi/rexdiff/vcs"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "rexdiff:", err)
		stop()
		os.Exit(1)
	}
}

const usage = `usage: rexdiff [flags] [repo]
       rexdiff install [--key=shift+cmd+r] [--placement=split|zoomed|tab|overlay]
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

	repo, err := vcs.Open(ctx, cmp.Or(f.repo, "."))
	if err != nil {
		return errors.New("not a git repository")
	}
	log, closeLog, err := openLog(f.logPath)
	if err != nil {
		return err
	}
	defer closeLog()

	return tui.Run(ctx, tui.Options{
		Repo:       repo,
		Rex:        rex.FromEnv(),
		ConfigPath: tui.ConfigPath(),
		Layout:     f.layout,
		Scope:      f.scope,
		Agent:      f.agent,
		Base:       f.base,
		Wrap:       f.wrap,
		Log:        log,
	})
}

// flags are the command-line arguments of the diff viewer.
type flags struct {
	layout  tui.Layout
	scope   tui.Scope
	wrap    bool
	base    string
	agent   string
	logPath string
	repo    string // the repo argument, or ""
}

// parseFlags parses the arguments of the diff viewer. It writes the usage
// and errors to stderr. After -h, it returns flag.ErrHelp.
func parseFlags(args []string, stderr io.Writer) (flags, error) {
	var f flags
	fs := flag.NewFlagSet("rexdiff", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Func("layout", "diff layout: unified (top to bottom) or split (side by side)",
		oneOf(&f.layout, tui.LayoutUnified, tui.LayoutSplit))
	fs.Func("scope", "start scope: uncommitted, branch, or last-turn",
		oneOf(&f.scope, tui.ScopeUncommitted, tui.ScopeBranch, tui.ScopeLastTurn))
	fs.BoolVar(&f.wrap, "wrap", true, "wrap long lines; w toggles it")
	fs.StringVar(&f.base, "base", "",
		"base for the branch scope (default: the repo's default branch)")
	fs.StringVar(&f.agent, "agent", os.Getenv("REX_AGENT_BLOCK"),
		"the agent's Rex block (id or label) this pane was opened beside")
	fs.StringVar(&f.logPath, "log", os.Getenv("REXDIFF_LOG"),
		"append a debug log to this file")
	fs.Usage = func() {
		fmt.Fprint(stderr, usage)
		fs.PrintDefaults()
		fmt.Fprintf(stderr, "\nconfig: %s\n", tui.ConfigPath())
	}
	err := fs.Parse(args)
	f.repo = fs.Arg(0)
	return f, err
}

// oneOf returns a flag function that sets *p to a value from allowed.
// Other values are an error, as in the configuration file.
func oneOf[T ~string](p *T, allowed ...T) func(string) error {
	return func(v string) error {
		if !slices.Contains(allowed, T(v)) {
			return fmt.Errorf("expected one of %s", join(allowed))
		}
		*p = T(v)
		return nil
	}
}

// join returns the values, separated by commas.
func join[T ~string](values []T) string {
	var b strings.Builder
	for i, v := range values {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(string(v))
	}
	return b.String()
}

// openLog returns a logger that appends to the file at path. If path is
// empty, the logger discards everything.
func openLog(path string) (*slog.Logger, func() error, error) {
	if path == "" {
		return slog.New(slog.DiscardHandler), func() error { return nil }, nil
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, nil, err
	}
	h := slog.NewTextHandler(f, &slog.HandlerOptions{Level: slog.LevelDebug})
	return slog.New(h), f.Close, nil
}
