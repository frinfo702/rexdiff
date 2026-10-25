<img src="assets/logo.png" alt="rexdiff" width="360" align="right">

A diff viewer for coding agents in [Rex]. Think [herdr-reviewr], but for Rex.


It opens next to the agent and shows the changes. You comment on lines and send
the comments to the agent.

https://github.com/user-attachments/assets/e44159eb-3b02-4ea8-88ca-f6314261c1aa

## Install

With Homebrew:

```sh
brew tap rednafi/rexdiff https://github.com/rednafi/rexdiff
brew trust rednafi/rexdiff
brew install --cask rexdiff
```

With Go:

```sh
go install github.com/rednafi/rexdiff/cmd/rexdiff@latest
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
- Send: press `s`. rexdiff pastes the comments into the agent's input. It does
  not press Enter.
- Update: when rexdiff shows a new version at the bottom, press `U`. Your
  comments stay.

A long run of unchanged lines folds into one row. Three lines show beside each
change.

A comment stays on its line numbers and keeps the code it was made on. It
shows only where you made it: in the changes or in all files, and on the same
commits. If its file leaves the changes (or, for all files, is gone), the
comment list marks it (stale). The list edits only a comment on the commits
that show.

| Key | Action |
| --- | --- |
| `u` `b` `t` | uncommitted, branch, or last-turn changes |
| `G` | pick commits: `v` marks the start of a range, enter opens, esc clears or closes |
| `]` `[` | next or previous change. Past the last one, press it again to go to the next file |
| `→` or click | show the unchanged lines of a fold (`⋯ 12 unmodified lines`) |
| `f` `F` | next or previous file |
| `c` | new comment, also on a line that has one |
| `e` | edit the comment at the cursor, else open the file in the editor |
| `d` | delete the comment at the cursor |
| `l` | list the comments: `e` edit, `d` delete, `s` send, `y` copy, enter jump |
| `s` | send the comments to the agent |
| `y` | copy the comments and clear them, as a send does |
| `\|` | side by side |
| `:` | go to a line (`0` first, `$` last) |
| `m` | rendered markdown |
| `1` `2` `3` | changes, all files, or pull request |
| `esc` | clear the selection, then the armed `]` or `[`, then the key list |
| `?` | all keys |
| `q` | quit, or ask first if comments are unsent |

Side by side needs 135 columns. A narrower pane shows one column.

An edit with blank text keeps the comment as it was.

The commit picker lists the branch's own commits (first parent only), or the last 50 on
the base branch.

When `q` asks first, `Q` quits, `s` sends, `y` copies, and any other key closes the
question.

A copy uses the first of `pbcopy`, `wl-copy`, `xclip`, or `xsel` on your PATH.

## Configure

Edit the settings at the top of `~/.config/rex/rexdiff.lua`:

```lua
local settings = {
  key = "shift+cmd+r",   -- toggle key, or false for none
  placement = "split",   -- split, zoomed, tab, or overlay
  layout = "split",      -- unified or split
  editor = "code -g {file}:{line}",
  keybindings = { send = { "S" } },
}
```

Other settings:

- `default_scope`
- `markdown_view`
- `file_list`
- `file_list_width`
- `diff_signs`: `true` marks changed lines with `+` and `-`, in place of a coloured bar
- `review_prefix`
- `url_opener`
- `focus_agent_after_send`
- `github_host`
- `gitlab_host`
- `azure_devops_host`

See [herdr-reviewr] for the values and the action names.

A key in `keybindings` is one character, or `left`, `right`, `up`, `down`, `pageup`, or
`pagedown`, alone or after `ctrl+` or `alt+`. A key bound to two actions is an error that
names both.

`e` opens the file at the cursor line in `editor`, else in `$VISUAL`, `$EDITOR`, or git's
`core.editor`. If none is set, rexdiff shows "no editor" and opens nothing. A terminal editor, such as `nvim`, `micro`, or `fresh`, opens in a new pane
beside rexdiff. A window editor, such as `code`, opens in its window.
rexdiff opens an editor that it does not know in a pane. To open such an editor in a window,
start it with `open`, for example `editor = "open -a Zed {file}"`.

rexdiff gets `$VISUAL` and `$EDITOR` from your login shell. With zsh, set them in
`~/.zprofile` or `~/.zshenv`, not in `~/.zshrc`.

After a change:

- Run `rex config reload`.
- Do not edit the line `-- rexdiff code below` or the code under it.

## Requirements

- Rex
- git 2.31 or later
- `gh`, `glab`, or `az` for the pull request tab. You must be logged in.

<!-- references -->

[Rex]: https://www.superlogical.com/rex
[herdr-reviewr]: https://github.com/persiyanov/herdr-reviewr
