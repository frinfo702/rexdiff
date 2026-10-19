package main

// The install command adds a Rex action, "Toggle rexdiff", to the command
// palette and binds it to a key. The action opens rexdiff beside the agent
// in focus. If rexdiff runs in the session, the action closes each block
// that runs it instead. It finds these blocks by the program status that
// rexdiff reports. A Rex session is the same as a herdr workspace.

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/rednafi/rexdiff/rex"
)

// placement tells where the toggle action opens rexdiff.
type placement string

const (
	placementSplit   placement = "split"   // beside the agent
	placementZoomed  placement = "zoomed"  // beside the agent, zoomed
	placementTab     placement = "tab"     // in a new tab
	placementOverlay placement = "overlay" // in a layer above the window
)

// placements are the values that --placement accepts.
var placements = []placement{
	placementSplit,
	placementZoomed,
	placementTab,
	placementOverlay,
}

// luaTemplate is the code of rexdiff.lua. Its verbs are the path of
// rexdiff, the placement, and the key.
const luaTemplate = `-- Installed by "rexdiff install". Remove with "rexdiff uninstall".
local BIN = %q
local PLACEMENT = %q -- split | zoomed | tab | overlay
local PLACEMENTS = { split = true, zoomed = true, tab = true, overlay = true }
local TERM = "com.superlogical.terminal"

-- must returns res, or fails the action with err. A Rex call returns nil
-- and an error message when it fails.
local function must(res, err)
  if err ~= nil then error(err, 0) end
  return res
end

-- call calls method on the terminal block id.
local function call(method, id)
  return must(rex.block.call(TERM, method, { block_id = id }))
end

-- runs_rexdiff tells if the terminal block id runs rexdiff. rexdiff
-- reports app=rexdiff to Rex with OSC 7501. Rex keeps an idle status after
-- its program stops, so the block counts only while the process that
-- reported the status is in the foreground.
local function runs_rexdiff(id)
  for _, r in ipairs(call("program_status", id).records or {}) do
    if r.app == "rexdiff" and r.owner then
      local fg = call("process", id).foreground
      return fg ~= nil and fg.pid == r.owner.pid
    end
  end
  return false
end

-- reviewers returns the IDs of the blocks in the session that run rexdiff.
local function reviewers()
  local ids = {}
  for _, b in ipairs(must(rex.session.list_blocks{}).blocks or {}) do
    if b.creator_name == TERM and runs_rexdiff(b.block_id) then
      table.insert(ids, b.block_id)
    end
  end
  return ids
end

-- open_rexdiff opens rexdiff at placement, beside the agent in ctx.
-- rexdiff opens in the directory of the agent, as the rex package of
-- rexdiff finds it.
local function open_rexdiff(ctx, placement)
  local p = call("process", ctx.block_id)
  local cwd = (p.foreground or {}).cwd or (p.child or {}).cwd
  local node = rex.layout.block{
    flavor = TERM .. ".shell",
    label = "rexdiff",
    options = {
      command = { BIN, "--agent", ctx.block_id },
      cwd = cwd,
      shell = "none",
    },
  }
  if placement == "tab" then
    return must(rex.session.new_window{ window_label = "rexdiff", layout = node })
  elseif placement == "overlay" then
    return must(rex.session.new_layer{
      bounds = { 0.04, 0.04, 0.92, 0.92 },
      layout = node,
      focus = true,
    })
  end
  local s = must(rex.session.new_split{
    anchor_id = ctx.block_id,
    direction = "horizontal",
    side = "after",
    ratio = 0.45,
    focus = true,
    layout = node,
  })
  if placement == "zoomed" then
    must(rex.session.zoom_block{ block_id = s.block_ids[1], zoom = true })
  end
  return s
end

rex.action{
  name = "rexdiff_toggle",
  title = "Toggle rexdiff",
  description = "Review the agent's changes beside it and send comments back",
  category = "rexdiff",
  keywords = "diff review git comments agent",
  args = { placement = "string?" }, -- split, zoomed, tab, or overlay
  run = function(ctx, args)
    local placement = args.placement or PLACEMENT
    if not PLACEMENTS[placement] then
      error("unknown placement " .. placement
        .. "; use split, zoomed, tab, or overlay", 0)
    end
    local open = reviewers()
    if #open == 0 then
      return open_rexdiff(ctx, placement)
    end
    for _, id in ipairs(open) do
      must(rex.call("block.close", { session_id = ctx.session_id, block_id = id }))
    end
    return { closed = open }
  end,
}

rex.bind(%q, "rexdiff_toggle")
`

