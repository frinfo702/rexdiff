package tui

// The footer tells the user about a newer release. A key or a click
// installs it in the background. Then rexdiff starts again in the same
// pane, with the comments and the place of the user. The update package
// installs the release. This file shows the state of the update.

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/frinfo702/rexdiff/update"
)

// Updater finds and installs releases. *update.Updater implements it.
type Updater interface {
	// Latest returns the latest release, and whether it is newer.
	Latest(ctx context.Context) (update.Release, bool, error)
	// Install installs the release, so that the next start runs it.
	Install(ctx context.Context, r update.Release) error
}

// updatePhase is the step of the update of rexdiff.
type updatePhase int

const (
	phaseIdle       updatePhase = iota
	phaseInstalling             // the release installs in the background
	phaseInstalled              // rexdiff restarts when the user is ready
)

// installedMsg tells that the install of the release ended.
type installedMsg struct{ err error }

// checkRelease returns a command that asks for the latest release. Its
// message is the *update.Release if it is newer. A failure shows nothing,
// because rexdiff works the same without the check.
func (m *model) checkRelease() tea.Cmd {
	u, ctx := m.updater, m.ctx
	return func() tea.Msg {
		r, newer, err := u.Latest(ctx)
		if err != nil || !newer {
			return nil
		}
		return &r
	}
}

// releaseBadge returns the text of the footer about m.release.
func (m *model) releaseBadge() string {
	switch m.phase {
	case phaseInstalling:
		return fmt.Sprintf("updating rexdiff to %s…", m.release.Tag)
	case phaseInstalled:
		return fmt.Sprintf("rexdiff %s is installed; it starts when you are done", m.release.Tag)
	}
	return fmt.Sprintf("rexdiff %s is out: press U or click here to update", m.release.Tag)
}

// install starts the install of the release in the background.
func (m *model) install() tea.Cmd {
	if m.release == nil || m.phase != phaseIdle {
		return nil
	}
	m.phase = phaseInstalling
	u, ctx, r := m.updater, m.ctx, *m.release
	return func() tea.Msg { return installedMsg{u.Install(ctx, r)} }
}

// onInstalled prepares the restart, or tells why the install failed. After
// a failure, the user can try again.
func (m *model) onInstalled(msg installedMsg) {
	if msg.err != nil {
		m.phase = phaseIdle
		m.flash("update failed: " + msg.err.Error())
		return
	}
	m.phase = phaseInstalled
}

// restartIfReady saves the state for the restart into m.restartState. It
// reports whether rexdiff can quit for the restart. It waits while the
// user writes a comment or uses an overlay. It also waits while a send
// runs, and while pending (the command of the last message) runs. Thus no
// work is lost.
func (m *model) restartIfReady(pending tea.Cmd) bool {
	if m.mode != modeNormal || pending != nil || m.sends > 0 || m.guide.running {
		return false
	}
	path, err := m.saveState()
	if err != nil {
		m.phase, m.release = phaseIdle, nil
		m.flash("updated, but cannot save your comments for the restart: " + err.Error() +
			"; open rexdiff again for the new version")
		return false
	}
	m.restartState = path
	return true
}
