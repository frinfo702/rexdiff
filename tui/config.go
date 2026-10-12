package tui

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/alecthomas/chroma/v2/styles"
)

// Layout is the layout of the diff.
type Layout string

const (
	LayoutUnified Layout = "unified" // the old and the new lines in one column
	LayoutSplit   Layout = "split"   // the old lines beside the new lines
)

// Scope tells what the Changes tab compares with the working tree.
type Scope string

const (
	ScopeUncommitted Scope = "uncommitted" // HEAD
	ScopeBranch      Scope = "branch"      // the base branch
	ScopeLastTurn    Scope = "last-turn"   // the start of the last turn of the agent
)

// markdownView tells how the program shows a Markdown file.
type markdownView string

const (
	markdownViewSource   markdownView = "source"
	markdownViewRendered markdownView = "rendered"
)

// position is a side of the screen.
type position string

const (
	positionRight position = "right"
	positionLeft  position = "left"
)

// The values that the configuration accepts, in the order of the help.
var (
	layouts       = []Layout{LayoutUnified, LayoutSplit}
	configScopes  = []Scope{ScopeUncommitted, ScopeBranch, ScopeLastTurn}
	markdownViews = []markdownView{markdownViewSource, markdownViewRendered}
	positions     = []position{positionRight, positionLeft}
)

// config is the content of the configuration file.
type config struct {
	Layout       Layout `toml:"layout"`
	DefaultScope Scope  `toml:"default_scope"`
	// ReviewPrefix is the text above each review that rexdiff sends or
	// copies.
	ReviewPrefix string `toml:"review_prefix"`
	// Editor is a command, for example "code -g {file}:{line}".
	Editor string `toml:"editor"`
	// URLOpener is a command, for example "open {url}".
	URLOpener string `toml:"url_opener"`
	// Theme is "terminal" or the name of a chroma style, for example
	// "monokai". loadConfig checks the name.
	Theme        string       `toml:"theme"`
	MarkdownView markdownView `toml:"markdown_view"`
	// FileList is the side of the screen that shows the list of files.
	FileList position `toml:"file_list"`
	// FileListWidth is in columns, minListWidth or more.
	FileListWidth int `toml:"file_list_width"`
	// FocusAgent tells if the focus moves to the agent after a send. Nil
	// means true.
	FocusAgent *bool `toml:"focus_agent_after_send"`
	// GitHubHost, GitLabHost, and AzureHost are bare hostnames of
	// self-hosted forges.
	GitHubHost  string      `toml:"github_host"`
	GitLabHost  string      `toml:"gitlab_host"`
	AzureHost   string      `toml:"azure_devops_host"`
	Keybindings keybindings `toml:"keybindings"`
}

// keybindings sets the keys of each action. The TOML keys are the names of
// the actions, as in herdr-reviewr. A nil list keeps the default keys.
type keybindings struct {
	// Move.
	Down     []string `toml:"down"`
	Up       []string `toml:"up"`
	PageDown []string `toml:"page-down"`
	PageUp   []string `toml:"page-up"`
	HalfDown []string `toml:"half-down"`
	HalfUp   []string `toml:"half-up"`
	Collapse []string `toml:"collapse"`
	Expand   []string `toml:"expand"`
	NextHunk []string `toml:"next-hunk"`
	PrevHunk []string `toml:"prev-hunk"`
	NextFile []string `toml:"next-file"`
	PrevFile []string `toml:"prev-file"`
	Focus    []string `toml:"focus"`

	// Scopes and tabs.
	ScopeUncommitted []string `toml:"scope-uncommitted"`
	ScopeBranch      []string `toml:"scope-branch"`
	ScopeLastTurn    []string `toml:"scope-last-turn"`
	ScopeCommits     []string `toml:"scope-commits"`
	BasePick         []string `toml:"base-pick"`
	CommitPick       []string `toml:"commit-pick"`
	TabChanges       []string `toml:"tab-changes"`
	TabAllFiles      []string `toml:"tab-all-files"`
	TabPR            []string `toml:"tab-pr"`

	// Comments.
	Select      []string `toml:"select"`
	Comment     []string `toml:"comment"`
	Edit        []string `toml:"edit"`
	Delete      []string `toml:"delete"`
	NextComment []string `toml:"next-comment"`
	PrevComment []string `toml:"prev-comment"`
	Comments    []string `toml:"comments"`
	Send        []string `toml:"send"`
	Copy        []string `toml:"copy"`

	// View and other actions.
	Search            []string `toml:"search"`
	Find              []string `toml:"find"`
	GotoLine          []string `toml:"goto-line"`
	Rendered          []string `toml:"rendered"`
	Split             []string `toml:"split"`
	Wrap              []string `toml:"wrap"`
	NavigatorPosition []string `toml:"navigator-position"`
	NavigatorHide     []string `toml:"navigator-hide"`
	NavigatorGrow     []string `toml:"navigator-grow"`
	NavigatorShrink   []string `toml:"navigator-shrink"`
	OpenPR            []string `toml:"open-pr"`
	Refresh           []string `toml:"refresh"`
	Keys              []string `toml:"keys"`
	Quit              []string `toml:"quit"`
	QuitDiscard       []string `toml:"quit-discard"`
}

