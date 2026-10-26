package tui

// The Overview tab tells where the branch stands, from git alone: the
// branch and its base, the commits ahead and behind, and the size of the
// changes. It needs no forge and no login.

import (
	"cmp"
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/frinfo702/rexdiff/diff"
	"github.com/frinfo702/rexdiff/vcs"
)

// Sizes of the Overview tab.
const (
	overviewWidth   = 100 // the widest column of text; a wider pane centres it
	overviewCommits = 50  // the most commits ahead that the tab lists
	overviewBehind  = 10  // the most commits behind that the tab lists
)

// renderOverview shows the Overview tab.
func (m *model) renderOverview() {
	p := &m.overPane
	p.path, p.note = "", ""
	w := min(m.mainWidth()-4, overviewWidth)
	indent := strings.Repeat(" ", max((m.mainWidth()-w)/2, 2))

	var lines []string
	add := func(ls ...string) {
		for _, l := range ls {
			lines = append(lines, truncate(indent+l, m.mainWidth()))
		}
	}
	add("")
	add(m.overviewTitle(w)...)
	add("")
	add(m.overviewSync()...)
	add("", "")
	add(m.overviewChanges(w)...)
	add("", "")
	add(m.overviewCommits(w)...)
	p.set(lines, make([]diff.Row, len(lines)))
}

// overviewTitle returns the name of the branch, and the base that it
// merges into.
func (m *model) overviewTitle(w int) []string {
	ctx := m.ctx
	branch := m.repo.Branch(ctx)
	title := branch
	if branch == "" {
		head, err := m.repo.Resolve(ctx, "HEAD")
		if err != nil {
			return []string{lipgloss.NewStyle().Bold(true).Render("No commits yet")}
		}
		title, branch = "HEAD detached at "+shortSHA(head), shortSHA(head)
	}
	title = lipgloss.NewStyle().Bold(true).Render(diff.Quote(title))
	route := dimStyle.Render(diff.Quote(m.base)+" ← ") + diff.Quote(branch)
	return []string{truncate(title, w), truncate(route, w)}
}

// overviewSync returns the commits ahead of and behind the base, and the
// state against the upstream branch.
func (m *model) overviewSync() []string {
	ctx := m.ctx
	var parts []string
	if ahead, behind, err := m.repo.AheadBehind(ctx, m.base); err == nil {
		parts = append(parts,
			countStyle(ahead, addStyle).Render(fmt.Sprintf("↑ %d ahead", ahead)),
			countStyle(behind, delStyle).Render(fmt.Sprintf("↓ %d behind", behind))+
				dimStyle.Render(" "+diff.Quote(m.base)),
		)
	}
	up := m.repo.Upstream(ctx)
	switch ahead, behind, err := m.repo.AheadBehind(ctx, up); {
	case up == "":
		parts = append(parts, dimStyle.Render("not pushed"))
	case err != nil:
	case ahead == 0 && behind == 0:
		parts = append(parts, dimStyle.Render("in sync with "+diff.Quote(up)))
	default:
		var s []string
		if ahead > 0 {
			s = append(s, plural(ahead, "commit")+" to push")
		}
		if behind > 0 {
			s = append(s, fmt.Sprintf("%d to pull", behind))
		}
		parts = append(parts, noteStyle.Render(strings.Join(s, ", "))+
			dimStyle.Render(" · "+diff.Quote(up)))
	}
	return []string{strings.Join(parts, dimStyle.Render("   "))}
}

// countStyle returns style for a count above 0, and the dim style for 0.
func countStyle(n int, style lipgloss.Style) lipgloss.Style {
	if n == 0 {
		return dimStyle
	}
	return style
}

// overviewChanges returns the size of the changes of the scope.
func (m *model) overviewChanges(w int) []string {
	head := dimStyle.Render("Changes · " + m.scope.label())
	if len(m.files) == 0 {
		note := cmp.Or(m.changes.note, "no changes")
		return []string{head, truncate(dimStyle.Render(note), w)}
	}
	var adds, dels, binary int
	for _, f := range m.files {
		adds, dels = adds+f.adds, dels+f.dels
		if f.Binary {
			binary++
		}
	}
	parts := []string{
		plural(len(m.files), "file"),
		addStyle.Render(fmt.Sprintf("+%d", adds)) + " " + delStyle.Render(fmt.Sprintf("-%d", dels)),
	}
	if binary > 0 {
		parts = append(parts, dimStyle.Render(plural(binary, "binary file")))
	}
	if n := m.comments.Len(); n > 0 {
		parts = append(parts, cursorStyle.Render("● "+plural(n, "comment")))
	}
	return []string{head, strings.Join(parts, "   ")}
}

// overviewCommits returns the commits of the branch that the base does
// not have, and the commits of the base that the branch does not have.
func (m *model) overviewCommits(w int) []string {
	ctx := m.ctx
	ahead, err := m.repo.Log(ctx, m.base+"..HEAD", overviewCommits)
	if err != nil {
		return nil
	}
	out := []string{dimStyle.Render(fmt.Sprintf("Commits  %d", len(ahead)))}
	if len(ahead) == 0 {
		out = append(out, dimStyle.Render("no commits ahead of "+diff.Quote(m.base)))
	}
	for _, c := range ahead {
		out = append(out, commitRow(c, w))
	}
	behind, err := m.repo.Log(ctx, "HEAD.."+m.base, overviewBehind)
	if err != nil || len(behind) == 0 {
		return out
	}
	out = append(out, "", "", dimStyle.Render("New on "+diff.Quote(m.base)))
	for _, c := range behind {
		out = append(out, commitRow(c, w))
	}
	return out
}

// commitRow returns "abc1234  subject      author · 2 hours ago", w
// columns wide. The subject gives way first.
func commitRow(c vcs.Commit, w int) string {
	sha := noteStyle.Render(c.Short) + "  "
	meta := dimStyle.Render(diff.Quote(c.Author) + " · " + c.When)
	room := w - lipgloss.Width(sha) - lipgloss.Width(meta) - 2
	if room < 16 { // too narrow for the author: keep the subject
		meta = dimStyle.Render(c.When)
		room = w - lipgloss.Width(sha) - lipgloss.Width(meta) - 2
	}
	subject := truncate(diff.Quote(c.Subject), room)
	pad := strings.Repeat(" ", max(room-lipgloss.Width(subject), 0)+2)
	return sha + subject + pad + meta
}
