#!/usr/bin/env python3
"""The fork's plugin listing, from the releases of tinhthanh/vetgo-releases.

    scripts/plugin-registry.py [--repo OWNER/NAME] [--out index.json] [--upload]

Each plugin there is released as <id>-v<version> with <id>-<version>.tar.gz
(its folder, <id>/ at the top, with berth-plugin.json), or one such archive
per platform named <id>-<version>-<os>-<arch>.tar.gz (a Mac's may be
darwin-universal), usually with a .sha256 beside each. This lists the newest
published release of each plugin (drafts and pre-releases are skipped) by
its archives, the way the laptop agent reads a listing
(internal/agent/marketarchive.go):

  {"plugins": [{"id", "name", "description", "version", "author", "homepage",
                "archive": {"url", "sha256", "size"}            # one for all, or
                "archives": {"<os>-<arch>": {"url", "sha256", "size"}}}]}

The archive's url is relative to the listing, which lives on the release
tagged "registry" in the same repository
(…/releases/download/registry/index.json), so every archive is on the
listing's own address as the agent requires. Each archive is downloaded:
its SHA-256 is worked out (and held to the .sha256 when there is one) and
its manifest read for the name and description, and has to give the
release's version.

--upload puts index.json on the "registry" release (made, not as the
latest, when there is none), replacing the one there. It needs gh signed in
with write access to the repository.
"""

import argparse
import hashlib
import io
import json
import re
import subprocess
import sys
import tarfile
import tempfile
import urllib.request

TAG = re.compile(r"^([a-z0-9][a-z0-9-]{0,47})-v([0-9A-Za-z][0-9A-Za-z.+-]{0,63})$")
REGISTRY_TAG = "registry"
MAX_ARCHIVE = 256 << 20


def gh(*args, capture=True):
    r = subprocess.run(["gh", *args], check=True, text=True, capture_output=capture)
    return r.stdout


def releases(repo):
    out = gh("api", "--paginate", "--slurp", f"repos/{repo}/releases?per_page=100")
    return [r for page in json.loads(out) for r in page]


def download(url, limit):
    with urllib.request.urlopen(url, timeout=120) as resp:
        data = resp.read(limit + 1)
    if len(data) > limit:
        raise SystemExit(f"{url} is larger than {limit >> 20} MB")
    return data


def manifest_of(data, plugin_id):
    with tarfile.open(fileobj=io.BytesIO(data), mode="r:gz") as t:
        tops = {m.name.lstrip("./").split("/", 1)[0] for m in t.getmembers() if m.name.strip("./")}
        if tops != {plugin_id}:
            raise ValueError(f"the archive's top folders are {sorted(tops)}, not just {plugin_id}/")
        for name in (f"{plugin_id}/berth-plugin.json", f"./{plugin_id}/berth-plugin.json"):
            try:
                f = t.extractfile(name)
            except KeyError:
                continue
            if f:
                return json.load(f)
    raise ValueError("the archive has no berth-plugin.json")


def archive(release, asset, plugin_id, version):
    """An archive as the listing gives it, checked, and its manifest."""
    assets = {a["name"]: a for a in release["assets"]}
    name = asset["name"]
    data = download(asset["browser_download_url"], MAX_ARCHIVE)
    digest = hashlib.sha256(data).hexdigest()
    side = assets.get(name + ".sha256")
    if side:
        want = download(side["browser_download_url"], 4096).decode().split()[0].lower()
        if want != digest:
            raise ValueError(f"{name} is {digest}, its .sha256 says {want}")
    m = manifest_of(data, plugin_id)
    if m.get("id", plugin_id) != plugin_id or m.get("version") != version:
        raise ValueError(f"{name}'s manifest is {m.get('id', plugin_id)} {m.get('version')}, not {plugin_id} {version}")
    return {"url": f"../{release['tag_name']}/{name}", "sha256": digest, "size": len(data)}, m


def entry(release, plugin_id, version):
    """A plugin's listing: by its one archive (<id>-<version>.tar.gz, for
    every computer), or by one per platform (<id>-<version>-<os>-<arch>.tar.gz,
    a Mac's maybe darwin-universal), of which the agent takes its own."""
    pattern = re.compile(re.escape(f"{plugin_id}-{version}") + r"(?:-([a-z0-9]+-[a-z0-9]+))?\.tar\.gz$")
    plain, per_platform = None, {}
    for a in release["assets"]:
        m = pattern.fullmatch(a["name"])
        if m and m.group(1):
            per_platform[m.group(1)] = a
        elif m:
            plain = a
    if not plain and not per_platform:
        raise ValueError(f"no {plugin_id}-{version}.tar.gz or {plugin_id}-{version}-<os>-<arch>.tar.gz")
    manifest = None
    out = {"id": plugin_id, "version": version}
    if per_platform:
        out["archives"] = {}
        # A Mac's build first: its manifest describes the plugin.
        for platform in sorted(per_platform, key=lambda p: (not p.startswith("darwin"), p)):
            out["archives"][platform], m = archive(release, per_platform[platform], plugin_id, version)
            manifest = manifest or m
    else:
        out["archive"], manifest = archive(release, plain, plugin_id, version)
    meta = {"id": plugin_id, "name": manifest.get("name") or plugin_id, "version": version}
    for key in ("description", "author", "homepage"):
        if manifest.get(key):
            meta[key] = manifest[key]
    return {**meta, **out}


def listing(repo):
    newest = {}
    for r in releases(repo):
        if r.get("draft") or r.get("prerelease"):
            continue
        m = TAG.match(r["tag_name"])
        if not m:
            continue
        plugin_id, version = m.groups()
        if plugin_id not in newest or r["published_at"] > newest[plugin_id][0]["published_at"]:
            newest[plugin_id] = (r, version)
    plugins = []
    for plugin_id in sorted(newest):
        r, version = newest[plugin_id]
        try:
            plugins.append(entry(r, plugin_id, version))
        except (ValueError, tarfile.TarError, OSError) as e:
            # One broken release does not take the others off the listing.
            print(f"plugin-registry: skipping {r['tag_name']}: {e}", file=sys.stderr)
            continue
        print(f"plugin-registry: {plugin_id} {version}", file=sys.stderr)
    return {"plugins": plugins}


def upload(repo, path):
    try:
        gh("release", "view", REGISTRY_TAG, "-R", repo, "--json", "tagName")
    except subprocess.CalledProcessError:
        gh("release", "create", REGISTRY_TAG, "-R", repo, "--title", "Plugin listing",
           "--notes", "The plugin listing the Vetgo app browses (Settings → Plugins). Written by tinhthanh/shipyard's scripts/plugin-registry.py; not a release to download.",
           "--latest=false")
    gh("release", "upload", REGISTRY_TAG, path, "-R", repo, "--clobber")


def main():
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    p.add_argument("--repo", default="tinhthanh/vetgo-releases")
    p.add_argument("--out", default="index.json")
    p.add_argument("--upload", action="store_true")
    args = p.parse_args()
    doc = listing(args.repo)
    if not doc["plugins"]:
        raise SystemExit("plugin-registry: no plugin releases found; nothing written")
    with open(args.out, "w") as f:
        json.dump(doc, f, indent=2, ensure_ascii=False)
        f.write("\n")
    if args.upload:
        # gh names the asset after the file: it has to be index.json.
        with tempfile.TemporaryDirectory() as d:
            path = f"{d}/index.json"
            with open(path, "w") as f:
                json.dump(doc, f, indent=2, ensure_ascii=False)
                f.write("\n")
            upload(args.repo, path)


if __name__ == "__main__":
    main()