// minListWidth is the smallest width of the file list, in columns.
const minListWidth = 16

// defaultConfig returns the configuration that applies when there is no
// configuration file.
func defaultConfig() config {
	return config{
		Layout:        LayoutUnified,
		DefaultScope:  ScopeUncommitted,
		Theme:         themeTerminal,
		MarkdownView:  markdownViewSource,
		FileList:      positionRight,
		FileListWidth: 32,
	}
}

// ConfigPath returns the path of the configuration file:
// $REXDIFF_CONFIG, else $XDG_CONFIG_HOME/rexdiff/config.toml, else
// ~/.config/rexdiff/config.toml.
func ConfigPath() string {
	if p := os.Getenv("REXDIFF_CONFIG"); p != "" {
		return p
	}
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "rexdiff", "config.toml")
}

// loadConfig reads the file at path over the default configuration. If
// the file does not exist, loadConfig returns the default configuration.
// As in herdr-reviewr, an unknown key or a bad value is an error.
func loadConfig(path string) (config, error) {
	c := defaultConfig()
	if path == "" {
		return c, nil
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return config{}, fmt.Errorf("config: %w", err)
	}
	md, err := toml.Decode(string(b), &c)
	if err != nil {
		return config{}, fmt.Errorf("config %s: %w", path, err)
	}
	if keys := md.Undecoded(); len(keys) > 0 {
		return config{}, fmt.Errorf("config %s: unknown key %q", path, keys[0])
	}
	if err := c.check(md); err != nil {
		return config{}, fmt.Errorf("config %s: %w", path, err)
	}
	return c, nil
}

// check returns an error for the first value that rexdiff does not accept.
// md tells which keys the file sets.
func (c config) check(md toml.MetaData) error {
	if err := c.checkChoices(); err != nil {
		return err
	}
	if err := c.checkTexts(md); err != nil {
		return err
	}
	if err := c.checkHosts(md); err != nil {
		return err
	}
	return c.checkKeybindings()
}

// checkChoices checks the values that have fixed choices or a range. They
// are the layout, the scope, the theme, and the file list.
func (c config) checkChoices() error {
	choices := []struct {
		key, value string
		allowed    []string
	}{
		{"layout", string(c.Layout), names(layouts...)},
		{"default_scope", string(c.DefaultScope), names(configScopes...)},
		{"markdown_view", string(c.MarkdownView), names(markdownViews...)},
		{"file_list", string(c.FileList), names(positions...)},
	}
	for _, k := range choices {
		if !slices.Contains(k.allowed, k.value) {
			return invalid(k.key, "one of "+strings.Join(k.allowed, ", "), k.value)
		}
	}
	if c.Theme != themeTerminal && styles.Registry[strings.ToLower(c.Theme)] == nil {
		return invalid("theme", "terminal or a chroma style name", c.Theme)
	}
	if c.FileListWidth < minListWidth {
		want := strconv.Itoa(minListWidth) + " or more"
		return invalid("file_list_width", want, strconv.Itoa(c.FileListWidth))
	}
	return nil
}

