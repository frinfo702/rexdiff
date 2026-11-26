package tui

// The Guide tab reads the changes in the order of their meaning, as
// pulls.review does. An agent CLI (claude, pi, codex, and others) groups the
// changed files into sections, each with a title and a short summary. The
// tab shows a section at the left and the diff of one of its files at the
// right. The guide of a pair of trees is kept on disk, so the agent writes
// it once.

import (
	"bytes"
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/aymanbagabas/go-udiff"

	"github.com/frinfo702/rexdiff/diff"
)

// Limits of the guide.
const (
	guideWidth    = 48                    // the widest column of the guide text
	guideMaxDiff  = 200_000               // the most bytes of diff that the agent reads
	guideTimeout  = 5 * time.Minute       // the longest that the agent can take
	guideCommits  = 20                    // the most commit subjects in the prompt
	guideMaxTitle = 80                    // the longest title, in columns
	guideFrame    = 80 * time.Millisecond // a step of the shimmer of the Guide tab
)

// guideAgents are the agent CLIs that can write the guide, in the order
// that rexdiff tries them. Each command reads the prompt on its standard
// input. opencode, cursor-agent, and agy take the prompt only as an
// argument, so their commands pass the standard input as one, and a big
// diff does not fit. opencode and cursor-agent run in their modes that do
// not edit, since the diff can hold instructions.
var guideAgents = []agentCLI{
	{"claude", "claude -p", false},
	{"pi", "pi -p", false},
	{"codex", "codex exec -", false},
	{"opencode", `opencode run --agent plan "$(cat)"`, true},
	{"cursor-agent", `cursor-agent -p --mode ask --trust "$(cat)"`, true},
	{"agy", `agy -p "$(cat)"`, true},
}

// agentCLI is an agent CLI that can write the guide, and its command. arg
// tells that the command takes the prompt as an argument.
type agentCLI struct {
	name, command string
	arg           bool
}

// maxArg is the longest prompt, in bytes, that an agent can take as an
// argument. Linux caps one argument (MAX_ARG_STRLEN), and macOS all the
// arguments and the environment together (ARG_MAX). A variable, so that
// tests can set it.
var maxArg = func() int {
	if runtime.GOOS == "linux" {
		return 128<<10 - 1
	}
	env := 0
	for _, e := range os.Environ() {
		env += len(e) + 1 + 8 // the text, its NUL, and its pointer
	}
	return 1<<20 - env - 4<<10 // ponytail: 4 KiB for the other arguments
}()

// bigPrompt reports whether a prompt of n bytes does not fit an argument.
func bigPrompt(n int) bool { return n > maxArg }

// megabytes writes n bytes in MB, for the messages about a big diff.
func megabytes(n int) string { return fmt.Sprintf("%.1f MB", float64(n)/(1<<20)) }

// guideSection is one part of the guide: the files of one change.
type guideSection struct {
	Title   string   `json:"title"`
	Summary string   `json:"summary"`
	Files   []string `json:"files"`
}

// guideState is the content of the Guide tab.
type guideState struct {
	trees    string // the trees that sections explain
	sections []guideSection
	running  bool
	started  time.Time // the start of the run
	agent    string    // the command of the run
	err      string
}

// guideFile is a file of the guide, and the section that it is in.
type guideFile struct {
	section int
	path    string
}

// guideSections returns the sections of the guide, with a last section
// for the changed files that the agent left out.
func (m *model) guideSections() []guideSection {
	secs := m.guide.sections
	if len(secs) == 0 {
		return nil
	}
	seen := map[string]bool{}
	for _, s := range secs {
		for _, f := range s.Files {
			seen[f] = true
		}
	}
	var rest []string
	for _, f := range m.files {
		if !seen[f.Path] {
			rest = append(rest, f.Path)
		}
	}
	if len(rest) > 0 {
		secs = append(slices.Clip(secs), guideSection{
			Title:   "Other changes",
			Summary: "Files that the guide does not mention.",
			Files:   rest,
		})
	}
	return secs
}

// guideFiles returns the files of all the sections, in order.
func (m *model) guideFiles() []guideFile {
	var out []guideFile
	for i, s := range m.guideSections() {
		for _, f := range s.Files {
			out = append(out, guideFile{i, f})
		}
	}
	return out
}

