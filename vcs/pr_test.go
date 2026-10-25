package vcs_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/rednafi/rexdiff/vcs"
)

// fakeTool writes a script called name in dir. If the arguments of the
// script are a key of calls, it prints the value. Else it fails with
// "unexpected call".
func fakeTool(t *testing.T, dir, name string, calls map[string]string) {
	t.Helper()
	var script strings.Builder
	script.WriteString("#!/bin/sh\ncase \"$*\" in\n")
	for args, out := range calls {
		script.WriteString("'" + args + "') cat <<'EOF'\n" + out + "\nEOF\n;;\n")
	}
	script.WriteString("*) echo unexpected call: \"$*\" >&2; exit 1 ;;\nesac\n")
	if err := os.WriteFile(filepath.Join(dir, name), []byte(script.String()), 0o755); err != nil {
		t.Fatal(err)
	}
}

// repoAt returns the test repository, on the branch feat, with origin at
// the URL remote.
func repoAt(t *testing.T, remote string) (*vcs.Repo, string) {
	t.Helper()
	r, dir := newRepo(t)
	git(t, dir, "remote", "add", "origin", remote)
	return r, dir
}

// ghArgs are the arguments of the gh call that gets a pull request.
const ghArgs = "pr view --json number,title,state,isDraft,url,body,author," +
	"headRefName,baseRefName,comments,statusCheckRollup"

// azArgs returns the arguments of the az call that gets the pull request of
// branch.
func azArgs(branch string) string {
	return "repos pr list --source-branch " + branch + " --status all --top 1 --output json"
}

// TestPullRequest checks the pull request that each forge tool gives.
func TestPullRequest(t *testing.T) {
	dir := t.TempDir()
	fakePath(t, dir)
	fakeTool(t, dir, "gh", map[string]string{ghArgs: `{
		"number": 7, "title": "Add x", "state": "OPEN", "isDraft": true,
		"url": "https://github.com/o/r/pull/7",
		"body": "Body", "author": {"login": "ann"},
		"headRefName": "feat", "baseRefName": "main",
		"comments": [
			{"author": {"login": "bob"}, "body": "LGTM",
				"createdAt": "2026-01-02T03:04:05Z"}],
		"statusCheckRollup": [
			{"name": "test", "conclusion": "SUCCESS"},
			{"context": "ci/lint", "state": "PENDING"},
			{"name": "build", "conclusion": "STARTUP_FAILURE"},
			{"context": "ci/deploy", "state": "ERROR"},
			{"name": "docs", "conclusion": "NEUTRAL"},
			{"name": "e2e", "status": "IN_PROGRESS"}]}`})
	fakeTool(t, dir, "glab", map[string]string{
		"mr view --output json": `{"iid": 3, "title": "Fix y", "state": "opened",
		"web_url": "https://gitlab.com/o/r/-/merge_requests/3", "description": "D",
		"source_branch": "fix", "target_branch": "main", "author": {"username": "cy"}}`,
		// A list of discussions, as glab gives it. Each has its notes.
		"mr note list --output json": `[
		{"id": "d1", "notes": [
			{"body": "added 1 commit", "system": true, "author": {"username": "cy"}}]},
		{"id": "d2", "notes": [
			{"body": "Why?", "created_at": "2026-01-02T03:04:05Z",
				"author": {"username": "eve"}},
			{"body": "Speed.", "created_at": "2026-01-03T03:04:05Z",
				"author": {"username": "cy"}}]}]`,
	})
	fakeTool(t, dir, "az", map[string]string{azArgs("feat"): `[{
		"pullRequestId": 9, "title": "Z", "status": "active", "description": "",
		"sourceRefName": "refs/heads/feat", "targetRefName": "refs/heads/main",
		"createdBy": {"displayName": "Dee"},
		"repository": {"webUrl": "https://dev.azure.com/o/p/_git/r"}}]`})

	tests := []struct {
		remote string
		hosts  vcs.Hosts
		want   *vcs.PullRequest
	}{
		{"git@github.com:o/r.git", vcs.Hosts{}, &vcs.PullRequest{
			Forge:  vcs.ForgeGitHub,
			Number: 7,
			Title:  "Add x",
			State:  vcs.StateDraft,
			URL:    "https://github.com/o/r/pull/7",
			Author: "ann",
			Head:   "feat",
			Base:   "main",
			Body:   "Body",
			Checks: []vcs.Check{
				{Name: "test", Result: vcs.ResultPass},
				{Name: "ci/lint", Result: vcs.ResultPending},
				{Name: "build", Result: vcs.ResultFail},
				{Name: "ci/deploy", Result: vcs.ResultFail},
				{Name: "docs", Result: vcs.ResultSkip},
				{Name: "e2e", Result: vcs.ResultPending},
			},
			Comments: []vcs.Comment{
				{Author: "bob", When: "2026-01-02T03:04:05Z", Body: "LGTM"},
			},
		}},
		{"https://Git.Corp/o/r", vcs.Hosts{GitLab: "git.CORP"}, &vcs.PullRequest{
			Forge:  vcs.ForgeGitLab,
			Number: 3,
			Title:  "Fix y",
			State:  vcs.StateOpen,
			URL:    "https://gitlab.com/o/r/-/merge_requests/3",
			Author: "cy",
			Head:   "fix",
			Base:   "main",
			Body:   "D",
			Comments: []vcs.Comment{
				{Author: "eve", When: "2026-01-02T03:04:05Z", Body: "Why?"},
				{Author: "cy", When: "2026-01-03T03:04:05Z", Body: "Speed."},
			},
		}},
		{"https://dev.azure.com/o/p/_git/r", vcs.Hosts{}, &vcs.PullRequest{
			Forge:  vcs.ForgeAzure,
			Number: 9,
			Title:  "Z",
			State:  vcs.StateOpen,
			URL:    "https://dev.azure.com/o/p/_git/r/pullrequest/9",
			Author: "Dee",
			Head:   "feat",
			Base:   "main",
		}},
	}
	for _, tt := range tests {
		r, _ := repoAt(t, tt.remote)
		pr, err := r.PullRequest(t.Context(), tt.hosts)
		if err != nil {
			t.Errorf("PullRequest(%s) = %v, want no error", tt.remote, err)
			continue
		}
		if !reflect.DeepEqual(pr, tt.want) {
			t.Errorf("PullRequest(%s) got:\n%+v\nwant:\n%+v",
				tt.remote, pr, tt.want)
		}
	}
}