// checkTexts checks the commands and the review prefix. A command can use
// only its placeholders. A text that the file sets must not be blank.
func (c config) checkTexts(md toml.MetaData) error {
	commands := []struct {
		key, value, want string
		known            []string
	}{
		{
			key:   "editor",
			value: c.Editor,
			want:  "a command with only {file} and {line}",
			known: []string{"file", "line"},
		},
		{
			key:   "url_opener",
			value: c.URLOpener,
			want:  "a command with only {url}",
			known: []string{"url"},
		},
	}
	for _, k := range commands {
		blank := md.IsDefined(k.key) && strings.TrimSpace(k.value) == ""
		if blank || !placeholders(k.value, k.known...) {
			return invalid(k.key, k.want, k.value)
		}
	}
	if md.IsDefined("review_prefix") && strings.TrimSpace(c.ReviewPrefix) == "" {
		return invalid("review_prefix", "text", c.ReviewPrefix)
	}
	return nil
}

// checkHosts checks the hostnames of the self-hosted forges.
func (c config) checkHosts(md toml.MetaData) error {
	hosts := []struct{ key, value string }{
		{"github_host", c.GitHubHost},
		{"gitlab_host", c.GitLabHost},
		{"azure_devops_host", c.AzureHost},
	}
	for _, k := range hosts {
		if md.IsDefined(k.key) && !hostname(k.value) {
			return invalid(k.key, "a bare hostname", k.value)
		}
	}
	return nil
}

// checkKeybindings checks the keys of the actions. The decoder rejects an
// unknown action. A list that is set must have keys.
func (c config) checkKeybindings() error {
	for _, b := range new(keyMap).bindings(c.Keybindings) {
		if b.keys != nil && (len(b.keys) == 0 || slices.Contains(b.keys, "")) {
			return invalid("keybindings."+b.name, "a list of keys", fmt.Sprint(b.keys))
		}
	}
	return nil
}

// invalid returns the error for a bad value of key.
func invalid(key, want, value string) error {
	return fmt.Errorf("invalid value for %s: expected %s, not %q", key, want, value)
}

// placeholders reports whether each { in command opens one of the known
// placeholders, for example {file}.
func placeholders(command string, known ...string) bool {
	for _, s := range strings.Split(command, "{")[1:] {
		name, _, ok := strings.Cut(s, "}")
		if !ok || !slices.Contains(known, name) {
			return false
		}
	}
	return true
}

// The longest DNS name and the longest label of a DNS name (RFC 1035).
const (
	maxHostname  = 253
	maxHostLabel = 63
)

// hostname reports whether s is a bare DNS name, for example
// "git.example.com", with no scheme, port, or path.
func hostname(s string) bool {
	if len(s) > maxHostname {
		return false
	}
	for label := range strings.SplitSeq(s, ".") {
		if !hostLabel(label) {
			return false
		}
	}
	return true
}

// hostLabel reports whether s is one valid label of a DNS name.
func hostLabel(s string) bool {
	if s == "" || len(s) > maxHostLabel || s[0] == '-' || s[len(s)-1] == '-' {
		return false
	}
	return !strings.ContainsFunc(s, func(r rune) bool {
		letter := 'a' <= r && r <= 'z' || 'A' <= r && r <= 'Z'
		return !letter && !('0' <= r && r <= '9') && r != '-'
	})
}

// modTime returns the modification time of the file at path, or the zero
// time if there is no file.
func modTime(path string) time.Time {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}

// themeTerminal is the theme that uses the colours of the terminal.
const themeTerminal = "terminal"

// syntax returns the chroma style of the theme, or "" for the terminal
// palette.
func (c config) syntax() string {
	if c.Theme == themeTerminal {
		return ""
	}
	return c.Theme
}

// focusAgent tells if the focus moves to the agent after a send.
func (c config) focusAgent() bool { return c.FocusAgent == nil || *c.FocusAgent }

// names returns the values as strings.
func names[T ~string](values ...T) []string {
	s := make([]string, len(values))
	for i, v := range values {
		s[i] = string(v)
	}
	return s
}
