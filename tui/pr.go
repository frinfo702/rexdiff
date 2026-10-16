package tui

// The PR tab shows the pull request of the branch. It cannot change it.

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/rednafi/rexdiff/diff"
	"github.com/rednafi/rexdiff/vcs"
)

// loadPR returns a command that reads the pull request of the branch.
func (m *model) loadPR() tea.Cmd {
	repo, ctx := m.repo, m.ctx
	hosts := vcs.Hosts{
		GitHub: m.cfg.GitHubHost,
		GitLab: m.cfg.GitLabHost,
		Azure:  m.cfg.AzureHost,
	}
	return func() tea.Msg {
		pr, err := repo.PullRequest(ctx, hosts)
		if pr != nil {
			cleanPR(pr)
		}
		return prMsg{pr, err}
	}
}

// cleanPR removes the control characters from the text of pr, because the
// text comes from other users.
func cleanPR(pr *vcs.PullRequest) {
	texts := []*string{&pr.Title, &pr.URL, &pr.Author, &pr.Head, &pr.Base, &pr.Body}
	for i := range pr.Checks {
		texts = append(texts, &pr.Checks[i].Name)
	}
	for i := range pr.Comments {
		c := &pr.Comments[i]
		texts = append(texts, &c.Author, &c.When, &c.Body)
	}
	for _, s := range texts {
		*s = diff.Plain(*s)
	}
}

// prStateStyle returns the style that shows the state of a pull request.
func prStateStyle(s vcs.State) lipgloss.Style {
	switch s {
	case vcs.StateOpen:
		return addStyle
	case vcs.StateMerged:
		return headerStyle
	case vcs.StateClosed:
		return delStyle
	}
	return dimStyle
}

// checkSummary counts the checks that passed, failed, or are pending, and
// returns the names of the checks that failed. A skipped check counts as
// passed.
func checkSummary(checks []vcs.Check) (pass, fail, pend int, failed []string) {
	for _, c := range checks {
		switch c.Result {
		case vcs.ResultPass, vcs.ResultSkip:
			pass++
		case vcs.ResultFail:
			fail++
			failed = append(failed, c.Name)
		default:
			pend++
		}
	}
	return pass, fail, pend, failed
}

// renderPR shows the pull request in the PR tab.
func (m *model) renderPR() {
	p := &m.prPane
	p.path, p.note = "", ""
	pr := m.pr.pr
	switch {
	case m.pr.err != "":
		p.note = m.pr.err
		p.set(nil, nil)
		return
	case pr == nil:
		if !m.pr.loading {
			p.note = vcs.ErrNotFound.Error()
		}
		p.set(nil, nil)
		return
	}
	md := m.markdown(m.codeWidth() - 4)
	lines := append([]string{""}, prHeader(pr)...)
	lines = append(lines, checkLines(pr.Checks)...)
	lines = append(lines, "")
	if strings.TrimSpace(pr.Body) != "" {
		lines = append(lines, renderMarkdown(md, pr.Body)...)
	}
	for _, c := range pr.Comments {
		author := headerStyle.Render(c.Author) + "  " + dimStyle.Render(c.When)
		lines = append(lines, "", "  "+author)
		lines = append(lines, renderMarkdown(md, c.Body)...)
	}
	p.set(lines, make([]diff.Row, len(lines)))
}

// prHeader returns the title line and the state line of pr.
func prHeader(pr *vcs.PullRequest) []string {
	title := lipgloss.NewStyle().Bold(true).Render(pr.Title)
	number := dimStyle.Render(fmt.Sprintf("#%d", pr.Number))
	state := prStateStyle(pr.State).Render("● " + string(pr.State))
	return []string{
		fmt.Sprintf("  %s %s", title, number),
		fmt.Sprintf(
			"  %s  %s → %s  %s  %s",
			state,
			pr.Head,
			pr.Base,
			dimStyle.Render("by "+pr.Author),
			dimStyle.Render(pr.URL),
		),
	}
}

// checkLines returns the counts of the checks, and the names of the
// checks that failed. With no checks, it returns no lines.
func checkLines(checks []vcs.Check) []string {
	if len(checks) == 0 {
		return nil
	}
	pass, fail, pend, failed := checkSummary(checks)
	lines := []string{fmt.Sprintf(
		"  checks: %s  %s  %s",
		addStyle.Render(fmt.Sprintf("✓ %d", pass)),
		delStyle.Render(fmt.Sprintf("✗ %d", fail)),
		noteStyle.Render(fmt.Sprintf("◌ %d", pend)),
	)}
	for _, f := range failed {
		lines = append(lines, "    "+delStyle.Render("✗ ")+f)
	}
	return lines
}
