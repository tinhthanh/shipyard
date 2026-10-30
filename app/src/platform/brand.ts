// What a fork's build calls itself, in one place: the app's name (branding/,
// scripts/brand.mjs sets VITE_BRAND_NAME) and where its bugs are reported.

export const APP_NAME: string = (import.meta.env?.VITE_BRAND_NAME as string | undefined) || "Shipyard";

// The fork's repository: its releases (the CLI, daemons and install.sh),
// issues and update feed (src-tauri/tauri.fork.conf.json).
export const REPO = "https://github.com/tinhthanh/shipyard";
export const ISSUES = `${REPO}/issues`;
export const RELEASES = `${REPO}/releases`;

// The one line that installs berthd on a box: the fork's install.sh, which
// its release workflow (fork-release.yml) attaches to each release.
export const INSTALL_COMMAND = `curl -fsSL ${RELEASES}/latest/download/install.sh | sh`;
