#!/usr/bin/env node
// brand.mjs NAME: makes a build's branding from branding/NAME (see
// branding/README.md): icons from its logo, a Tauri config to merge, and the
// web app's name. Prints the config's path. Works on macOS, Linux and Windows.
import { execFileSync } from "node:child_process";
import { mkdirSync, readFileSync, readdirSync, rmSync, writeFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const name = process.argv[2];
if (!name || !/^[a-z0-9-]+$/.test(name)) {
  console.error("usage: node scripts/brand.mjs NAME (a folder in branding/)");
  process.exit(2);
}
const src = join(root, "branding", name);
const brand = JSON.parse(readFileSync(join(src, "brand.json"), "utf8"));
const out = join(root, "app", "src-tauri", "target", "brand", name);
const icons = join(out, "icons");
rmSync(out, { recursive: true, force: true });
mkdirSync(icons, { recursive: true });

// Tauri's own icon generator, from the app's dependencies.
const tauri = join(root, "app", "node_modules", "@tauri-apps", "cli", "tauri.js");
try {
  execFileSync(process.execPath, [tauri, "icon", join(src, brand.logo ?? "logo.png"), "--output", icons], { stdio: ["ignore", "pipe", "pipe"] });
} catch (err) {
  console.error(`tauri icon failed: ${err.stderr ?? err.message}`);
  process.exit(1);
}

const have = new Set(readdirSync(icons));
const icon = ["32x32.png", "128x128.png", "128x128@2x.png", "icon.icns", "icon.ico"].filter((f) => have.has(f)).map((f) => join(icons, f));
const conf = { productName: brand.productName, bundle: { icon } };
writeFileSync(join(out, "brand.conf.json"), JSON.stringify(conf, null, 2));
writeFileSync(join(out, "env"), `VITE_BRAND_NAME=${brand.productName}\n`);
console.log(join(out, "brand.conf.json"));