// guideSectionIndex returns the section of the selected file.
func (m *model) guideSectionIndex() int {
	fs := m.guideFiles()
	if m.guideSel >= len(fs) {
		return 0
	}
	return fs[m.guideSel].section
}

// guideSection returns the section of the selected file.
func (m *model) guideSection() guideSection {
	secs := m.guideSections()
	if len(secs) == 0 {
		return guideSection{}
	}
	return secs[m.guideSectionIndex()]
}

// guideSectionStart returns the index of the first file of the section of
// the selected file.
func (m *model) guideSectionStart() int { return m.guideSectionFirst(m.guideSectionIndex()) }

// renderGuide shows the selected file of the guide, or why there is no
// guide.
func (m *model) renderGuide() {
	p := &m.guidePane
	p.note = ""
	fs := m.guideFiles()
	if len(fs) == 0 {
		p.path, p.note = "", m.guideNote()
		p.set(nil, nil)
		return
	}
	m.guideSel = min(m.guideSel, len(fs)-1)
	f := m.changedAt(fs[m.guideSel].path)
	if f == nil {
		p.path = ""
		p.setNote(diff.Quote(fs[m.guideSel].path) + " has no changes in this scope now")
		return
	}
	m.renderChanged(p, f)
}

// guideNote tells why the Guide tab has no guide.
func (m *model) guideNote() string {
	g := m.guide
	switch {
	case g.running:
		return m.guideProgress()
	case g.err != "":
		return g.err + " · press r to try again, a to pick the agent"
	case len(m.files) == 0:
		return cmp.Or(m.changes.note, "no changes to explain")
	}
	return "press r to write a guide, a to pick the agent"
}

// guideProgress tells which agent writes the guide, and for how long.
func (m *model) guideProgress() string {
	secs := int(time.Since(m.guide.started).Seconds())
	return fmt.Sprintf("%s is reading the changes… %ds", agentName(m.guide.agent), secs)
}

// agentName returns the program of the command of an agent, for example
// "claude" for "claude -p".
func agentName(command string) string {
	name, _, _ := strings.Cut(strings.TrimSpace(command), " ")
	return name
}

// guideLines renders h lines of the guide column: the section, its
// summary, its files, and the titles of all the sections. first is the
// line of the first file, and outline the line of the first title, or -1.
func (m *model) guideLines(h int) (lines []string, first, outline int) {
	w := m.listWidth()
	secs := m.guideSections()
	if len(secs) == 0 {
		return nil, -1, -1
	}
	sec := m.guideSectionIndex()
	s := secs[sec]
	// add adds the lines of text, wrapped to the column.
	add := func(style lipgloss.Style, text string) {
		for l := range strings.SplitSeq(wrapWords(text, w-2), "\n") {
			lines = append(lines, " "+style.Render(l))
		}
	}
	lines = append(lines, "")
	switch {
	case m.guide.running:
		icon := cmp.Or(m.agentLogo(agentName(m.guide.agent)), noteStyle.Render("↻")+" ")
		for i, l := range strings.Split(wrapWords(m.guideProgress(), w-2-logoCols-1), "\n") {
			if i > 0 {
				icon = "  "
			}
			lines = append(lines, " "+icon+" "+noteStyle.Render(l))
		}
		lines = append(lines, "")
	case m.guide.trees != m.shownTrees:
		add(noteStyle, "↻ the changes moved on · r to rewrite")
		lines = append(lines, "")
	}
	add(lipgloss.NewStyle().Bold(true), s.Title)
	add(dimStyle, fmt.Sprintf("%02d / %02d", sec+1, len(secs)))
	lines = append(lines, "")
	// The summary gives way to the files.
	summary := strings.Split(wrapWords(s.Summary, w-2), "\n")
	room := max(h-len(lines)-len(s.Files)-1, 1)
	if len(summary) > room {
		summary = append(summary[:room-1], "…")
	}
	for _, l := range summary {
		lines = append(lines, " "+l)
	}
	lines = append(lines, "")
	first = len(lines)
	start := m.guideSectionStart()
	for i, p := range s.Files {
		r := dimStyle.Render(fileRow(" ", p, "", w))
		if f := m.changedAt(p); f != nil {
			r = m.changedRow(*f, w)
		}
		if start+i == m.guideSel {
			r = m.selectedRow(r, w)
		}
		lines = append(lines, r)
	}
	if len(secs) < 2 {
		return lines, first, -1
	}
	lines = append(lines, "", "")
	outline = len(lines)
	for i, s := range secs {
		style := dimStyle
		if i == sec {
			style = lipgloss.NewStyle()
		}
		lines = append(lines, " "+style.Render(truncate(fmt.Sprintf("%02d  %s", i+1, s.Title), w-2)))
	}
	return lines, first, outline
}

