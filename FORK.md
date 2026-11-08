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
  `HOMEBREW_TAP_KEY`, the private half of the tap's write deploy key), and the npm package `vetgo-cli`
  with its `vetgo-cli-*` builds (scripts/npm-packages.py; npm's trusted
  publishing, no token). Upstream's `release.yml` stays as upstream has it
  and is turned off in this repository (`gh workflow disable release.yml`).
- **Windows CLI:** `scripts/windows-cli-archive.sh` builds
  `berth-windows-{amd64,arm64}.zip` (berth.exe and berthw.exe, the build
  without a console the agent starts from at sign-in, paired in
  `internal/winsys`), which `scripts/install-cli.ps1`, the npm packages
  `vetgo-cli-win32-*` and the Scoop bucket github.com/tinhthanh/scoop-bucket
  (`scripts/scoop-manifest.py`; secret `SCOOP_BUCKET_KEY`, a write deploy
  key) install.
- **One-line installers:** `scripts/install.ps1` (the Windows app),
  `scripts/install-cli.sh` (berth on macOS and Linux) and
  `scripts/install-cli.ps1` (berth on Windows), with `site/install.sh`
  rewritten for this repository, served at https://tinhthanh.github.io/shipyard/ by
  `.github/workflows/pages.yml` (GitHub Pages).
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
