#!/usr/bin/env python3
"""The fork's npm packages for one release (fork-release.yml).

    scripts/npm-packages.py TAG REPO ARCHIVES OUTDIR

TAG is the release's tag (v0.3.31-fork.3), REPO this repository
(tinhthanh/shipyard), ARCHIVES a folder with the release's berth-*.tar.gz
and berthd-darwin-*.tar.gz. Writes one package folder per package to OUTDIR:

  vetgo                      `npm install -g vetgo` gives the `berth` command
  @vetgo/cli-darwin-arm64    berth for that platform, with the berthd and
  @vetgo/cli-darwin-amd64    tmux builds it puts on boxes (as its archive
  @vetgo/cli-linux-arm64     has them, and on a Mac the Mac berthd builds
  @vetgo/cli-linux-amd64     too, for `berth upgrade` of Mac boxes)

As esbuild does: vetgo lists the platform packages as optional dependencies,
npm installs only the one whose os and cpu match, and vetgo's bin starts the
berth in it. No install script runs, so nothing is downloaded at install
time (and installs where scripts are off, as pnpm's default, still work).
"""

import json
import pathlib
import shutil
import sys
import tarfile

PLATFORMS = [("darwin", "arm64", "arm64"), ("darwin", "amd64", "x64"), ("linux", "arm64", "arm64"), ("linux", "amd64", "x64")]

WRAPPER = r"""#!/usr/bin/env node
// berth, from the package for this computer (@vetgo/cli-<os>-<arch>), which
// npm installed beside this one as an optional dependency.
"use strict";
const { spawnSync } = require("node:child_process");
const fs = require("node:fs");

const arch = { arm64: "arm64", x64: "amd64" }[process.arch];
const pkg = `@vetgo/cli-${process.platform}-${arch}`;
let bin;
try {
  bin = require.resolve(`${pkg}/bin/berth`);
} catch {
  console.error(`vetgo: no berth for ${process.platform}/${process.arch} (${pkg} isn't installed).`);
  console.error("It runs on macOS and Linux, arm64 and x64. If yours is one, reinstall without --no-optional / --omit=optional.");
  process.exit(1);
}
try {
  fs.accessSync(bin, fs.constants.X_OK);
} catch {
  try {
    fs.chmodSync(bin, 0o755);
  } catch {}
}
const r = spawnSync(bin, process.argv.slice(2), { stdio: "inherit" });
if (r.error) {
  console.error(`vetgo: ${r.error.message}`);
  process.exit(1);
}
if (r.signal) process.kill(process.pid, r.signal);
process.exit(r.status ?? 1);
"""


def common(repo, version):
    return {
        "version": version,
        "license": "MIT",
        "homepage": f"https://github.com/{repo}",
        "repository": {"type": "git", "url": f"git+https://github.com/{repo}.git"},
        "bugs": {"url": f"https://github.com/{repo}/issues"},
    }


def extract(archive, into, rename=None):
    with tarfile.open(archive) as t:
        for m in t.getmembers():
            if not m.isfile():
                continue
            name = rename.get(m.name, m.name) if rename else m.name
            dest = into / name
            dest.parent.mkdir(parents=True, exist_ok=True)
            with t.extractfile(m) as src, open(dest, "wb") as out:
                shutil.copyfileobj(src, out)
            dest.chmod(0o755)


def main(argv):
    if len(argv) != 5:
        raise SystemExit(__doc__)
    tag, repo, archives, outdir = argv[1:]
    version = tag.removeprefix("v")
    archives, outdir = pathlib.Path(archives), pathlib.Path(outdir)
    if outdir.exists():
        shutil.rmtree(outdir)
    optional = {}
    for os_name, arch, npm_cpu in PLATFORMS:
        name = f"@vetgo/cli-{os_name}-{arch}"
        d = outdir / f"cli-{os_name}-{arch}"
        bin_dir = d / "bin"
        bin_dir.mkdir(parents=True)
        # berth looks beside its own file for the builds it uploads to boxes.
        extract(archives / f"berth-{os_name}-{arch}.tar.gz", bin_dir)
        if os_name == "darwin":
            for a in ("arm64", "amd64"):
                extract(archives / f"berthd-darwin-{a}.tar.gz", bin_dir, {"berthd": f"berthd-darwin-{a}"})
        pkg = {"name": name, **common(repo, version),
               "description": f"berth for {os_name} {arch}, installed by the vetgo package",
               "os": [os_name], "cpu": [npm_cpu], "files": ["bin"], "preferUnplugged": True}
        (d / "package.json").write_text(json.dumps(pkg, indent=2) + "\n")
        (d / "README.md").write_text(f"# {name}\n\nThe `berth` build for {os_name} {arch}. Install [`vetgo`](https://www.npmjs.com/package/vetgo) rather than this.\n")
        optional[name] = version
    d = outdir / "vetgo"
    (d / "bin").mkdir(parents=True)
    (d / "bin" / "berth.js").write_text(WRAPPER)
    (d / "bin" / "berth.js").chmod(0o755)
    pkg = {"name": "vetgo", **common(repo, version),
           "description": "Run coding agents on your own boxes: the berth CLI and laptop agent (a fork of Shipyard)",
           "keywords": ["coding-agents", "claude-code", "codex", "ssh", "tmux", "developer-tools"],
           "bin": {"berth": "bin/berth.js"}, "files": ["bin"], "engines": {"node": ">=18"},
           "optionalDependencies": optional}
    (d / "package.json").write_text(json.dumps(pkg, indent=2) + "\n")
    (d / "README.md").write_text(f"""# vetgo

Run coding agents on your own boxes, and watch them from your laptop. Vetgo is a fork of [Shipyard](https://github.com/cosscom/shipyard); source and docs at [{repo}](https://github.com/{repo}).

```sh
npm install -g vetgo
berth agent install      # the laptop agent, started when you log in
berth add ssh USER@HOST  # make a server a box
```

This package gives the `berth` command on macOS and Linux (arm64 and x64). The desktop app for Windows is on the [releases page](https://github.com/{repo}/releases).
""")


if __name__ == "__main__":
    main(sys.argv)
