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
# Windows PowerShell's progress bars make downloads and Expand-Archive crawl,
# and in a hidden window can stall them outright.
$ProgressPreference = "SilentlyContinue"
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
  Remove-Item Env:GOOS, Env:GOARCH -ErrorAction SilentlyContinue
  # berthd for Windows itself, for this computer's box run natively (Labs):
  # berthd-bg.exe is the same program without a console, which Windows
  # starts at sign-in (berthd install).
  go build -trimpath -ldflags $ld -o (Join-Path $bin "berthd.exe") ./cmd/berthd
  if ($LASTEXITCODE) { throw "go build (berthd) failed" }
  go build -trimpath -ldflags "$ld -H windowsgui" -o (Join-Path $bin "berthd-bg.exe") ./cmd/berthd
  if ($LASTEXITCODE) { throw "go build (berthd-bg) failed" }
} finally {
  Remove-Item Env:GOOS, Env:GOARCH, Env:CGO_ENABLED -ErrorAction SilentlyContinue
  Pop-Location
}

# Its tmux: psmux (MIT, github.com/psmux/psmux), built from the source of a
# pinned release whose archive must match this hash, never a prebuilt
# binary. Built once per version; it takes a few minutes.
$psmuxTag = "v3.4.0"
$psmuxSha256 = "f4a93929cd3462d9e744f5076824f3ad57b9d6d5ba0df098389f51fa296fdc15"
$tmuxExe = Join-Path $bin "tmux.exe"
$tmuxStamp = Join-Path $bin "tmux.version"
if (-not ((Test-Path $tmuxExe) -and (Test-Path $tmuxStamp) -and ((Get-Content $tmuxStamp) -eq $psmuxTag))) {
  $work = Join-Path $root "app\src-tauri\target\psmux"
  New-Item -ItemType Directory -Force -Path $work | Out-Null
  $zip = Join-Path $work "psmux-$psmuxTag.zip"
  # curl.exe, which Windows 10 and 11 carry: Windows PowerShell's
  # Invoke-WebRequest can stall for minutes drawing its progress.
  curl.exe -fsSL -o $zip "https://github.com/psmux/psmux/archive/refs/tags/$psmuxTag.zip"
  if ($LASTEXITCODE) { throw "downloading psmux $psmuxTag failed" }
  if ((Get-FileHash $zip -Algorithm SHA256).Hash.ToLower() -ne $psmuxSha256) { throw "psmux $psmuxTag's source archive is not the one that was reviewed" }
  $src = Join-Path $work "src"
  Remove-Item $src -Recurse -Force -ErrorAction SilentlyContinue
  New-Item -ItemType Directory -Force -Path $src | Out-Null
  tar.exe -xf $zip -C $src
  if ($LASTEXITCODE) { throw "unpacking psmux $psmuxTag failed" }
  $dir = (Get-ChildItem $src -Directory | Select-Object -First 1).FullName
  Push-Location $dir
  try {
    $env:PSMUX_GIT_SHA = $psmuxTag
    cargo build --release --bin psmux
    if ($LASTEXITCODE) { throw "building psmux failed" }
  } finally { Remove-Item Env:PSMUX_GIT_SHA -ErrorAction SilentlyContinue; Pop-Location }
  Copy-Item (Join-Path $dir "target\release\psmux.exe") $tmuxExe -Force
  Copy-Item (Join-Path $dir "LICENSE") (Join-Path $bin "psmux-LICENSE.txt") -Force
  Set-Content -Path $tmuxStamp -Value $psmuxTag -Encoding ascii
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