// TestPullRequestErrors checks the error for a missing remote, an unknown
// forge, a missing pull request, and a missing tool.
func TestPullRequestErrors(t *testing.T) {
	dir := t.TempDir()
	fakePath(t, dir)
	fakeTool(t, dir, "az", map[string]string{azArgs("none"): `[]`})
	const azure = "https://dev.azure.com/o/p/_git/r"
	const unsupported = "the remote is not on GitHub, GitLab, or Azure DevOps"
	corp := vcs.Hosts{GitLab: "Git.Corp"}
	tests := []struct {
		remote, branch string
		hosts          vcs.Hosts
		want           error  // if not nil, err must wrap it
		msg            string // if not empty, the text of err
	}{
		// The host selects the forge, not a name in the path.
		{remote: "https://example.com/github.com/r.git", branch: "feat", msg: unsupported},
		{remote: "git@example.com:gitlab/r.git", branch: "feat", msg: unsupported},
		{remote: "https://git.corp.example/r", branch: "feat", hosts: corp, msg: unsupported},
		{remote: "git@git.corp:o/r.git", branch: "feat", hosts: corp, msg: "glab isn't installed"},
		{remote: "ssh://git@GitHub.com:22/o/r", branch: "feat", msg: "gh isn't installed"},
		{
			remote: "https://u@org.visualstudio.com/p/_git/r",
			branch: "other",
			msg:    "unexpected call: " + azArgs("other"),
		},
		{remote: "", branch: "feat", msg: "no git remote"},
		{
			remote: "https://example.com/r.git",
			branch: "feat",
			msg:    "the remote is not on GitHub, GitLab, or Azure DevOps",
		},
		{remote: azure, branch: "none", want: vcs.ErrNoPullRequest},
		{
			remote: azure,
			branch: "other",
			msg:    "unexpected call: " + azArgs("other"),
		},
		{remote: "https://github.com/o/r", branch: "feat", msg: "gh isn't installed"},
	}
	for _, tt := range tests {
		r, dir := newRepo(t)
		if tt.remote != "" {
			git(t, dir, "remote", "add", "origin", tt.remote)
		}
		if tt.branch != "feat" {
			git(t, dir, "checkout", "-qb", tt.branch)
		}
		_, err := r.PullRequest(t.Context(), tt.hosts)
		switch {
		case err == nil:
			t.Errorf("PullRequest(%q): no error, want an error", tt.remote)
		case tt.want != nil && !errors.Is(err, tt.want):
			t.Errorf("PullRequest(%q): err = %v, want %v", tt.remote, err, tt.want)
		case tt.msg != "" && err.Error() != tt.msg:
			t.Errorf("PullRequest(%q): err = %q, want %q", tt.remote, err, tt.msg)
		}
	}
}

// TestPullRequestHostile checks that the text of a pull request has no
// control characters.
func TestPullRequestHostile(t *testing.T) {
	dir := t.TempDir()
	fakePath(t, dir)
	const osc = `\u001b]0;owned\u0007`
	fakeTool(t, dir, "gh", map[string]string{ghArgs: `{
		"number": 7, "title": "Add` + osc + `", "state": "OPEN",
		"body": "a` + osc + `\nb", "author": {"login": "ann` + osc + `"},
		"comments": [{"author": {"login": "bob"}, "body": "x` + osc + `"}],
		"statusCheckRollup": [{"name": "t` + osc + `", "conclusion": "SUCCESS"}]}`})
	r, _ := repoAt(t, "https://github.com/o/r")
	pr, err := r.PullRequest(t.Context(), vcs.Hosts{})
	if err != nil {
		t.Fatal(err)
	}
	got := []string{pr.Title, pr.Body, pr.Author, pr.Comments[0].Body, pr.Checks[0].Name}
	want := []string{"Add]0;owned", "a]0;owned\nb", "ann]0;owned", "x]0;owned", "t]0;owned"}
	if !slices.Equal(got, want) {
		t.Errorf("PullRequest texts = %q, want %q", got, want)
	}
}

// fakePath sets PATH to dir and the system directories, and links git into
// dir. Then PATH has the fake forge tools and no real ones.
func fakePath(t *testing.T, dir string) {
	t.Helper()
	p, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(p, filepath.Join(dir, "git")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":/bin:/usr/bin")
}
