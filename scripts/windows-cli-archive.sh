#!/bin/sh
# The fork's CLI archives for Windows, built after `make release` into the
# same dist/ (fork-release.yml):
#
#   berth-windows-<arch>.zip   berth.exe, berthw.exe (the same build without a
#                              console, which the agent starts from at sign-in,
#                              internal/winsys), and the daemons and tmux berth
#                              puts on boxes: Linux, as the other berth
#                              archives carry, and Mac (berthd-darwin-*)
#
#   scripts/windows-cli-archive.sh vX.Y.Z [DIST]
#
# Adds each archive's sha256 to DIST/checksums.txt, which scripts/install-cli.ps1
# and the npm packages check against.
set -eu

version=${1:?usage: scripts/windows-cli-archive.sh vX.Y.Z [DIST]}
case "$version" in
v*) ;;
*) version="v$version" ;;
esac
root=$(cd "$(dirname "$0")/.." && pwd)
dist=$(cd "${2:-$root/dist}" && pwd)
go=${GO:-go}
ldflags="-s -w -X github.com/cosscom/shipyard/internal/version.Version=$version"
tmux_dir=${TMUX_DIR:-$root/bin}

for f in berthd-linux-amd64 berthd-linux-arm64 berthd-darwin-amd64 berthd-darwin-arm64; do
	[ -f "$dist/$f.tar.gz" ] || { echo "no $dist/$f.tar.gz: run make release first" >&2; exit 1; }
done
for a in amd64 arm64; do
	[ -x "$tmux_dir/tmux-linux-$a" ] || { echo "no $tmux_dir/tmux-linux-$a" >&2; exit 1; }
done

stage=$(mktemp -d)
trap 'rm -rf "$stage"' EXIT
mkdir "$stage/daemons"
for f in berthd-linux-amd64 berthd-linux-arm64 berthd-darwin-amd64 berthd-darwin-arm64; do
	tar -xzf "$dist/$f.tar.gz" -C "$stage" berthd
	mv "$stage/berthd" "$stage/daemons/$f"
done

cd "$root"
for arch in amd64 arm64; do
	echo "berth $version windows/$arch"
	dir="$stage/berth-windows-$arch"
	mkdir "$dir"
	CGO_ENABLED=0 GOOS=windows GOARCH=$arch "$go" build -trimpath -ldflags "$ldflags" -o "$dir/berth.exe" ./cmd/berth
	CGO_ENABLED=0 GOOS=windows GOARCH=$arch "$go" build -trimpath -ldflags "$ldflags -H windowsgui" -o "$dir/berthw.exe" ./cmd/berth
	cp "$stage"/daemons/* "$dir/"
	cp "$tmux_dir/tmux-linux-amd64" "$tmux_dir/tmux-linux-arm64" "$dir/"
	rm -f "$dist/berth-windows-$arch.zip"
	# A fixed time and order in the zip, so the same build packs the same bytes.
	python3 - "$dir" "$dist/berth-windows-$arch.zip" <<'EOF'
import os, sys, zipfile
src, out = sys.argv[1], sys.argv[2]
with zipfile.ZipFile(out, "w", zipfile.ZIP_DEFLATED, compresslevel=9) as z:
    for name in sorted(os.listdir(src)):
        info = zipfile.ZipInfo(name, date_time=(2020, 1, 1, 0, 0, 0))
        info.external_attr = 0o755 << 16
        info.compress_type = zipfile.ZIP_DEFLATED
        with open(os.path.join(src, name), "rb") as f:
            z.writestr(info, f.read())
EOF
done

cd "$dist"
sum() { if command -v sha256sum >/dev/null 2>&1; then sha256sum -- "$@"; else shasum -a 256 -- "$@"; fi; }
grep -v ' berth-windows-' checksums.txt >checksums.new || true
sum berth-windows-*.zip >>checksums.new
mv checksums.new checksums.txt
grep ' berth-windows-' checksums.txt
