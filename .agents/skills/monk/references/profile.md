# Research: Alasdair Monk (almonk), work-style profile

Researched 2026-10-25. Sources: GitHub API (`gh`), alasdairmonk.com, superlogical.com, Amplify Partners post,
a16z podcast page, Full Stack Radio #128 show notes, Vercel blog, HN (Algolia API), and the Rex Beta Discord
via the superlogical MCP (feedback + changelog).
Tags used below: **[STATED]** = his own words or official bio; **[OBSERVED]** = seen directly in his code/commits/posts;
**[INFERRED]** = my reading of patterns, not confirmed.

---

## 1. Identity & career

Identity confirmed: GitHub `almonk` (id 51724, joined 2009-02) → profile blog `https://alasdairmonk.com`, bio "Software Designer",
company field `@poolsideai` (stale; he has since moved). On Discord he appears as `almonk (@alasdairmonk)`. X handle `@almonk`.
Called "Al" by Mitchell Hashimoto and team in Discord.

Career (from https://alasdairmonk.com, [STATED]):
- **Superlogical**: Co-founder (2026 to now). Building "a multiplexer for all work". The first product is **Rex**, a terminal emulator and multiplexer built on libghostty.
  Co-founders: Mitchell Hashimoto (Ghostty, HashiCorp), Jack Pearkes (first HashiCorp employee), Hector Simpson (designer, `dizzyup`).
  (https://www.superlogical.com/, https://www.amplifypartners.com/blog-posts/announcing-our-investment-in-superlogical)
- **poolside**: Founding Designer (2023 to 2026). superlogical.com calls him "Head of Experience at Poolside".
- **Vercel**: VP of Design (2022 to 2023). Led Product Design, Creative Design and Design Engineering.
- **HashiCorp**: Principal Designer (2021 to 2022). Shipped Waypoint UI PRs in Ember (see section 5).
- **Heroku**: UX Architect (2015 to 2021). Built Heroku's utility CSS approach (purple3 for product UI, shibori3 for marketing).
  **Correction to the brief:** he was *UX Architect* at Heroku, not VP/Head of Design. "VP of Design" was at Vercel.
- **GoCardless**: Head of Design (2013 to 2015).
- Earlier (2011 to 2013, from gists): ProcessWire/Craft CMS modules, PHP/Twig, Rails. He has been a hands-on web developer since at least 2011.

Side work [STATED]:
- **Replay Software** (https://replay.software): "A tiny studio making delightful apps for Mac & iOS", co-run with Hector Simpson.
  Apps: **Sleeve** ("The ultimate music accessory for your Mac"), **Echo** ("Beautiful, pocketable SSH", iOS SSH client),
  **Umbra** ("The missing app for Dark Mode").
- Angel investor in dev tools: Caligra, Paper, Incident.io. "actively investing in new dev tools".
- superlogical.com: "two decades designing developer interfaces at Vercel, HashiCorp, and Heroku."

Relationship to Ghostty and Mitchell [OBSERVED]: he was one of the earliest Ghostty testers. He filed ghostty-org/ghostty issues #3, #4, #5 in Aug 2022
(the private beta), and #51 and #53 in Nov 2022. He also has a `ghostty-color-converter` repo (2022). Mitchell on Discord called a Rex UI element
"<@almonk>'s mona lisa". Rex's split library "Supersplit" was "born from bonsplit" (Mitchell), and Al described it as "<Hector>'s baby".

---

## 2. Core beliefs (direct quotes + sources)

On AI interfaces (a16z podcast "Developer Tool UX in the Age of Generative AI", June 2024,
https://a16z.com/podcast/developer-tool-ux-in-the-age-of-generative-ai/) [STATED]:
- "When interacting with machine learning models, we've almost thrown away 30 years of human-computer interaction knowledge"
  and "reverted to using a terminal circa 1980 to interact with computer, or the prompt bar."
- "This very plain-text way to interact with AI is really interesting."
- "I think it's very different when you can't predict what a user interface is going to look like." ... "design around the guardrails for that are the really interesting things"
- "sometimes you want to chat to the computer as if it's the rubber duck."
- "a lot of where AI is going to really help us, particularly with engineering, will actually probably look much more like interacting with traditional software today ... where I interact with it via windows and buttons and all sorts of GUI elements."
  → Belief: chat is not the end state. Good AI tooling goes back to real GUI affordances.

On iteration speed (Vercel blog "Iterating from design to deploy", Jul 2023, co-authored with Alice Alexandra Moore,
https://vercel.com/blog/iterating-from-design-to-deploy) [STATED, co-authored]:
- "The faster your team can fail and learn from failure, the faster you can have a truly great product."
- Teams can now work "design first, engineering second". Previews on every commit let non-engineers touch real builds.

On CSS (Full Stack Radio #128, "Scaling CSS at Heroku with Utility Classes", Nov 2019, https://fullstackradio.com/128) [STATED in show notes; no transcript found]:
- Heroku tried BEM first and it didn't work. Utility CSS was the change that fixed it. Non-utility CSS kept needing new styles for each feature.
  Utility classes worked for both the app and marketing sites. The team was still happy with it after about three years.

Company-level (Superlogical; shared, not his alone) [STATED]:
- Mitchell: "One thing we all share is an uncompromising commitment to well-crafted software and software experiences." (https://mitchellh.com/writing/superlogical)
- superlogical.com: "We believe the missing layer is a durable session around the work itself."
- Amplify: "If you get the primitive right, it becomes the building block that everyone else standardizes around."

His own product design calls (Rex Discord, [STATED]):
- On the empty "no session" screen: "I want to make it so that user's shouldnt ever see this screen". He would rather remove an empty state than polish it.
  (https://discord.com/channels/1546797889009881158/1551597790495510540)
- On the update notification floating outside the window: "i decided to float it outside the window so it doesnt cover anything in the terminal" ...
  "yeah its certainly a bit novel" ... "i based it on how raycast used to handle the same scenario which i always really liked"
  (https://discord.com/channels/1546797889009881158/1549105234071134259)
- On tabs dragging past the "+" button: "let the tabs be free" ... "it's not really intentional but i also dont hate it"
  (https://discord.com/channels/1546797889009881158/1549538606845263944)

Principles implied by the code [INFERRED, well supported]:
- **Native feel and motion quality are features.** "120fps animations" is a headline feature of Bonsplit. Animation timings live in named constants
  (`splitAnimationDuration = 0.15 // fast and snappy like Hyprland`).
- **Inherit the user's environment.** Bontree defaults to ANSI colors so it adapts to the terminal, and it reads Ghostty-compatible themes and config syntax.
- **Small, sharp tools that pair with others.** sesh = agent + lazygit in Zellij. bontree is "A file explorer to pair with your favourite agent".
  ogpk is one job. width has "Zero dependencies."
- **Ship the website too.** Each serious tool gets a polished landing page with a live demo (Bonsplit Next.js site with an animated logo easter egg; bontree WASM-in-xterm.js demo).
  On HN someone asked why such an elaborate site for a library. Others answered "Why make good things?" (https://news.ycombinator.com/item?id=46753301)

---

## 3. Design approach

- **Designer who codes, end to end** [OBSERVED]. He designs in the medium: Swift/SwiftUI, Go TUIs, Svelte, Next.js, CSS. Hardly any Figma artifacts are public.
  The exceptions are a figma-title-plugin (2022) and the mock image in his Ghostty feature request.
- **Declarative layout primitives** [OBSERVED]. Pylon (2019, 482★) wraps Flexbox in SwiftUI-like HTML tags: `<hstack spacing=s>`, `<vstack align-x=center>`, `<spacer>`, `<list>`, `<text>`.
  Configured through `tokens.css`. "Only 2kb". "it's just HTML & CSS". He deprecated `alignment`/`justify` "in favour of contextually aware `align-x` and `align-y`" (pylon PR #4).
- **Atomic/utility CSS since about 2016** [OBSERVED]. `yank` (2017, Go) "lets you treat standard CSS classes like @mixins" from tachyons. A gist from 2017 is "Auto specific spacing class".
  Heroku purple3/shibori3. Recent web work uses Tailwind v4 with CSS custom properties as tokens (`--background`, `--foreground`, `@theme inline`).
- **Design tokens as named constants** [OBSERVED]. `TabBarMetrics` enum (Bonsplit), grouped with `// MARK:` sections: bar height 33, tab 32, min/max width 140/220,
  icon 14, title font 12, content spacing 6, selection 0.15s, close 0.2s, reorder 0.3s with 0.15 bounce, hover 0.1s. Doc comment: "following macOS HIG".
- **Semantic color schemes** [OBSERVED]. bontree `colorScheme` struct (blue/green/red/…, fg/fgDim/comment/gutter, bg/selection). The default uses ANSI indices and `AdaptiveColor{Light, Dark}`.
  A theme maps palette indices to semantics.
- **Micro-iteration on visual details** [OBSERVED in commits]. bontree in one day: "FILTER purple, FLAT orange" → "Both modes labelled FILTER, purple for tree, orange for flat",
  "Softer default text color (ANSI 7 instead of 15)", "add thin space between tree connector and icon", "Place folder path at 50% column in flat search mode",
  "Change 'Copied' to 'Copied path'", "Powerline-style status bar with mode segment and chevrons".
- **Copy matters** [OBSERVED]. Commits like "Wording", "Update copy", "Simplify copy", "docs copy". At HashiCorp he aligned all English UI strings with design-system copy guidelines
  and began to "codify other guidelines (such as respecting brand name capitalisation e.g. `Git` not `git`)" (hashicorp/waypoint#1600).
- **Before/after screenshots and type rhythm** [OBSERVED]. waypoint#2036: "fix alignment and type rhythm of the `status-row` ... makes the `status-indicator` text the same size as the supporting text ...
  moves the status text next to the indicator to avoid uneven whitespace." Before/After images follow.
- **Accessibility awareness** [OBSERVED]. waypoint#1916: "Follows a11y guidelines of having most unique information at the beginning of the page title."
- **Avoid jarring experiences** [STATED]. ghostty#2001 (feature request): buffer output while selecting, then "the scroll position does not change so as not to create a jarring experience."
- **Steals good patterns from apps he admires** [STATED/OBSERVED]: Raycast (update notification), Hyprland (snappy split animation), macOS HIG and native tabs,
  Ghostty (config syntax, themes), Sublime/Hyprland-style splits. He copies proven mental models instead of inventing config formats.

---

## 4. Coding style (concrete)

Languages over time [OBSERVED from repos]: PHP/Twig (2011) → Ruby/Rails, SCSS, Ember (Heroku 2013–2021) → Swift/SwiftUI macOS apps (2016–now: keeper, Quiplet,
aspidistra, timer, ports, bonsplit) → Go CLIs and TUIs (yank 2017, nxo 2022, ogpk 2023, bontree/tui-pets/width 2026) → Svelte/SvelteKit (pickthedate, quilter,
bontree www, pooleval) and Next.js/React (bonsplit www) → shell (fish-first, then zsh/bash). He contributed a Zig fork of Ghostty and a Python fork of Toad.

Go style (bontree, width, tui-pets) [OBSERVED]:
- Flat, conventional layout: `main.go`, `config/`, `theme/`, `tree/`, `ui/`, `icons/`, `cmd/wasm/`. tui-pets uses `cmd/` plus `internal/{kitty,pets,tui}`.
  He splits big files when they grow ("Split ui/model.go into logical modules").
- Plain `main` with manual `os.Args` parsing (no cobra). Early-exit validation with friendly errors:
  ```go
  if !info.IsDir() {
      fmt.Fprintf(os.Stderr, "Error: %s is not a directory\n", path)
      os.Exit(1)
  }
  ```
- Every function has a short doc comment in sentence form: `// clampCursor ensures cursor is within bounds.`,
  `// refreshTree rebuilds the file tree from disk, preserving expanded state and cursor.` Sections are marked `// --- Helpers ---`.
  Struct fields are grouped under `// Mouse`, `// Search`, `// Saved state before search`, with trailing comments such as `// true = flat file search (ctrl+f), false = hierarchy search (/)`.
- Ships tests next to the code (`config_test.go`, `theme_test.go`, `width_test.go` plus the official Unicode conformance suite in `testdata/`, run in CI).
- Zero or few deps. Uses Charm's Bubble Tea and Lip Gloss for TUIs. `width` has none and generates its tables (`//go:generate go run ./gen`), so
  "Bumping Unicode versions is a one-line change".
- Package doc comments are honest about limits: "A caveat that applies to every width library: the terminal is the final arbiter."
- A Makefile with `build`/`install`, a GitHub Actions release workflow (manual trigger), a `-v/--version` flag set via ldflags, and `mise.toml`.

Swift style (bonsplit, ports) [OBSERVED]:
- `Sources/Bonsplit/Public/` vs `Internal/{Controllers,Models,Styling,Utilities,Views}`. The public API surface is kept small and explicit (`BonsplitController`, `BonsplitDelegate`,
  `BonsplitConfiguration`, ID types `TabID`/`PaneID`). There is an Example app with a Debug window.
- API ergonomics: optional labeled params with defaults (`createTab(title:icon:isDirty:inPane:)`). Uses delegate callbacks plus configuration presets (`default`, `singlePane`, `readOnly`).
  Behavior is opt-in ("Splits create empty panes by default, giving you full control").
- Uses SF Symbols, system colors, `CADisplayLink` for 120fps.
- ports/agents.md notes: "Uses modern SwiftUI with computed properties to avoid compiler expression complexity limits", "smooth port list updates that maintain order and minimize UI flicker",
  "All process operations run on background queues to avoid blocking UI".

Web style [OBSERVED]:
- Next.js 16 / React 19 / Tailwind v4 / Base UI / anime.js / framer-motion / Geist fonts / dark `#0a0a0a` bg with a custom `::selection` color (bonsplit www).
- SvelteKit with hand-written CSS variables (Tokyo Night palette, `--radius: 8px`) and Maple Mono NF self-hosted (bontree www).
- Svelte 5 component library with theme-token props (`theme={{ 'bar-radius': '3px', ... }}`, `darkTheme`, CSS vars `--pe-*`) for pooleval at poolside.
- Uses pnpm. Sometimes lockfiles of both npm and pnpm get committed (sloppy-but-fast, [INFERRED]).

Shell style (sesh) [OBSERVED]: fish first (`functions/sesh.fish` + completions), POSIX sh port for bash/zsh. It checks dependencies up front and prints errors that say how to fix them:
`echo "sesh: 'claude' is not installed. Install with: npm install -g @anthropic-ai/claude-code"`. Messages are prefixed with the tool name.

AI-agent-native workflow [OBSERVED]:
- `ports` has `agents.md` (renamed from CLAUDE.md), with a "Make agent" commit. bonsplit www vendors a Vercel `web-design-guidelines` Claude skill.
- `width` commit trailer: `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`. Long, structured commit bodies on bontree/bonsplit read like agent output [INFERRED].
- sesh README has a "Prompt" install method: "Paste this into your AI coding agent and it will handle the rest". sesh launches `claude --dangerously-skip-permissions` by default.
  It supports Claude Code, Codex, Amp and Pi.
- Contributions upstream to agent TUIs: opencode#1929, toad#97.

---

## 5. Commit / PR / changelog writing style (verbatim)

### Commit subjects [OBSERVED]
- Imperative, Sentence case, no trailing period, mostly 2 to 8 words. Very granular (dozens per day on bontree/bonsplit/pylon). No emoji.
- Typical: "Add mouse support, flat file search, and git file status coloring", "Fix tree connector alignment with folder icons",
  "Escape with empty search goes straight to normal mode", "Make keybinds additive instead of replacing all defaults", "Reduce startup sleep to 50ms" → "Reduce startup sleep to 1ms",
  "Rename last to pickup", "Remove yazi integration", "Add note distinguishing from joshmedeski/sesh", "Emphasise remote and multi-device workflow" (British spelling).
- Lazy/WIP subjects are common on design-heavy or solo work: "First", "Latest" (many in a row), "L", "Bye", "Bump", "Bump v", "New build", "Mobile fix",
  "Fix more mob issues", "Vanity rename", "Remove dsstores", "No dsstore", "Bunch o improvements", "Better ux", "Guard", "Copy", "tweak shadow", "Updates".
- He occasionally uses conventional-commit prefixes, probably when an agent writes them [INFERRED]: "feat: add interactive website with WASM demo",
  "fix: support 'space' as named key in config keybinds", "refactor: eliminate WASM duplication, fix search mode input", "style: use JetBrains Darcula theme, remove terminal chrome".
- Version bumps go in as bare commits: "0.1.3", "1.0.1", "Prep 0.1.2".
- Branches: `am/www`, `am-new-aligns`, `port-to-go` (initials prefix).
- A long body example (bontree, likely agent-assisted):
  ```
  refactor: eliminate WASM duplication, fix search mode input

  Extract shared input handling into ui/key.go so both the Bubble Tea
  TUI and the WASM bridge use the same HandleKey logic. This fixes h/j
  (and other printable keys) being treated as movement commands instead
  of typable characters in search mode.

  - Add ui/key.go: single source of truth for HandleKey/search/normal input
  - Reduce cmd/wasm/main.go from ~600 to ~220 lines
  ```

### PR descriptions [OBSERVED, verbatim]
- Short, plain, often 1 to 3 sentences, with screenshots doing the work.
- pylon#4: "Deprecates the alignment and justify attributes in favour of contextually aware `align-x` and `align-y` attributes. So now we can do [code] This PR also... * Removes default spacing from lists * Adds the `vstack[stretch]` attribute ... * Updates documentation"
- opencode#1929: "This PR reverses a recent commit which prefers to only show the title of the tool call when in `run` mode. The total tool information is very useful in this headless mode and I would like to see it restored."
- toad#97: "Added Alt/Option+Delete and Alt/Option+Backspace keyboard shortcuts for word-by-word deletion in the prompt input field, matching standard macOS text field behavior."
- waypoint#1916: "Addresses #1853. Adds `ember-page-title` and names all our major routes. Follows a11y guidelines ... Some examples;" followed by screenshots.
- Some PRs have an empty body (nxo#1 "Port to go", wterm#90).
- Issue/feature request format (ghostty#2001): `**Problem:**` written personally ("It always bugs me when i start copying something out the terminal but some ongoing process keeps outputting newlines AS i'm trying to select something.")
  → `**Proposed solution:**` with concrete interaction details, plus a mock image ("Frame 8", from Figma).
- Bug reports: one-line repro and the workaround ("If I leave a newline at the end of the Ghostty `config` file, Ghostty will refuse to open. Removing the newline fixes the issue.").

### Changelogs [OBSERVED]
- Bonsplit CHANGELOG.md follows Keep a Changelog plus SemVer. It has `### Added` / `### Fixed` and bold feature names with an explanation ("**Geometry Query**: Query pane layout with pixel coordinates for integration with external programs").
  It lists API methods in backticks with a one-line explanation each.
- The website mirrors the changelog as short noun phrases ("Two-Way Synchronization API", "120fps animations").
- Rex changelog posts (team-written; authorship not attributed to him): bold lead sentence per highlight, user-facing past/present tense, concrete effects ("This results in a very snappy feeling UI"),
  and "This requires a server restart." notes. Sections: Highlights / macOS App / `rex` CLI / Rex Server / Web.

### READMEs [OBSERVED]
- One-line tagline first ("A file explorer to pair with your favourite agent.", "Eval charts. For Svelte.", "Declarative layout primitives for CSS & HTML").
- Then the why/use-case paragraph, a video or screenshot near the top, install (brew/go install/curl/prompt), a usage block, then keybinding and config tables.
- pooleval voice: "A small, opinionated charting component for showing model evals. Drop it in. Customize the colors. Ship." / "Numbers count up, bars rise from the floor."
  Short punchy fragments, and examples headed "With units. Without animation."
- He heads off confusion early: "> **Note:** This project is not related to joshmedeski/sesh".

---

## 6. Prose / voice

- **Chat (Discord)** [OBSERVED]: all lowercase, terse, friendly, apostrophes often dropped ("doesnt", "dont", "user's shouldnt"), small typos left in.
  Examples: "yup agree" / "we're on it"; "tell me more"; "interesting, will investigate"; "ohh wow" / "i see"; "hello".
  He is candid when he doesn't know something: "(i dont know how font variation works, it might like just redraw the font itself regardless of prescribed weights)".
  He pushes back lightly with a question: "how come you don't just use a less wide font?"
- **Structured triage replies** [OBSERVED]: he quotes the user's points one by one and answers each in a sentence or two. He separates what already exists ("This should be possible with the existing Rex API."),
  what's coming ("i want this too. We're building towards a smart and synchronised notification system with Activity Mode, but it's not there yet."),
  what's unclear ("Can you elaborate? Don't understand this one.") and what he likes ("I like the idea a lot"). He opens with a framing line: "Lotta stuff in here that breaks down to some distinct features I think."
  (https://discord.com/channels/1546797889009881158/1561804281261203456)
- **Defends design choices with intent plus a reference**, not authority. He concedes novelty without being defensive ("yeah its certainly a bit novel").
- **Technical first reply when he thinks it's user error**: "I think this is just your shell startup time coinciding with the animation finishing. We don't delay any signals to bring the shell up". When shown evidence, he says "interesting, will investigate".
- **Written docs/PRs**: clear, short, Sentence case, British spelling (favour, colour→ but code uses "color", emphasise, capitalisation, synchronised, favourite).
  Uses "Some examples;" with a semicolon, and "This PR also...". Low on hype in technical docs. Marketing copy is punchy fragments.
- Humour is dry and playful: "let the tabs be free", "Vanity rename", "Bunch o improvements", an easter-egg logo on the Bonsplit site, and the tui-pets toy.

---

## 7. Decision-making & priorities

- **Feel first: speed, smoothness, no flicker** [OBSERVED/STATED]. 120fps splits, optimistic UI, "Fix load in", "Prevent terminal layout pop-in while WASM loads", "Fix overlapping animation".
- **Remove rather than add** [STATED/OBSERVED]: wants users never to see an empty screen. "Remove yazi integration" one day after adding it. "Remove pool from examples". "Remove unused logic". "Simplify double-click: use whatever action is bound to enter".
- **Defaults that respect the user's setup, with escape hatches** [OBSERVED]: keybinds "additive instead of replacing all defaults". `open_editor` is "opt-in keybinding". "Let agents name sessions is now off by default" (team decision).
  He accepted the request to disable focused-pane highlighting the same day (broke users' themes).
- **Ship fast, polish in public** [OBSERVED]: bontree was built from first commit to website with WASM demo in about 2 days (2026-02-22/23). sesh went from initial to v-done in 2 days. ogpk in 2 days.
- **Willing to keep happy accidents** [STATED]: "it's not really intentional but i also dont hate it".
- **Delegation and roles at Rex** [OBSERVED]: Mitchell triages and accepts. Al and Hector own UI polish ("Al or Hector haven't taken a pass at all on this"). Users ping Al directly about visual and UX details.
- **Pragmatic about upstream** [OBSERVED]: reverts an opencode change because the headless output got worse, and says plainly why.
- **AI as collaborator** [OBSERVED]: agent files in repos, Claude co-author trailers, tools built around agent workflows. He uses multiple agents (Claude Code, Codex, Amp, Pi, opencode, aider fork, toad fork).

---

## 8. Tools / stack

- Terminal: Ghostty (early tester, theme/config conventions), now Rex. Zellij (sesh), lazygit, yazi (tried), fish shell, Neovim (forked "SnaxVim", "A snackable Neovim setup"), Kitty graphics protocol (tui-pets).
- Agents: Claude Code (default in sesh, `--dangerously-skip-permissions`), Codex, Amp, Pi, opencode, aider, Toad.
- Languages and frameworks: Swift/SwiftUI/AppKit, Go plus Bubble Tea/Lip Gloss, Svelte 5/SvelteKit, Next.js/React, Tailwind v4, Ember (past), Ruby/Rails (past), Zig (reading Ghostty).
- Fonts and themes he picks: Maple Mono NF, Geist, JetBrains Darcula, Tokyo Night palette, Nerd Font icons.
- Infra: Vercel deploys, Homebrew taps (homebrew-ogpk), GitHub Actions releases, mise, pnpm, Make.
- Mac utilities referenced: Raycast, Rectangle, CleanShot, PowerToys mappings for a Windows Magic Keyboard (2021 gist), sensible-side-buttons.
- Mobile SSH: Echo (his own app), recommended in sesh README.

---

## 9. Things he dislikes / avoids

- [STATED] UI that covers terminal content (reason for floating the update notice outside the window).
- [STATED] Jarring scroll jumps. Output moving while you select text ("It always bugs me ...").
- [STATED] Throwing away decades of HCI for bare chat boxes. He thinks AI tooling should come back to GUI affordances.
- [STATED, via FSR notes] BEM / bespoke per-feature CSS that keeps growing. He prefers utility classes and tokens.
- [STATED] Empty states users shouldn't have to see at all.
- [OBSERVED] Heavy dependencies and frameworks for small tools (zero-dep width, 2kb Pylon, manual arg parsing).
- [OBSERVED] Hardcoded colors that ignore the user's terminal theme. Wrong brand capitalisation ("Git" not "git"). Misaligned type and uneven whitespace.
- [OBSERVED] Non-native-feeling macOS UI. His libraries stress "Native macOS look and feel using system colors".
- [INFERRED] Long process docs. His written artifacts are short and screenshot-heavy.

---

## 10. Open uncertainties

- No transcript found for Full Stack Radio #128 or the full a16z episode beyond excerpts. CSS philosophy quotes are show-note paraphrase, not verbatim.
- X/Twitter (@almonk) and read.cv were not fetchable (HTTP 402). Tweet-level voice is unverified. The Discord lowercase voice is the best proxy.
- Rex changelog and docs authorship is not attributed. Likely team or Mitchell-written, so don't treat it as his personal voice.
- I couldn't tell which commit messages and bodies he wrote and which an agent wrote. Long structured bodies and `feat:` prefixes are probably agent-generated. Terse "Latest"/"Fix X" subjects are his own habit.
- Vercel-era writing: only one co-authored blog post found. "A chat with Alasdair Monk, VP of Design at Vercel" is referenced on a profile aggregator but wasn't located.
- No public talks or long essays by him were found. His personal site has no blog. Philosophy comes mostly from artifacts and short replies.
- The poolside work is mostly private. The only public piece is `poolsideai/pooleval` (Svelte eval charts, built with Hector Simpson).
- His exact Superlogical role is unknown (design lead vs. also writing Swift client code). Discord suggests he implements UI himself ("al fixed this", "Al tweaked this").
