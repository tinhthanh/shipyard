# Builds the Windows app (an alpha): berth.exe as the app's sidecar, the
# Linux berthd it carries, then the Tauri app and its NSIS installer. Run
# from the repository root on Windows with Go, Node + pnpm, Rust (MSVC) and
# the VS C++ build tools:
#
#   powershell -ExecutionPolicy Bypass -File scripts\windows-app-build.ps1 [-Version 0.3.31] [-Brand vetgo]
#
# -Brand NAME builds it under branding\NAME's name and icon (branding\README.md).
#
# The installer lands in app\src-tauri\target\release\bundle\nsis\.
param([string]$Version = "", [string]$Brand = "")
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
  # The same program without a console: what Windows runs at logon to start
  # the agent (berth agent install), so no console window opens.
  go build -trimpath -ldflags "$ld -H windowsgui" -o (Join-Path $bin "berth-agent-$triple.exe") ./cmd/berth
  if ($LASTEXITCODE) { throw "go build (agent) failed" }
  # The Linux berthd, for this computer's box in WSL and for boxes added over
  # SSH (beside berth.exe once installed, where both look for it).
  $env:CGO_ENABLED = "0"
  $env:GOOS = "linux"
  foreach ($arch in "amd64", "arm64") {
    $env:GOARCH = $arch
    go build -trimpath -ldflags $ld -o (Join-Path $bin "berthd-linux-$arch") ./cmd/berthd
    if ($LASTEXITCODE) { throw "go build (berthd linux/$arch) failed" }
  }
} finally {
  Remove-Item Env:GOOS, Env:GOARCH, Env:CGO_ENABLED -ErrorAction SilentlyContinue
  Pop-Location
}

Push-Location (Join-Path $root "app")
try {
  pnpm.cmd install --frozen-lockfile
  if ($LASTEXITCODE) { throw "pnpm install failed" }
  $extra = @()
  if ($Brand) {
    $brandConf = (node (Join-Path $root "scripts\brand.mjs") $Brand | Select-Object -Last 1)
    if ($LASTEXITCODE) { throw "brand $Brand failed" }
    $env:VITE_BRAND_NAME = ((Get-Content (Join-Path (Split-Path $brandConf) "env")) -replace "^VITE_BRAND_NAME=", "")
    $extra += @("--config", $brandConf)
  }
  if ($Version) {
    # A file, not inline JSON: Windows PowerShell drops the quotes of an
    # argument it passes to a program.
    $conf = Join-Path $env:TEMP "berth-version.conf.json"
    Set-Content -Path $conf -Value ('{"version":"' + $Version + '"}') -Encoding ascii
    $extra += @("--config", $conf)
  }
  pnpm.cmd tauri build @extra
  if ($LASTEXITCODE) { throw "tauri build failed" }
} finally { Pop-Location }

Get-ChildItem (Join-Path $root "app\src-tauri\target\release\bundle\nsis") -Filter *.exe | ForEach-Object { $_.FullName }
