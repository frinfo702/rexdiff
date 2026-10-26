package tui

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
)

// Layout is the layout of the diff.
type Layout string

// The layouts.
const (
	LayoutUnified Layout = "unified" // the old and the new lines in one column
	LayoutSplit   Layout = "split"   // the old lines beside the new lines
)

// Scope tells what the Changes tab compares with the working tree.
type Scope string

// The scopes, and what they compare with the working tree.
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

// The values that the settings accept, in the order of the error messages.
var (
	layouts       = []Layout{LayoutUnified, LayoutSplit}
	configScopes  = []Scope{ScopeUncommitted, ScopeBranch, ScopeLastTurn}
	markdownViews = []markdownView{markdownViewSource, markdownViewRendered}
	positions     = []position{positionRight, positionLeft}
)

// config holds the settings of rexdiff. The settings are at the top of
// rexdiff.lua, beside the init.lua of Rex. The toggle action gives them to
// rexdiff as JSON in --config.
type config struct {
	Layout       Layout `json:"layout"`
	DefaultScope Scope  `json:"default_scope"`
	// ReviewPrefix is the text above each review that rexdiff sends or
	// copies.
	ReviewPrefix string `json:"review_prefix"`
	// Editor is a command, for example "code -g {file}:{line}".
	Editor string `json:"editor"`
	// URLOpener is a command, for example "open {url}".
	URLOpener    string       `json:"url_opener"`
	MarkdownView markdownView `json:"markdown_view"`
	// FileList is the side of the screen that shows the list of files.
	FileList position `json:"file_list"`
	// FileListWidth is in columns, minListWidth or more.
	FileListWidth int `json:"file_list_width"`
	// GuideAgent is the command that writes the Guide tab, for example
	// "claude -p". It reads the prompt on its standard input and prints
	// the answer. "" picks the first of claude, pi, and codex that is
	// installed.
	GuideAgent string `json:"guide_agent"`
	// DiffSigns marks a changed line with + or -, in place of a bar.
	DiffSigns bool `json:"diff_signs"`
	// FocusAgent tells if the focus moves to the agent after a send. Nil
	// means true.
	FocusAgent *bool `json:"focus_agent_after_send"`
	// GitHubHost, GitLabHost, and AzureHost are bare hostnames of
	// self-hosted forges.
	GitHubHost  string      `json:"github_host"`
	GitLabHost  string      `json:"gitlab_host"`
	AzureHost   string      `json:"azure_devops_host"`
	Keybindings keybindings `json:"keybindings"`
}

// keybindings sets the keys of each action. The JSON keys are the names of
// the actions. A nil list keeps the default keys.
type keybindings struct {
	// Move.
	Down     []string `json:"down"`
	Up       []string `json:"up"`
	PageDown []string `json:"page-down"`
	PageUp   []string `json:"page-up"`
	HalfDown []string `json:"half-down"`
	HalfUp   []string `json:"half-up"`
	Collapse []string `json:"collapse"`
	Expand   []string `json:"expand"`
	NextHunk []string `json:"next-hunk"`
	PrevHunk []string `json:"prev-hunk"`
	NextFile []string `json:"next-file"`
	PrevFile []string `json:"prev-file"`
	Focus    []string `json:"focus"`

	// Scopes and tabs.
	ScopeUncommitted []string `json:"scope-uncommitted"`
	ScopeBranch      []string `json:"scope-branch"`
	ScopeLastTurn    []string `json:"scope-last-turn"`
	ScopeCommits     []string `json:"scope-commits"`
	BasePick         []string `json:"base-pick"`
	CommitPick       []string `json:"commit-pick"`
	TabOverview      []string `json:"tab-overview"`
	TabGuide         []string `json:"tab-guide"`
	TabChanges       []string `json:"tab-changes"`
	TabAllFiles      []string `json:"tab-all-files"`
	TabPR            []string `json:"tab-pr"`

	// Comments.
	Select      []string `json:"select"`
	Comment     []string `json:"comment"`
	Edit        []string `json:"edit"`
	Delete      []string `json:"delete"`
	NextComment []string `json:"next-comment"`
	PrevComment []string `json:"prev-comment"`
	Comments    []string `json:"comments"`
	Send        []string `json:"send"`
	Copy        []string `json:"copy"`

	// View and other actions.
	Search            []string `json:"search"`
	Find              []string `json:"find"`
	GotoLine          []string `json:"goto-line"`
	Rendered          []string `json:"rendered"`
	Split             []string `json:"split"`
	Wrap              []string `json:"wrap"`
	NavigatorPosition []string `json:"navigator-position"`
	NavigatorHide     []string `json:"navigator-hide"`
	NavigatorGrow     []string `json:"navigator-grow"`
	NavigatorShrink   []string `json:"navigator-shrink"`
	OpenPR            []string `json:"open-pr"`
	Refresh           []string `json:"refresh"`
	Keys              []string `json:"keys"`
	Quit              []string `json:"quit"`
	QuitDiscard       []string `json:"quit-discard"`
}

