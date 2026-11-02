#!/bin/sh
# Installs Vetgo's berth command (the CLI and laptop agent) on macOS or Linux
# from the fork's latest release, in one line (served at
# https://tinhthanh.github.io/shipyard/install-cli.sh by
# .github/workflows/pages.yml):
#
#   curl -fsSL https://tinhthanh.github.io/shipyard/install-cli.sh | sh
#
# It needs no root. berth and the daemon builds it puts on boxes go in
# ~/.local/share/vetgo, and ~/.local/bin/berth links to it. The download is
# checked against the release's checksums.txt first. Running it again
# upgrades in place. To make a server a box, run install.sh there instead.
#
#   VETGO_VERSION=v0.3.31-fork.3   a release instead of the latest
#   VETGO_HOME=DIR                 where the files go (~/.local/share/vetgo)
#   VETGO_BIN_DIR=DIR              where the berth link goes (~/.local/bin)
set -eu

repo="tinhthanh/shipyard"
version=${VETGO_VERSION:-latest}
home=${VETGO_HOME:-$HOME/.local/share/vetgo}
bin_dir=${VETGO_BIN_DIR:-$HOME/.local/bin}

say() { printf '%s\n' "$*"; }
die() {
	printf 'vetgo install: %s\n' "$*" >&2
	exit 1
}
need() { command -v "$1" >/dev/null 2>&1 || die "this needs $1"; }

need uname
need tar
need gzip
need mktemp
need sed
if command -v curl >/dev/null 2>&1; then
	fetch() { curl -fsSL --proto '=https' --retry 3 -o "$2" "$1"; }
elif command -v wget >/dev/null 2>&1; then
	fetch() { wget -q -O "$2" "$1"; }
else
	die "this needs curl or wget"
fi
if command -v sha256sum >/dev/null 2>&1; then
	sha256() { sha256sum "$1" | cut -d' ' -f1; }
elif command -v shasum >/dev/null 2>&1; then
	sha256() { shasum -a 256 "$1" | cut -d' ' -f1; }
else
	die "this needs sha256sum or shasum to check the download"
fi

case $(uname -s) in
Linux) os=linux ;;
Darwin) os=darwin ;;
*) die "berth runs on Linux and macOS, not $(uname -s). On Windows: irm https://tinhthanh.github.io/shipyard/install.ps1 | iex" ;;
esac
case $(uname -m) in
x86_64 | amd64) arch=amd64 ;;
aarch64 | arm64) arch=arm64 ;;
*) die "berth is built for amd64 and arm64, not $(uname -m)" ;;
esac

if [ "$version" = latest ]; then
	base="https://github.com/$repo/releases/latest/download"
else
	base="https://github.com/$repo/releases/download/$version"
fi

tmp=$(mktemp -d 2>/dev/null || mktemp -d -t vetgo)
trap 'rm -rf "$tmp"' EXIT
trap 'rm -rf "$tmp"; exit 130' INT
trap 'rm -rf "$tmp"; exit 143' TERM HUP

# check ASSET: download it and stop unless it matches checksums.txt.
check() {
	fetch "$base/$1" "$tmp/$1" || die "could not download $base/$1"
	want=$(sed -n "s/^\([0-9a-f]\{64\}\)  \*\{0,1\}$1\$/\1/p" "$tmp/checksums.txt")
	[ -n "$want" ] || die "checksums.txt has no entry for $1"
	got=$(sha256 "$tmp/$1")
	[ "$got" = "$want" ] || die "$1 does not match its checksum (got $got, want $want); nothing was installed"
}

say "==> Downloading berth ($version, $os/$arch)"
fetch "$base/checksums.txt" "$tmp/checksums.txt" || die "could not download $base/checksums.txt. Releases: https://github.com/$repo/releases"
mkdir "$tmp/x"
check "berth-$os-$arch.tar.gz"
tar -xzf "$tmp/berth-$os-$arch.tar.gz" -C "$tmp/x"
# On a Mac, berth also carries the Mac daemons, to set up and upgrade Mac
# boxes (as the app and the Homebrew formula do).
if [ "$os" = darwin ]; then
	for a in arm64 amd64; do
		check "berthd-darwin-$a.tar.gz"
		mkdir "$tmp/d-$a"
		tar -xzf "$tmp/berthd-darwin-$a.tar.gz" -C "$tmp/d-$a" berthd
		mv "$tmp/d-$a/berthd" "$tmp/x/berthd-darwin-$a"
	done
fi
[ -f "$tmp/x/berth" ] || die "berth-$os-$arch.tar.gz has no berth in it"
chmod 755 "$tmp/x"/*
new_version=$("$tmp/x/berth" version 2>/dev/null) || die "the downloaded berth does not run on this machine"

# Each file is staged beside where it goes, then renamed over the old one: a
# rename on one file system replaces a berth that's running (the agent), as
# a copy over it can't.
mkdir -p "$home" "$bin_dir" || die "cannot create $home or $bin_dir"
rm -rf "$home/.new"
mkdir "$home/.new"
cp "$tmp/x"/* "$home/.new/"
for f in "$home/.new"/*; do
	mv -f "$f" "$home/$(basename "$f")"
done
rmdir "$home/.new"
ln -sf "$home/berth" "$bin_dir/berth"
say "==> Installed $new_version: $bin_dir/berth"

found=$(command -v berth 2>/dev/null || true)
case :$PATH: in
*:"$bin_dir":*)
	if [ -n "$found" ] && [ "$found" != "$bin_dir/berth" ]; then
		say "Note: $found comes before it on your PATH, so \`berth\` runs that one."
	fi
	;;
*) say "Add $bin_dir to your PATH to run berth: export PATH=\"$bin_dir:\$PATH\"" ;;
esac
say ""
say "Next:"
say "  berth agent install      # the laptop agent, started when you log in"
say "  berth add ssh USER@HOST  # make a server a box"
# An agent that was running keeps its build until it restarts.
case $("$home/berth" agent status --json 2>/dev/null | tr -d ' \n') in
*'"running":true'*) say "An agent is running: berth agent restart --if-stale moves it to this build once its work is done." ;;
esac
