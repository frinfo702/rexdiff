package tui

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"
)

// TestLogos checks that each agent CLI has a logo, and that the cells of
// a logo take logoCols columns, so that the rows around it line up.
func TestLogos(t *testing.T) {
	for _, a := range guideAgents {
		b, err := logoFiles.ReadFile("logos/" + a.name + ".png")
		if err != nil {
			t.Errorf("%s: %v", a.name, err)
			continue
		}
		if _, err := png.Decode(bytes.NewReader(b)); err != nil {
			t.Errorf("%s: %v", a.name, err)
		}
	}
	if w := lipgloss.Width(logoCells(logoImageID(0), lipgloss.NewStyle()) + " claude"); w != logoCols+7 {
		t.Errorf("width of a logo and its name = %d, want %d", w, logoCols+7)
	}
}

// TestGuideAgentPicker checks that a in the Guide tab lists the agent
// CLIs, that an agent that is not installed is not picked, and that an
// installed one writes the guide.
func TestGuideAgentPicker(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // the cache of the guides
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	m, _ := newTestModel(t)

	// The PATH has git and a fake agy, and no other agent.
	bin := t.TempDir()
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(git, filepath.Join(bin, "git")); err != nil {
		t.Fatal(err)
	}
	write(t, bin, "agy", `#!/bin/sh
echo '{"sections":[{"title":"Written by agy","summary":"S.","files":["a.go"]}]}'
`)
	if err := os.Chmod(filepath.Join(bin, "agy"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":/bin")

	m.cfg.GuideAgent = "codex" // a name stands for its command
	if got := m.guideAgent(0); got != "codex exec -" {
		t.Fatalf("guideAgent = %q", got)
	}
	m.tab = tabGuide
	m = press(t, m, "a")
	if m.overlay == nil || m.overlay.kind != overlayGuideAgent ||
		!shows(m, "claude", "opencode", "cursor-agent", "agy", "not installed") {
		t.Fatalf("a: no picker of agents:\n%s", view(m))
	}

	m = press(t, m, "enter") // the cursor starts on codex, the current agent
	if m.guide.running || m.guidePick != "" {
		t.Fatal("the picker ran codex, which is not installed")
	}

	m = press(t, m, "r") // write the guide with codex
	if m.guide.running || m.guide.err != `agent "codex" not found in PATH` {
		t.Fatalf("r with codex, which is not installed: %+v", m.guide)
	}

	m = press(t, m, "a", "j", "j", "j") // codex, opencode, cursor-agent, agy
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = *next.(*model)
	if cmd == nil || !m.guide.running || agentName(m.guide.agent) != "agy" {
		t.Fatalf("picking agy did not run it: %+v", m.guide)
	}
	m = press(t, m, "a", "enter") // another pick while agy writes
	if !strings.Contains(m.status, "wait for agy to finish") {
		t.Errorf("a pick during a run: status %q", m.status)
	}
	for _, c := range cmd().(tea.BatchMsg) {
		if c == nil {
			continue
		}
		if msg, ok := c().(guideMsg); ok {
			m = step(t, m, msg)
		}
	}
	if !shows(m, "Written by agy") {
		t.Errorf("Guide tab after agy:\n%s", view(m))
	}
}

// TestRasterLogo checks that the kitty renderer paints a logo in its
// picture of the line, where the terminal would paint it under the
// picture, in the colour of the text.
func TestRasterLogo(t *testing.T) {
	r := newRaster(16, 34)
	scr := uv.NewScreenBuffer(4, 1)
	uv.NewStyledString(logoCells(logoImageID(0), lipgloss.NewStyle())+"  ").Draw(scr, scr.Bounds())
	pal := &palette{fg: color.RGBA{0xdd, 0xdd, 0xdd, 0xff}, bg: color.RGBA{0, 0, 0, 0xff}}
	img := image.NewRGBA(image.Rect(0, 0, 4*16, 34))
	r.draw(img, scr.Line(0), pal, -1)
	if c := img.RGBAAt(16, 17); c != pal.fg { // the middle of the logo of claude
		t.Errorf("the middle of the logo is %v, not the colour of the text", c)
	}
	if c := img.RGBAAt(3*16+8, 17); c != pal.bg {
		t.Errorf("the cell after the logo is %v, not the background", c)
	}
}

// TestRecolorLogos checks that the logos go to the terminal once, and
// again when the colour of the text changes with the theme.
func TestRecolorLogos(t *testing.T) {
	m, _ := newTestModel(t)
	m.logos = true
	if m.sendLogos() == nil || m.sendLogos() != nil {
		t.Fatal("sendLogos does not send the logos once")
	}
	if _, cmd := m.Update(tea.ForegroundColorMsg{Color: color.RGBA{0x22, 0x22, 0x22, 0xff}}); cmd == nil {
		t.Error("a new colour of the text does not send the logos again")
	}
}

// TestGuideAgentPickerSelection checks that the name of the selected agent
// has the background of the selection after its logo, and that the list
// does not show the commands of the agents.
func TestGuideAgentPickerSelection(t *testing.T) {
	m, _ := newTestModel(t)
	m.logos, m.bg, m.dark = true, color.RGBA{0x10, 0x10, 0x10, 0xff}, true
	m.tab = tabGuide
	m.openGuideAgentPicker()
	m.overlay.cur = 0 // claude
	rows, _ := m.overlayRows(40)
	scr := uv.NewScreenBuffer(40, 1)
	uv.NewStyledString(rows[0]).Draw(scr, scr.Bounds())
	line := scr.Line(0)
	if got := ansi.Strip(rows[0]); strings.Contains(got, "-p") {
		t.Errorf("the row shows the command: %q", got)
	}
	x := 2 + logoCols + 1 // "▶ ", the logo, a space
	if c := line[x]; c.Content != "c" || c.Style.Bg == nil {
		t.Errorf("the cell of the name is %q with background %v", c.Content, c.Style.Bg)
	}
}

// TestGuideAgentPickerCommand checks that the picker lists the command of
// the setting, with its text, when it is not one of the agent CLIs.
func TestGuideAgentPickerCommand(t *testing.T) {
	m, _ := newTestModel(t)
	m.cfg.GuideAgent = "my-agent --fast"
	m.tab = tabGuide
	m = press(t, m, "a")
	if !shows(m, "guide_agent", "my-agent --fast", "current") {
		t.Errorf("the picker has no row for the command of the setting:\n%s", view(m))
	}
}

// TestCheckAgent checks the errors of an agent that cannot run: none at
// all, and an agent CLI that the PATH does not have. A command of the
// setting is left to the shell.
func TestCheckAgent(t *testing.T) {
	m, _ := newTestModel(t)
	t.Setenv("PATH", t.TempDir())
	cases := []struct{ command, want string }{
		{m.guideAgent(0), "no agent found"},
		{"codex exec -", `agent "codex" not found in PATH`},
		{"FOO=1 codex exec -", ""},
	}
	for _, c := range cases {
		if got := checkAgent(c.command, 0); got != c.want {
			t.Errorf("checkAgent(%q) = %q, want %q", c.command, got, c.want)
		}
	}
}

// TestKittyImages checks the guess of a terminal that shows images.
func TestKittyImages(t *testing.T) {
	cases := []struct {
		tmux, zellij, sty, program, kittyWindow, term string
		want                                          bool
	}{
		{program: "rex", want: true},
		{program: "ghostty", want: true},
		{kittyWindow: "1", want: true},
		{term: "xterm-kitty", want: true},
		{tmux: "/tmp/tmux", program: "ghostty", kittyWindow: "1", term: "xterm-kitty"},
		{program: "Apple_Terminal", term: "xterm-256color"},
		{zellij: "0", program: "ghostty"},
		{sty: "1.tty.host", program: "rex"},
	}
	for _, c := range cases {
		t.Setenv("TMUX", c.tmux)
		t.Setenv("ZELLIJ", c.zellij)
		t.Setenv("STY", c.sty)
		t.Setenv("TERM_PROGRAM", c.program)
		t.Setenv("KITTY_WINDOW_ID", c.kittyWindow)
		t.Setenv("TERM", c.term)
		if got := kittyImages(); got != c.want {
			t.Errorf("kittyImages() with %+v = %v", c, got)
		}
	}
}

// TestGuideProgressLogo checks the line of a run in the guide column: the
// logo of the agent before its first line, and the next lines in line with
// the text.
func TestGuideProgressLogo(t *testing.T) {
	m, _ := newTestModel(t)
	m.logos = true
	m.guide = guideState{
		sections: []guideSection{{Title: "T", Summary: "S", Files: []string{"a.go"}}},
		running:  true,
		started:  time.Now(),
		agent:    "cursor-agent -p",
	}
	lines, _, _ := m.guideLines(40)
	i := slices.IndexFunc(lines, func(l string) bool { return strings.ContainsRune(l, kitty.Placeholder) })
	if i < 0 || i+1 >= len(lines) {
		t.Fatalf("no logo in the guide column:\n%s", strings.Join(lines, "\n"))
	}
	first, next := ansi.Strip(lines[i]), ansi.Strip(lines[i+1])
	if !strings.Contains(first, "cursor-agent is") || !strings.HasPrefix(next, strings.Repeat(" ", 1+logoCols+1)) || strings.TrimSpace(next) == "" {
		t.Errorf("progress lines:\n%q\n%q", first, next)
	}
}

// TestGuideBigDiff checks a diff too big for an argument: the agents that
// take the prompt as one are off, the others carry a warning, and the
// first agent that is installed is one that reads its standard input.
func TestGuideBigDiff(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // the cache of the guides
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	m, _ := newTestModel(t)
	bin := t.TempDir()
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(git, filepath.Join(bin, "git")); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"agy", "claude"} {
		write(t, bin, name, "#!/bin/sh\ncat >/dev/null\n")
		if err := os.Chmod(filepath.Join(bin, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+":/bin")
	defer func(n int) { maxArg = n }(maxArg)
	maxArg = 10

	if got := m.guideAgent(100); got != "claude -p" {
		t.Errorf("first agent for a big diff = %q, want claude -p", got)
	}
	m.cfg.GuideAgent = "agy"
	m.tab = tabGuide
	m = press(t, m, "r")
	if m.guide.running || !strings.HasPrefix(m.guide.err, "the diff is too big for agy") {
		t.Fatalf("r with agy and a big diff: %+v", m.guide)
	}
	m = press(t, m, "a")
	if !shows(m, "diff too big", "big diff, slow") {
		t.Fatalf("picker for a big diff:\n%s", view(m))
	}
	m = press(t, m, "j", "j", "j", "j", "j", "enter") // agy
	if m.guide.running || !strings.Contains(m.status, "too big for agy") {
		t.Errorf("picking agy for a big diff: running %v, status %q", m.guide.running, m.status)
	}
	m.cfg.GuideAgent = "claude" // reads its standard input: it runs, with a warning
	m = press(t, m, "r")
	if !m.guide.running || !strings.Contains(m.status, "the diff is big") {
		t.Errorf("r with claude and a big diff: running %v, status %q", m.guide.running, m.status)
	}
}

// TestLogosAfterEditor checks that the logos go again after the editor,
// which takes the terminal off the alternate screen and its images.
func TestLogosAfterEditor(t *testing.T) {
	m, _ := newTestModel(t)
	m.logos, m.logosSent = true, true
	m = step(t, m, editorDoneMsg{})
	if m.logosSent {
		t.Fatal("the logos count as sent after the editor")
	}
	m.guide.running, m.guide.started = true, time.Now()
	m = step(t, m, frameMsg(m.guide.started))
	if !m.logosSent {
		t.Error("a frame of a run does not send the logos")
	}
}