// require is the code that loads rexdiff.lua. A line of init.lua that has
// it belongs to rexdiff.
const (
	require     = `require("rexdiff")`
	requireLine = require + ` -- rexdiff toggle; see rexdiff.lua`
	luaFile     = "rexdiff.lua" // the file of the action, beside init.lua
)

// executable returns the path of the rexdiff binary, for the Rex action.
func executable() (string, error) {
	bin, err := os.Executable()
	if err != nil {
		return "", err
	}
	if b, err := filepath.EvalSymlinks(bin); err == nil {
		bin = b
	}
	// go run deletes its binary at exit.
	if strings.Contains(bin, string(filepath.Separator)+"go-build") {
		return "", errors.New("install the binary first, for example with go install, " +
			"then run rexdiff install")
	}
	return bin, nil
}

// install writes the action that runs bin, the path of rexdiff.
func install(
	ctx context.Context,
	bin string,
	args []string,
	stdout, stderr io.Writer,
) error {
	fs := flag.NewFlagSet("rexdiff install", flag.ContinueOnError)
	fs.SetOutput(stderr)
	keyName := fs.String("key", "shift+cmd+r", "the key that toggles rexdiff")
	placementFlag := fs.String("placement", string(placementSplit),
		"where rexdiff opens: split, zoomed, tab, or overlay")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() > 0 {
		const use = "--key=KEY --placement=split|zoomed|tab|overlay"
		return fmt.Errorf("unknown argument %s (use %s)", fs.Arg(0), use)
	}
	placement := placement(*placementFlag)
	if !slices.Contains(placements, placement) {
		return fmt.Errorf("unknown placement %q (use split, zoomed, tab, or overlay)",
			placement)
	}

	initPath := rex.ConfigPath()
	dir := filepath.Dir(initPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	lua := filepath.Join(dir, luaFile)
	code := fmt.Appendf(nil, luaTemplate, bin, placement, *keyName)
	if err := os.WriteFile(lua, code, 0o644); err != nil {
		return err
	}
	if err := addRequire(initPath); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "wrote %s\nadded require(\"rexdiff\") to %s\n", lua, initPath)
	fmt.Fprint(stdout, reload(ctx, stdout))
	fmt.Fprintf(stdout,
		"press %s, or run \"Toggle rexdiff\" from the command palette, beside an agent\n",
		*keyName)
	return nil
}

// addRequire adds the require line to the end of the init.lua at path. It
// creates the file if it is not there. If the file has a require of
// rexdiff, addRequire does not change it.
func addRequire(path string) error {
	b, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	s := string(b)
	if strings.Contains(s, require) {
		return nil
	}
	if s != "" && !strings.HasSuffix(s, "\n") {
		s += "\n"
	}
	return os.WriteFile(path, []byte(s+requireLine+"\n"), 0o644)
}

// uninstall removes the require line from init.lua, then rexdiff.lua. In
// this order, a failure does not leave init.lua with a require of a file
// that is not there.
func uninstall(ctx context.Context, stdout io.Writer) error {
	initPath := rex.ConfigPath()
	if err := removeRequire(initPath); err != nil {
		return err
	}
	lua := filepath.Join(filepath.Dir(initPath), luaFile)
	if err := os.Remove(lua); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	fmt.Fprintln(stdout, "removed the rexdiff action from Rex")
	reload(ctx, stdout)
	return nil
}

// removeRequire removes each line that has a require of rexdiff from the
// init.lua at path. If the file is not there, it does nothing.
func removeRequire(path string) error {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	lines := slices.DeleteFunc(strings.Split(string(b), "\n"), func(l string) bool {
		return strings.Contains(l, require)
	})
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644)
}

// reload tells Rex to read its configuration file again, and returns the
// report of Rex. If Rex fails, reload tells the user to reload by hand and
// returns "".
func reload(ctx context.Context, stdout io.Writer) string {
	out, err := rex.ReloadConfig(ctx)
	if err != nil {
		fmt.Fprintf(stdout, "reload Rex's config yourself: rex config reload (%v)\n", err)
		return ""
	}
	return out
}
