# Vetgo

Run coding agents on your own boxes, and work with them as if they were on
your laptop. Vetgo is a fork of [Shipyard](https://github.com/cosscom/shipyard)
that follows it closely and adds a Windows app, more of Usage & accounts and
plugin tools for agents.

What Shipyard is and how it works, its pieces and its docs are in
[README.md](../README.md), Shipyard's own (kept as upstream writes it, so
syncing never conflicts there; GitHub shows this file instead).

## Install

Releases are at [tinhthanh/shipyard](https://github.com/tinhthanh/shipyard/releases).
What the fork adds is in [FORK.md](../FORK.md) and the
[fork changelog](../docs/changelog-fork.mdx).

**The desktop app on Windows** (an alpha): download
[`Vetgo-windows-x64-setup.exe`](https://github.com/tinhthanh/shipyard/releases/latest/download/Vetgo-windows-x64-setup.exe)
and run it. It installs for your user, with no administrator, and updates
itself from the fork's releases. The installer isn't code-signed yet, so
Windows SmartScreen may ask first: **More info → Run anyway**. With winget,
once its community repository lists it:

```powershell
winget install tinhthanh.Vetgo
```

See [docs/windows](../docs/windows/index.mdx) for what it carries and how this
PC becomes a box (WSL, or Windows itself with Labs).

**`berth`, the CLI and laptop agent, on macOS or Linux**, with Homebrew:

```sh
brew tap tinhthanh/tap && brew trust tinhthanh/tap
brew install tinhthanh/tap/vetgo
```

or with npm (Node 18 or later):

```sh
npm install -g vetgo-cli
```

Both give the `berth` command with the berthd builds it puts on boxes.
Start the laptop agent, and have it start when you log in, with
`berth agent install`. The fork publishes no Mac app (it isn't signed for
other Macs); build it from source (see
[README.md](../README.md#build-from-source), `make app-build`).

**`berthd`, on each box**, as the user your agents will run as:

```sh
curl -fsSL https://github.com/tinhthanh/shipyard/releases/latest/download/install.sh | sh
```

or `brew install tinhthanh/tap/berthd` and then `berthd install`. Both print
a pairing link; paste it into the app (**Add a box**) or run
`berth pair '<link>'`. `berth add ssh me@my-box` does it all over SSH
instead.

| Channel | Gets you | Command |
| --- | --- | --- |
| Release download | Windows app | `Vetgo-windows-x64-setup.exe` |
| winget | Windows app | `winget install tinhthanh.Vetgo` |
| Homebrew | `berth` / `berthd` | `brew install tinhthanh/tap/vetgo` / `…/berthd` |
| npm | `berth` | `npm install -g vetgo-cli` |
| install.sh | `berthd` on a box | `curl -fsSL …/releases/latest/download/install.sh \| sh` |
