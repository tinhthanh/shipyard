# Builds the Windows app (an alpha): berth.exe as the app's sidecar, then
# the Tauri app and its NSIS installer. Run from the repository root on
# Windows with Go, Node + pnpm, Rust (MSVC) and the VS C++ build tools:
#
#   powershell -ExecutionPolicy Bypass -File scripts\windows-app-build.ps1 [-Version 0.3.31]
#
# The installer lands in app\src-tauri\target\release\bundle\nsis\.
param([string]$Version = "")
$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
$triple = "x86_64-pc-windows-msvc"
$bin = Join-Path $root "app\src-tauri\binaries"
New-Item -ItemType Directory -Force -Path $bin | Out-Null

$ld = "-s -w"
if ($Version) { $ld += " -X github.com/cosscom/shipyard/internal/version.Version=v$Version" }
Push-Location $root
try {
  go build -trimpath -ldflags $ld -o (Join-Path $bin "berth-cli-$triple.exe") ./cmd/berth
  if ($LASTEXITCODE) { throw "go build failed" }
} finally { Pop-Location }

Push-Location (Join-Path $root "app")
try {
  pnpm.cmd install --frozen-lockfile
  if ($LASTEXITCODE) { throw "pnpm install failed" }
  $extra = @()
  if ($Version) { $extra = @("--config", "{`"version`":`"$Version`"}") }
  pnpm.cmd tauri build @extra
  if ($LASTEXITCODE) { throw "tauri build failed" }
} finally { Pop-Location }

Get-ChildItem (Join-Path $root "app\src-tauri\target\release\bundle\nsis") -Filter *.exe | ForEach-Object { $_.FullName }
