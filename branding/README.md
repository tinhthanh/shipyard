# Branding (fork)

A build can carry another name and icon than Shipyard's, without touching
the repository's own icons: `app/src-tauri/icons` stays as upstream has it.

Each folder here is one brand: `brand.json` (`productName`) and a square PNG
logo (`logo`, 1024×1024 is best; 500×500 works). `scripts/brand.mjs NAME`
turns it into what a build needs, under `app/src-tauri/target/brand/NAME/`
(ignored by git):

- `icons/`: every icon Tauri wants, made from the logo with `tauri icon`;
- `brand.conf.json`: a Tauri config to merge (`--config`): the product name
  and those icons;
- `env`: `VITE_BRAND_NAME`, which the web app reads for its window title.

Builds that take a brand:

    make app-build BRAND=vetgo …                              # macOS, Linux
    scripts\windows-app-build.ps1 -Brand vetgo -Version 0.3.31   # Windows

Without one, the app is Shipyard, as upstream builds it. The bundle id
(`dev.berth.app`), the CLI (`berth`) and every path and setting keep their
names, so a branded app reads the same state as Shipyard.
