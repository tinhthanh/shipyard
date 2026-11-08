#!/usr/bin/env python3
"""The fork's Scoop manifest for one release (fork-release.yml).

    scripts/scoop-manifest.py TAG REPO CHECKSUMS OUTDIR

TAG is the release's tag (v0.3.31-fork.6), REPO this repository
(tinhthanh/shipyard), CHECKSUMS the release's checksums.txt. Writes
OUTDIR/bucket/vetgo.json, berth for Windows from berth-windows-*.zip, for the
bucket at github.com/tinhthanh/scoop-bucket:

    scoop bucket add vetgo https://github.com/tinhthanh/scoop-bucket
    scoop install vetgo/vetgo

Scoop installs each version in a folder of its own, so an update never has
to replace the berth.exe a running agent holds.
"""

import json
import pathlib
import sys

ARCHES = {"64bit": "amd64", "arm64": "arm64"}


def sums(path):
    out = {}
    for line in pathlib.Path(path).read_text().splitlines():
        parts = line.split()
        if len(parts) == 2:
            out[parts[1].lstrip("*")] = parts[0]
    return out


def main(argv):
    if len(argv) != 5:
        raise SystemExit(__doc__)
    tag, repo, checksums, outdir = argv[1:]
    s = sums(checksums)
    arch = {}
    for scoop_arch, arch_name in ARCHES.items():
        name = f"berth-windows-{arch_name}.zip"
        if name not in s:
            raise SystemExit(f"{checksums} has no {name}")
        arch[scoop_arch] = {"url": f"https://github.com/{repo}/releases/download/{tag}/{name}", "hash": s[name]}
    manifest = {
        "version": tag.removeprefix("v"),
        "description": "Run coding agents on your own boxes: the berth CLI and laptop agent (Vetgo, a fork of Shipyard)",
        "homepage": f"https://github.com/{repo}",
        "license": "MIT",
        "architecture": arch,
        "bin": "berth.exe",
        "notes": [
            "Start the laptop agent when you sign in: berth agent install",
            "After an update, berth agent restart --if-stale moves a running agent to it.",
        ],
    }
    out = pathlib.Path(outdir) / "bucket" / "vetgo.json"
    out.parent.mkdir(parents=True, exist_ok=True)
    out.write_text(json.dumps(manifest, indent=4) + "\n")


if __name__ == "__main__":
    main(sys.argv)