// minListWidth is the smallest width of the file list, in columns.
const minListWidth = 16

// defaultConfig returns the value of each setting that --config does not
// set.
func defaultConfig() config {
	return config{
		Layout:        LayoutSplit,
		DefaultScope:  ScopeUncommitted,
		MarkdownView:  markdownViewSource,
		FileList:      positionRight,
		FileListWidth: 32,
	}
}

// parseConfig reads the JSON text over the default settings. An empty
// text gives the default settings. An unknown key, a key in the wrong
// case, or a bad value gives an error.
func parseConfig(text string) (config, error) {
	c := defaultConfig()
	if text == "" {
		return c, nil
	}
	// set tells which keys the text has. A blank text that the user sets
	// is an error. A text that the user does not set is not an error.
	var set map[string]jsontext.Value
	if err := json.Unmarshal([]byte(text), &set); err != nil {
		return config{}, configError(err)
	}
	if err := json.Unmarshal([]byte(text), &c, json.RejectUnknownMembers(true)); err != nil {
		return config{}, configError(err)
	}
	if err := c.check(set); err != nil {
		return config{}, fmt.Errorf("config: %w", err)
	}
	return c, nil
}

// configError changes an error of the JSON decoder into an error that
// uses the names of the settings, for example "keybindings.send".
func configError(err error) error {
	se, ok := errors.AsType[*json.SemanticError](err)
	if !ok {
		return fmt.Errorf("config: %w", err)
	}
	key := strings.ReplaceAll(strings.TrimPrefix(string(se.JSONPointer), "/"), "/", ".")
	if errors.Is(err, json.ErrUnknownName) {
		return fmt.Errorf("config: unknown key %q", key)
	}
	if key == "" || se.GoType == nil {
		return errors.New("config: expected a table of settings")
	}
	want := "a table"
	switch se.GoType.Kind() {
	case reflect.String:
		want = "text"
	case reflect.Int:
		want = "a whole number"
	case reflect.Bool:
		want = "true or false"
	case reflect.Slice:
		want = "a list of keys" // an empty Lua table encodes as a JSON object
	}
	return fmt.Errorf("config: invalid value for %s: expected %s, not %s",
		key, want, jsonKinds[se.JSONKind])
}

// jsonKinds names the kinds of JSON values in the words of Lua.
var jsonKinds = map[jsontext.Kind]string{
	'"': "text",
	'0': "a number",
	'{': "a table",
	'[': "a list",
	't': "a boolean",
	'f': "a boolean",
	'n': "nil",
}

// check returns an error for the first value that rexdiff does not accept.
// set has the keys that the text sets.
func (c config) check(set map[string]jsontext.Value) error {
	if err := c.checkChoices(); err != nil {
		return err
	}
	if err := c.checkTexts(set); err != nil {
		return err
	}
	if err := c.checkHosts(set); err != nil {
		return err
	}
	return c.checkKeybindings()
}

// checkChoices checks the values that have fixed choices or a range. They
// are the layout, the scope, the markdown view, and the file list.
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
	if c.FileListWidth < minListWidth {
		want := strconv.Itoa(minListWidth) + " or more"
		return invalid("file_list_width", want, strconv.Itoa(c.FileListWidth))
	}
	return nil
}

// checkTexts checks the commands and the review prefix. A command can use
// only its placeholders. A text that the user sets must not be blank.
func (c config) checkTexts(set map[string]jsontext.Value) error {
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
		blank := set[k.key] != nil && strings.TrimSpace(k.value) == ""
		if blank || !placeholders(k.value, k.known...) {
			return invalid(k.key, k.want, k.value)
		}
	}
	if set["review_prefix"] != nil && strings.TrimSpace(c.ReviewPrefix) == "" {
		return invalid("review_prefix", "text", c.ReviewPrefix)
	}
	if set["guide_agent"] != nil && strings.TrimSpace(c.GuideAgent) == "" {
		return invalid("guide_agent", "a command", c.GuideAgent)
	}
	return nil
}

// checkHosts checks the hostnames of the self-hosted forges.
func (c config) checkHosts(set map[string]jsontext.Value) error {
	hosts := []struct{ key, value string }{
		{"github_host", c.GitHubHost},
		{"gitlab_host", c.GitLabHost},
		{"azure_devops_host", c.AzureHost},
	}
	for _, k := range hosts {
		if set[k.key] != nil && !hostname(k.value) {
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

// focusAgent tells if the focus moves to the agent after a send.
func (c config) focusAgent() bool { return c.FocusAgent == nil || *c.FocusAgent }

func names[T ~string](values ...T) []string {
	s := make([]string, len(values))
	for i, v := range values {
		s[i] = string(v)
	}
	return s
}
