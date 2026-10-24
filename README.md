![rexdiff in Rex]

# rexdiff

rexdiff is a diff viewer for coding agents in [Rex]. It opens in a pane next to
your agent and shows what the agent changed. You comment on the lines you don't
like and send the comments back to the agent.

The 37-second [demo] shows an agent adding a `-version` flag to rexdiff and
getting a review comment back.

## Install

With Homebrew:

```sh
brew tap rednafi/rexdiff https://github.com/rednafi/rexdiff
brew trust rednafi/rexdiff
brew install --cask rexdiff
```

Or with Go:

```sh
go install github.com/rednafi/rexdiff/cmd/rexdiff@latest
```

Then add rexdiff to Rex:

```sh
rexdiff install
```

This adds a "Toggle rexdiff" action to your Rex config and binds it to
Shift+Cmd+R. Run `rexdiff uninstall` to remove the action and the key.

## Use

Press Shift+Cmd+R next to an agent, or run "Toggle rexdiff" from the command
palette. rexdiff opens in the agent's folder. Press the key again to close it.

You start on the uncommitted changes. Press `b` to see the whole branch, or `t`
to see only what the agent did in its last turn.

To comment, move to a line and press `c`. For a range, press `v` and move, or
drag over the line numbers. Press `s` when you're done. rexdiff pastes all the
comments into the agent's input but doesn't press Enter, so you can still edit
them.

A comment moves with its lines when the agent edits the file. If the lines are
gone, rexdiff marks the comment as stale.

When a new version is out, rexdiff says so at the bottom. Press `U`, or click
the message, to update. rexdiff installs the new version in the background, the
same way you installed it, and then starts it in the same pane. Your comments,
the file, and the cursor stay.

The PR tab shows the pull request description, the CI checks, and the review
comments. It needs `gh`, `glab`, or `az`.

| Key | Action |
| --- | --- |
| `u` `b` `t` | uncommitted, branch, or last-turn changes |
| `]` `[` | next or previous change |
| `f` `F` | next or previous file |
| `c` | comment |
| `s` | send the comments to the agent |
| `y` | copy the comments |
| `\|` | side by side |
| `1` `2` `3` | changes, all files, or the pull request |
| `?` | all keys |

## Configure

The settings are at the top of `~/.config/rex/rexdiff.lua`, the file that
`rexdiff install` writes:

```lua
local settings = {
  key = "shift+cmd+r",   -- the key of the toggle, or false for none
  placement = "split",   -- split, zoomed, tab, or overlay
  layout = "split",      -- unified or split
  editor = "code -g {file}:{line}",
  keybindings = { send = { "S" } },
}
```

The other settings are `default_scope`, `markdown_view`, `file_list`,
`file_list_width`, `review_prefix`, `url_opener`, `focus_agent_after_send`,
`github_host`, `gitlab_host`, and `azure_devops_host`. They and the names of
the actions in `keybindings` are the same as in [herdr-reviewr], but there is
no `theme`: rexdiff takes its colours from the Rex theme.

Run `rex config reload` after a change, and toggle rexdiff again. The toggle
reports a bad setting instead of opening rexdiff.

The rexdiff code is under the line `-- rexdiff code below`. `rexdiff install`
and upgrades replace that code and keep your settings. If the line is gone,
rexdiff cannot tell your settings from its code, so it does not upgrade. It
tells you so, in `rexdiff install` and at the bottom of rexdiff.

## Requirements

- Rex and git
- `gh`, `glab`, or `az` for the PR tab, logged in

<!-- references -->

[rexdiff in Rex]: assets/banner.png
[Rex]: https://www.superlogical.com/rex
[demo]: assets/demo.mp4
[herdr-reviewr]: https://github.com/persiyanov/herdr-reviewr