// guideSectionFirst returns the index of the first file of section sec.
func (m *model) guideSectionFirst(sec int) int {
	return slices.IndexFunc(m.guideFiles(), func(f guideFile) bool { return f.section == sec })
}

// ensureGuide shows the guide of the current trees from the disk, or
// starts to write one if there is no guide. A guide of other trees stays
// until the user asks for a new one, because each guide costs a run of the
// agent.
func (m *model) ensureGuide() tea.Cmd {
	if m.guide.running || m.shownTrees == "" || m.guide.trees == m.shownTrees {
		return nil
	}
	if secs := loadGuide(m.shownTrees); secs != nil {
		m.guide = guideState{trees: m.shownTrees, sections: secs}
		m.guideSel = 0
		m.renderGuide()
		return nil
	}
	if m.guide.sections != nil {
		return nil
	}
	return m.writeGuide()
}

// writeGuide returns the command that runs the agent on the changes of
// the scope.
func (m *model) writeGuide() tea.Cmd {
	if m.guide.running {
		return nil
	}
	if len(m.files) == 0 {
		m.renderGuide()
		return nil
	}
	prompt := guidePrompt(m.guideCommits(), m.files)
	agent := m.guideAgent(len(prompt))
	if err := checkAgent(agent, len(prompt)); err != "" {
		m.guide.err = err
		m.renderGuide()
		return nil
	}
	if bigPrompt(len(prompt)) {
		m.flash(fmt.Sprintf("the diff is big (%s): %s can take long", megabytes(len(prompt)), agentName(agent)))
	}
	m.guide.running, m.guide.started, m.guide.agent, m.guide.err = true, time.Now(), agent, ""
	m.renderGuide()

	ctx, dir, trees := m.ctx, m.repo.Root(), m.shownTrees
	// The logos go with the first frame, after Bubble Tea is on the
	// alternate screen, where the terminal keeps them.
	return tea.Batch(frame(m.guide.started), func() tea.Msg {
		out, err := runAgent(ctx, dir, agent, prompt)
		if err != nil {
			return guideMsg{trees: trees, err: err}
		}
		secs, err := parseGuide(out)
		if err == nil {
			saveGuide(trees, secs)
		}
		return guideMsg{trees: trees, sections: secs, err: err}
	})
}

// frame returns a command that sends the next frame of the shimmer of
// the run that started at start.
func frame(start time.Time) tea.Cmd {
	return tea.Tick(guideFrame, func(time.Time) tea.Msg { return frameMsg(start) })
}

// shimmer renders s in style with a band of light that sweeps from left
// to right, d after the start, as Rex marks a working agent. The band is
// white on a dark background and black on a light one.
func shimmer(style lipgloss.Style, s string, d time.Duration, dark bool) string {
	band := lipgloss.Color("#000000")
	if dark {
		band = lipgloss.Color("#ffffff")
	}
	rs := []rune(s)
	const tail = 3 // the frames that the band is off the text at each end
	pos := int(d/guideFrame)%(len(rs)+2*tail) - tail
	var b strings.Builder
	for i, r := range rs {
		st := style // the band lights the text from the colour of the style
		switch abs(i - pos) {
		case 0:
			st = style.Foreground(band).Bold(true)
		case 1:
			st = style.UnsetForeground()
		}
		b.WriteString(st.Render(string(r)))
	}
	return b.String()
}

func abs(n int) int { return max(n, -n) }

// onGuide shows the guide that the agent wrote. After an error, the old
// guide stays.
func (m *model) onGuide(msg guideMsg) {
	m.guide.running = false
	if msg.err != nil {
		m.guide.err = msg.err.Error()
		m.flash(m.guide.err)
	} else {
		m.guide = guideState{trees: msg.trees, sections: msg.sections}
		m.guideSel = 0
		m.guidePane.toStart()
	}
	if m.tab == tabGuide {
		m.renderGuide()
	}
}

