# This fork

`tinhthanh/shipyard` follows `cosscom/shipyard` and adds the Windows app, a
floating bar, more of Usage & accounts, and plugin tools for agents. It is
laid out so that syncing from upstream meets as few conflicts as it can:
the fork adds files of its own, and upstream's files only call into them.

## Where the fork's code lives

- **Windows, Go:** `internal/winsys` (hidden consoles, private files,
  elevation), `*_windows.go` files, and per-platform files beside upstream's
  (`internal/agent/localbox_platform.go`, `cmd/berth/proc_*.go`,
  `cmd/berthd/platform.go`, `internal/box/platform.go`, `powershell.go`).
  Shared files have no `GOOS == "windows"` branches; they call these.
- **Windows, app:** `app/src-tauri/src/windows.rs`, `app/src/platform/`,
  `app/src/views/onboarding/local-box-windows.tsx`,
  `app/src-tauri/tauri.windows.conf.json`, `scripts/windows-app-build.ps1`.
- **Name and repository:** `app/src/platform/brand.ts` (the app's name, this
  repository's links and install command), `app/src-tauri/tauri.fork.conf.json`
  (the update feed), `branding/` with `scripts/brand.mjs`.
- **Releases:** `.github/workflows/fork-release.yml`, on `vX.Y.Z-fork.N`
  tags: the CLI and daemons, the Windows app and its update feed, then a
  pull request to winget's community repository for `tinhthanh.Vetgo`
  (secret `WINGET_TOKEN`), and the Homebrew formulas `vetgo` and `berthd` in
  github.com/tinhthanh/homebrew-tap (scripts/homebrew-formulas.py; secret
  `HOMEBREW_TAP_KEY`, the private half of the tap's write deploy key), the
  Mac app (`Vetgo-macos-universal.zip`, signed with the Apple Development
  certificate in `MAC_SIGNING_P12_BASE64`/`MAC_SIGNING_P12_PASSWORD` or ad
  hoc, not notarized) and its cask `vetgo-app` in the same tap
  (scripts/homebrew-cask.py), and the npm package `vetgo-cli`
  with its `vetgo-cli-*` builds (scripts/npm-packages.py; npm's trusted
  publishing, no token). Upstream's `release.yml` stays as upstream has it
  and is turned off in this repository (`gh workflow disable release.yml`).
- **Windows CLI:** `scripts/windows-cli-archive.sh` builds
  `berth-windows-{amd64,arm64}.zip` (berth.exe and berthw.exe, the build
  without a console the agent starts from at sign-in, paired in
  `internal/winsys`), which `scripts/install-cli.ps1`, the npm packages
  `vetgo-cli-windows-*` and the Scoop bucket github.com/tinhthanh/scoop-bucket
  (`scripts/scoop-manifest.py`; secret `SCOOP_BUCKET_KEY`, a write deploy
  key) install.
- **One-line installers:** `scripts/install.ps1` (the Windows app),
  `scripts/install-cli.sh` (berth on macOS and Linux) and
  `scripts/install-cli.ps1` (berth on Windows), with `site/install.sh`
  rewritten for this repository, served at https://tinhthanh.github.io/shipyard/ by
  `.github/workflows/pages.yml` (GitHub Pages).
- **An account per task:** `internal/box/taskaccounts.go` (the request's
  `accounts`, checked against the box's account folders, set on the session
  it starts) and `app/src/components/conversation/account-pick.tsx` (New
  task's Account picker), with seams in `api.go`, `tasks.go`, `secrets.go`,
  `start-work.ts`, `api.ts` and `task-composer.tsx`.
- **Prompt suggestions:** `internal/box/promptsuggest.go` (asks a
  structured Claude for `--prompt-suggestions`, keeps the latest, `GET
  /v1/sessions/{name}/suggestion`) and
  `app/src/components/conversation/prompt-suggestion.ts` (grey in Reply, Tab
  takes it), with seams in `claudedriver.go`, `api.go`, `upgrade.go` and
  `conversation-pane.tsx`.
- **Pinned projects:** `app/src/components/sidebar/project-pins.tsx` (the
  rail lists only pinned projects once one is, the sheet heading's project
  switcher, the pin item in a project's menu) and `app/e2e/project-pins.spec.ts`,
  with seams in `rail-layout.tsx` and `actions.tsx`.
- **Session accounts:** `internal/box/sessionaccount.go` (each session's
  `account`, `POST /v1/sessions/{name}/account`, which copies the Claude
  conversation to the other account's folder and starts it again there
  with `--resume`) and `app/src/components/conversation/account-switch.tsx`
  (the account chip, the dialog, the limit notice's step), with
  `app/src/lib/mock-session-account.ts`; seams in `sessions.go`,
  `tasks.go` (enrich), `api.go`, `upgrade.go`, `api.ts`, the SDK's
  `types.ts`, `chat-controls.tsx`, `conversation-pane.tsx`,
  `notice-card.tsx`, `rail.tsx`, `App.tsx`, `mock.ts` and the floating
  bar's `bar-state.ts`.
- **Plugin listing:** `internal/agent/marketarchive.go` (the default
  listing is `index.json` on the `registry` release of
  github.com/tinhthanh/vetgo-releases, not upstream's berthd.app; a listed
  plugin may give its release `.tar.gz` as `archive`, or one per platform as
  `archives`, of which a computer is offered only its own) and
  `scripts/plugin-registry.py` (writes that `index.json` from the
  `<id>-v<version>` releases there; `--upload` puts it on the release), with
  seams in `marketplace.go`; `internal/agent/marketadopt.go` takes as the
  listing's a plugin installed by hand whose files are exactly a release's
  (the listed one, or `../<id>-v<version>/` beside it), so its updates are
  offered. vetgo-releases runs a copy of the script hourly
  and on each release, after mirroring chrome and computer from their own
  repositories (its `mirrors.json`, `scripts/mirror-plugins.py`).
- **Plugins page and icons:** `app/src/views/settings/plugin-store/`
  (Settings → Plugins as a search, a filter, your plugins, a Discover grid
  and a sheet per plugin holding upstream's rows; `plugin-icons.tsx`) with
  `app/e2e/plugin-store.ts`, and `internal/agent/pluginicon.go` (a
  manifest's or listing's `icon`, handed to the app as a data: URL); seams
  in `settings-view.tsx`, `plugins-section.tsx` (PluginRow and BuiltinRow
  exported), `ui.go`, `plugintools.go` and `marketplace.go`.
- **Docs:** `docs/windows/`, `docs/changelog-fork.mdx`, and `.github/README.md`
  (the fork's front page on GitHub, with how to install; `README.md` stays
  upstream's).

## Rules

- **A change to an upstream file is a seam:** one to three lines that call
  the fork's code. Logic goes in the fork's files.
- **Don't reformat upstream files** (`cargo fmt`, `gofmt -w` on a whole
  tree, an editor's format-on-save): it turns a seam into a conflict.
- **Changelog:** the fork's user-visible changes get a bullet under
  **Unreleased** in `docs/changelog-fork.mdx`, not in `docs/changelog.mdx`,
  which stays as upstream writes it.
- **Syncing:** merge `upstream/main` into a `sync/upstream-*` branch and open
  a PR here; conflicts should only be in seams.
