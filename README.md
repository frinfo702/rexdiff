<img src="assets/logo.png" alt="rexdiff" width="360" align="right">

A diff viewer for coding agents in [Rex]. Think [herdr-reviewr], but for Rex.


It opens next to the agent and shows the changes. You comment on lines and send
the comments to the agent.

https://github.com/user-attachments/assets/e44159eb-3b02-4ea8-88ca-f6314261c1aa

## Install

With Homebrew:

```sh
brew tap frinfo702/rexdiff https://github.com/frinfo702/rexdiff
brew trust frinfo702/rexdiff
brew install --cask rexdiff
```

With Go:

```sh
go install github.com/frinfo702/rexdiff/cmd/rexdiff@latest
```

Add rexdiff to Rex:

```sh
rexdiff install
```

This adds the "Toggle rexdiff" action and binds it to Shift+Cmd+R. To remove
them, run `rexdiff uninstall`.

## Use

- Open: press Shift+Cmd+R next to an agent, or run "Toggle rexdiff" from the
  command palette. Press it again to close rexdiff.
- Comment: move to a line and press `c`. For a range, press `v` or drag over
  the line numbers.
- Copy: drag over the code to select it. Release copies the selection. `esc`
  clears it. It works in each tab.
- Send: press `s`. rexdiff pastes the comments into the agent's input. It does
  not press Enter.
- Update: when rexdiff shows a new version at the bottom, press `U`. Your
  comments stay.

When the agent edits a file, the comments move with their lines. If the lines
are gone, rexdiff marks the comment as stale.

| Key | Action |
| --- | --- |
| `u` `b` `t` | uncommitted, branch, or last-turn changes |
| `]` `[` | next or previous change |
| `f` `F` | next or previous file |
| `c` | comment |
| `s` | send the comments to the agent |
| `y` | copy the comments |
| `\|` | side by side or top to bottom |
| `1` … `5` | overview, guide, diff, all files, or pull request |
| `r` | in the guide: write it again |
| `?` | all keys |

## Tabs

- **Overview**: the branch and its base, commits ahead and behind, the state
  against the upstream, and the size of the changes. It reads git only, so it
  works with no forge and no login.
- **Guide**: the changes in the order of their meaning, as on [pulls.review].
  An agent groups the files into sections with a title and a summary; the diff
  of each file shows beside its section. rexdiff runs the first of `claude -p`,
  `pi -p`, and `codex exec -` that is installed, or `guide_agent`. It keeps
  each guide, so the agent writes it once. Press `r` to write it again.
- **Diff**: the changes, one file at a time. A wide pane shows them side by
  side; a narrow one, top to bottom. Binary files are not diffed.

## Configure

Edit the settings at the top of `~/.config/rex/rexdiff.lua`:

```lua
local settings = {
  key = "shift+cmd+r",   -- toggle key, or false for none
  placement = "split",   -- split, zoomed, tab, or overlay
  layout = "split",      -- split (the default) or unified
  editor = "code -g {file}:{line}",
  keybindings = { send = { "S" } },
}
```

Other settings:

- `default_scope`
- `markdown_view`
- `file_list`
- `file_list_width`
- `guide_agent`: the command that writes the guide. It reads the prompt on its
  standard input, for example `"claude -p --model sonnet"`
- `diff_signs`: `true` marks changed lines with `+` and `-`, in place of a coloured bar
- `review_prefix`
- `url_opener`
- `focus_agent_after_send`
- `github_host`
- `gitlab_host`
- `azure_devops_host`

See [herdr-reviewr] for the values and the action names.

`e` opens the file at the cursor line in `editor`, else in `$VISUAL`, `$EDITOR`, or git's
`core.editor`. A terminal editor, such as `nvim`, `micro`, or `fresh`, opens in a new pane
beside rexdiff. A window editor, such as `code`, opens in its window.

After a change:

- Run `rex config reload`.
- Do not edit the line `-- rexdiff code below` or the code under it.

## Requirements

- Rex
- git
- `gh`, `glab`, or `az` for the pull request tab. You must be logged in.

<!-- references -->

[Rex]: https://www.superlogical.com/rex
[herdr-reviewr]: https://github.com/persiyanov/herdr-reviewr
[pulls.review]: https://pulls.review/