// guideAgent returns the command that writes a guide of a prompt of n
// bytes: the agent that the user picked, or the setting, or the first
// agent CLI that is installed and takes the prompt, or "". The pick and
// the setting are a name of guideAgents or a command.
func (m *model) guideAgent(n int) string {
	if pick := cmp.Or(m.guidePick, m.cfg.GuideAgent); pick != "" {
		for _, a := range guideAgents {
			if a.name == pick {
				return a.command
			}
		}
		return pick
	}
	for _, a := range guideAgents {
		if installed(a.name) && !(a.arg && bigPrompt(n)) {
			return a.command
		}
	}
	return ""
}

// checkAgent returns why the agent command cannot write a guide of a
// prompt of n bytes, or "". It checks only the agent CLIs of guideAgents:
// a command of the setting can start with a variable, as in
// "FOO=1 claude -p", and the shell tells when it finds no program.
func checkAgent(command string, n int) string {
	if command == "" {
		return "no agent found"
	}
	for _, a := range guideAgents {
		switch {
		case a.command != command:
		case !installed(a.name):
			return fmt.Sprintf("agent %q not found in PATH", a.name)
		case a.arg && bigPrompt(n):
			return fmt.Sprintf("the diff is too big for %s (%s, at most %s)", a.name, megabytes(n), megabytes(maxArg))
		}
	}
	return ""
}

// installed reports whether the PATH has program.
func installed(program string) bool {
	_, err := exec.LookPath(program)
	return err == nil
}

// openGuideAgentPicker lets the user pick the agent that writes the
// guide. The list has the agent CLIs, and the command of the setting if
// it is not one of them.
//
// A diff that is too big for an argument turns off the agents that take
// the prompt as one, and marks the others, which take long on it.
func (m *model) openGuideAgentPicker() tea.Cmd {
	ov := newOverlay(overlayGuideAgent, "which agent writes the guide?")
	n := len(guidePrompt(m.guideCommits(), m.files))
	cur := m.guideAgent(n)
	var items []pickItem
	for _, a := range guideAgents {
		it := pickItem{label: a.name, id: a.command, logo: m.agentLogoID(a.name)}
		var tags []string
		switch {
		case !installed(a.name):
			tags, it.off = []string{"not installed"}, a.name+" is not installed"
		case a.arg && bigPrompt(n):
			tags, it.off = []string{"diff too big"}, checkAgent(a.command, n)
		default:
			if a.command == cur {
				tags = append(tags, "current")
			}
			if bigPrompt(n) {
				tags = append(tags, "big diff, slow")
			}
		}
		it.tag = strings.Join(tags, " · ")
		items = append(items, it)
	}
	if c := m.cfg.GuideAgent; c != "" && !slices.ContainsFunc(guideAgents, func(a agentCLI) bool {
		return a.name == c || a.command == c
	}) {
		tag := ""
		if c == cur {
			tag = "current"
		}
		items = append(items, pickItem{label: "guide_agent", detail: c, tag: tag, id: c})
	}
	ov.setItems(items)
	ov.cur = max(slices.IndexFunc(items, func(it pickItem) bool { return it.id == cur }), 0)
	m.open(ov)
	return m.sendLogos()
}

// pickGuideAgent makes the agent of item it of the picker the agent of
// the guide, and writes the guide with it.
func (m *model) pickGuideAgent(it pickItem) tea.Cmd {
	command := it.id
	switch {
	case it.off != "":
		m.flash(it.off)
		return nil
	case m.guide.running:
		m.flash("wait for " + agentName(m.guide.agent) + " to finish")
		return nil
	}
	m.guidePick = command
	return m.writeGuide()
}

// guideCommits returns the subjects of the commits of the branch, which
// tell the agent the intent and the language of the changes.
func (m *model) guideCommits() []string {
	cs, _ := m.repo.Log(m.ctx, m.base+"..HEAD", guideCommits) // no commits is fine
	var out []string
	for _, c := range cs {
		out = append(out, c.Subject)
	}
	return out
}

