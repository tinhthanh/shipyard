#!/usr/bin/env python3
"""The Homebrew cask for the fork's Mac app, in the tap (fork-release.yml).

    scripts/homebrew-cask.py TAG REPO SHA256 TAP

Writes TAP/Casks/vetgo-app.rb for the release TAG of REPO, whose
Vetgo-macos-universal.zip (Apple silicon and Intel) has the SHA-256 given:

    brew install --cask tinhthanh/tap/vetgo-app

The app is signed with an Apple Development certificate (or ad hoc, when the
repository has none), not Developer ID, and isn't notarized, so Gatekeeper
would refuse it as downloaded. The cask takes the quarantine flag off what it
installed, as `xattr -dr com.apple.quarantine` by hand would; once releases
are signed with Developer ID and notarized, that postflight goes. The app
doesn't update itself on a Mac (its feed has only the Windows app), so
`brew upgrade` is how it updates.
"""

import pathlib
import re
import sys

CASK = """\
# Written by tinhthanh/shipyard's scripts/homebrew-cask.py for each release
# (fork-release.yml); edits here are overwritten by the next one.
cask "vetgo-app" do
  version "{version}"
  sha256 "{sha256}"

  url "https://github.com/{repo}/releases/download/v#{{version}}/Vetgo-macos-universal.zip"
  name "Vetgo"
  desc "Run coding agents on your own boxes (a fork of Shipyard)"
  homepage "https://github.com/{repo}"

  depends_on :macos

  app "Vetgo.app"

  # Signed for development, not notarized: Gatekeeper would refuse the app as
  # downloaded. Taken off once releases are notarized.
  postflight_steps default_base: :appdir do
    run "/usr/bin/xattr",
        args:           ["-dr", "com.apple.quarantine", "Vetgo.app"],
        chdir:          ".",
        writable_paths: ["Vetgo.app"]
  end

  caveats <<~EOS
    Vetgo carries its own berth CLI and laptop agent. Open the app and start
    the agent there (Start at login keeps it running); `brew install
    tinhthanh/tap/vetgo` adds berth to your PATH, the same version.
    Update the app with `brew upgrade --cask vetgo-app`.
  EOS
end
"""


def main(argv):
    if len(argv) != 5:
        raise SystemExit(__doc__)
    tag, repo, sha256, tap = argv[1:]
    if not re.fullmatch(r"v[0-9][0-9A-Za-z.+-]*", tag):
        raise SystemExit(f"homebrew-cask: {tag} is not a release tag")
    if not re.fullmatch(r"[0-9a-f]{64}", sha256):
        raise SystemExit("homebrew-cask: the SHA-256 is 64 hex digits")
    out = pathlib.Path(tap) / "Casks" / "vetgo-app.rb"
    out.parent.mkdir(parents=True, exist_ok=True)
    out.write_text(CASK.format(version=tag.removeprefix("v"), sha256=sha256, repo=repo))
    print(f"homebrew-cask: {out} for {tag}")


if __name__ == "__main__":
    main(sys.argv)
