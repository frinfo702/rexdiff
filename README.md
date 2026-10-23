![rexdiff in Rex]

# rexdiff

rexdiff is a diff viewer for coding agents in [Rex]. It opens in a pane next to
your agent and shows what the agent changed. You comment on the lines you don't
like and send the comments back to the agent.

The 37-second [demo] shows an agent adding a `-version` flag to rexdiff and
getting a review comment back.

## Install

```sh
go install github.com/rednafi/rexdiff/cmd/rexdiff@latest
rexdiff install
```

`rexdiff install` adds a "Toggle rexdiff" action to your Rex config and binds
it to Shift+Cmd+R. Use `--key` to pick another key. The `--placement` flag
sets how rexdiff opens: `split`, `zoomed`, `tab`, or `overlay`. Run
`rexdiff uninstall` to remove the action and the key.

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

rexdiff reads its config from `~/.config/rexdiff/config.toml`. Set
`REXDIFF_CONFIG` to use another file. The keys are the same as in
[herdr-reviewr]. Edits to the file apply right away. `rexdiff -h` lists the
flags and prints the config path.

## Requirements

- Rex and git
- `gh`, `glab`, or `az` for the PR tab, logged in

<!-- references -->

[rexdiff in Rex]: assets/banner.png
[Rex]: https://www.superlogical.com/rex
[demo]: assets/demo.mp4
[herdr-reviewr]: https://github.com/persiyanov/herdr-reviewr