// guidePrompt returns the prompt that asks the agent for the guide of
// files.
func guidePrompt(commits []string, files []changed) string {
	var b strings.Builder
	b.WriteString(`You write a review guide for a code change. Group the changed files into sections by meaning, in the order a reviewer should read them: the core change first, then what builds on it, then tests, docs, and chores.

Reply with JSON only, no prose and no code fence:
{"sections":[{"title":"...","summary":"...","files":["path/of/file"]}]}

- title: a short phrase in sentence case, no trailing period, for example "Run thinking-enabled queries".
- summary: one to three short sentences for the reviewer: what changed and why. Plain text, no Markdown.
- files: paths exactly as in the diff below. Put each file in one section.
- One section for a small change; at most eight.
- Write in the language of the commit messages; English if there are none.
`)
	if len(commits) > 0 {
		b.WriteString("\nCommits:\n")
		for _, c := range commits {
			b.WriteString("- " + c + "\n")
		}
	}
	b.WriteString("\nDiff:\n")
	for _, f := range files {
		switch {
		case f.Binary:
			fmt.Fprintf(&b, "\n%s: binary file\n", f.Path)
		case f.TooBig:
			fmt.Fprintf(&b, "\n%s: too big to show\n", f.Path)
		case b.Len() > guideMaxDiff:
			fmt.Fprintf(&b, "\n%s: +%d -%d, diff left out\n", f.Path, f.adds, f.dels)
		default:
			b.WriteString("\n" + udiff.Unified("a/"+f.Path, "b/"+f.Path, f.Old, f.New))
		}
	}
	return b.String()
}

// runAgent runs the shell command agent in dir with prompt on its standard
// input, and returns its standard output.
func runAgent(ctx context.Context, dir, agent, prompt string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, guideTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-c", agent)
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(prompt)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "", fmt.Errorf("guide: %s took over %s", agent, guideTimeout)
	}
	if err != nil {
		msg, _, _ := strings.Cut(strings.TrimSpace(stderr.String()), "\n")
		return "", fmt.Errorf("guide: %s: %s", agent, diff.Plain(cmp.Or(msg, err.Error())))
	}
	return string(out), nil
}

// parseGuide reads the sections from the answer of the agent. The answer
// can have text around the JSON.
func parseGuide(out string) ([]guideSection, error) {
	i, j := strings.Index(out, "{"), strings.LastIndex(out, "}")
	if i < 0 || j < i {
		return nil, errors.New("guide: the agent wrote no JSON")
	}
	var g struct {
		Sections []guideSection `json:"sections"`
	}
	if err := json.Unmarshal([]byte(out[i:j+1]), &g); err != nil {
		return nil, fmt.Errorf("guide: the agent wrote bad JSON: %w", err)
	}
	var secs []guideSection
	for _, s := range g.Sections {
		s.Title = truncate(diff.Plain(strings.TrimSpace(s.Title)), guideMaxTitle)
		s.Summary = diff.Plain(strings.TrimSpace(s.Summary))
		s.Files = slices.Compact(slices.DeleteFunc(s.Files, func(f string) bool { return f == "" }))
		if s.Title != "" && len(s.Files) > 0 {
			secs = append(secs, s)
		}
	}
	if len(secs) == 0 {
		return nil, errors.New("guide: the agent wrote no sections")
	}
	return secs, nil
}

// guidePath returns the file that keeps the guide of trees, or "".
func guidePath(trees string) string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	sum := sha256.Sum256([]byte(trees))
	return filepath.Join(dir, "rexdiff", "guides", hex.EncodeToString(sum[:8])+".json")
}

// loadGuide returns the kept guide of trees, or nil.
func loadGuide(trees string) []guideSection {
	b, err := os.ReadFile(guidePath(trees))
	if err != nil {
		return nil
	}
	var secs []guideSection
	if json.Unmarshal(b, &secs) != nil {
		return nil
	}
	return secs
}

// saveGuide keeps the guide of trees. A failure only costs a new run.
func saveGuide(trees string, secs []guideSection) {
	p := guidePath(trees)
	b, err := json.Marshal(secs)
	if p == "" || err != nil || os.MkdirAll(filepath.Dir(p), 0o700) != nil {
		return
	}
	_ = os.WriteFile(p, b, 0o600)
}
