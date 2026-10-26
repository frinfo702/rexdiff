package tui

// The Guide tab reads the changes in the order of their meaning, as
// pulls.review does. An agent CLI (claude, pi, or codex) groups the
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
// that rexdiff tries them. Each one reads the prompt on its standard input.
var guideAgents = []struct{ name, command string }{
	{"claude", "claude -p"},
	{"pi", "pi -p"},
	{"codex", "codex exec -"},
}

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
		return g.err + " · press r to try again"
	case len(m.files) == 0:
		return cmp.Or(m.changes.note, "no changes to explain")
	}
	return "press r to write a guide"
}

// guideProgress tells which agent writes the guide, and for how long.
func (m *model) guideProgress() string {
	name, _, _ := strings.Cut(m.guide.agent, " ")
	secs := int(time.Since(m.guide.started).Seconds())
	return fmt.Sprintf("%s is reading the changes… %ds", name, secs)
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
		add(noteStyle, "↻ "+m.guideProgress())
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
	agent := m.guideAgent()
	if agent == "" {
		m.guide.err = "no agent to write the guide: install claude or pi, or set guide_agent"
		m.renderGuide()
		return nil
	}
	m.guide.running, m.guide.started, m.guide.agent, m.guide.err = true, time.Now(), agent, ""
	m.renderGuide()

	ctx, dir, trees := m.ctx, m.repo.Root(), m.shownTrees
	prompt := guidePrompt(m.guideCommits(), m.files)
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
// to right, d after the start, as Rex marks a working agent.
func shimmer(style lipgloss.Style, s string, d time.Duration) string {
	rs := []rune(s)
	const tail = 3 // the frames that the band is off the text at each end
	pos := int(d/guideFrame)%(len(rs)+2*tail) - tail
	var b strings.Builder
	for i, r := range rs {
		st := style // the band lights the text from the colour of the style
		switch abs(i - pos) {
		case 0:
			st = style.Foreground(lipgloss.Color("3")).Bold(true)
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

// guideAgent returns the command that writes the guide: the setting, or
// the first agent CLI that is installed, or "".
func (m *model) guideAgent() string {
	if m.cfg.GuideAgent != "" {
		return m.cfg.GuideAgent
	}
	for _, a := range guideAgents {
		if _, err := exec.LookPath(a.name); err == nil {
			return a.command
		}
	}
	return ""
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
