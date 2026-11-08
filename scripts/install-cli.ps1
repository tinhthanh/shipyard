# Installs Vetgo's berth command (the CLI and laptop agent) on Windows from
# the fork's latest release, in one line (served at
# https://tinhthanh.github.io/shipyard/install-cli.ps1 by
# .github/workflows/pages.yml):
#
#   irm https://tinhthanh.github.io/shipyard/install-cli.ps1 | iex
#
# It needs no administrator. berth.exe, berthw.exe (the same build without a
# console, which the agent starts from at sign-in) and the daemons berth puts
# on boxes go in %LOCALAPPDATA%\vetgo-cli, which is added to your PATH. The
# download is checked against the release's checksums.txt first. Running it
# again upgrades in place, also while the agent runs. For the desktop app,
# use install.ps1 instead.
#
#   $env:VETGO_VERSION = "v0.3.31-fork.6"   a release instead of the latest
#   $env:VETGO_HOME = "D:\tools\vetgo"      where the files go

$ErrorActionPreference = "Stop"
# Windows PowerShell's progress bar slows a download to a crawl.
$ProgressPreference = "SilentlyContinue"

function Install-VetgoCli {
  $repo = "tinhthanh/shipyard"
  $arch = if ($env:PROCESSOR_ARCHITEW6432) { $env:PROCESSOR_ARCHITEW6432 } else { $env:PROCESSOR_ARCHITECTURE }
  $arch = switch ($arch) { "AMD64" { "amd64" } "ARM64" { "arm64" } default { throw "berth is built for x64 and ARM64 Windows, not $arch." } }
  $asset = "berth-windows-$arch.zip"
  $base = if ($env:VETGO_VERSION) { "https://github.com/$repo/releases/download/$($env:VETGO_VERSION)" } else { "https://github.com/$repo/releases/latest/download" }
  $home_ = if ($env:VETGO_HOME) { $env:VETGO_HOME } else { Join-Path $env:LOCALAPPDATA "vetgo-cli" }

  $dir = Join-Path ([IO.Path]::GetTempPath()) ("vetgo-cli-" + [Guid]::NewGuid().ToString("N"))
  New-Item -ItemType Directory -Path $dir | Out-Null
  try {
    Write-Host "Downloading berth ($(if ($env:VETGO_VERSION) { $env:VETGO_VERSION } else { 'latest' }), windows/$arch)..."
    $sums = (Invoke-WebRequest -Uri "$base/checksums.txt" -UseBasicParsing).Content
    if ($sums -is [byte[]]) { $sums = [Text.Encoding]::UTF8.GetString($sums) }
    $line = ($sums -split "`n") | Where-Object { $_ -match "^([0-9a-f]{64})\s+\*?$([regex]::Escape($asset))\s*$" } | Select-Object -First 1
    if (-not $line -or $line -notmatch "^([0-9a-f]{64})") { throw "checksums.txt has no entry for $asset. Releases: https://github.com/$repo/releases" }
    $want = $Matches[1]
    $zip = Join-Path $dir $asset
    Invoke-WebRequest -Uri "$base/$asset" -OutFile $zip -UseBasicParsing
    $have = (Get-FileHash -Algorithm SHA256 -Path $zip).Hash.ToLower()
    if ($have -ne $want) { throw "$asset doesn't match its checksum ($have, not $want). Nothing was installed." }
    $x = Join-Path $dir "x"
    Expand-Archive -Path $zip -DestinationPath $x
    $new = & (Join-Path $x "berth.exe") version
    if ($LASTEXITCODE) { throw "the downloaded berth doesn't run on this computer." }

    New-Item -ItemType Directory -Force -Path $home_ | Out-Null
    # A running program can be renamed though not overwritten (the agent runs
    # from berth.exe): each file in use moves aside, and the agent moves to
    # the new one when it restarts.
    foreach ($f in Get-ChildItem $x -File) {
      $dest = Join-Path $home_ $f.Name
      if (Test-Path $dest) {
        Remove-Item "$dest.old", "$dest.old2" -Force -ErrorAction SilentlyContinue
        try { Remove-Item $dest -Force -ErrorAction Stop } catch {
          # .old can still be running too, from the update before.
          try { Rename-Item $dest "$($f.Name).old" -ErrorAction Stop } catch { Rename-Item $dest "$($f.Name).old2" -ErrorAction Stop }
        }
      }
      Move-Item $f.FullName $dest
    }
  } finally {
    Remove-Item -Recurse -Force $dir -ErrorAction SilentlyContinue
  }

  # Your PATH, for new terminals and this one.
  $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
  $parts = @($userPath -split ";" | Where-Object { $_ })
  if ($parts -notcontains $home_) {
    [Environment]::SetEnvironmentVariable("Path", (@($home_) + $parts) -join ";", "User")
  }
  if (($env:Path -split ";") -notcontains $home_) { $env:Path = "$home_;$env:Path" }

  Write-Host "Installed $new`: $home_\berth.exe"
  $found = (Get-Command berth -ErrorAction SilentlyContinue | Select-Object -First 1).Source
  if ($found -and $found -ne (Join-Path $home_ "berth.exe")) {
    Write-Host "Note: $found comes first on your PATH, so ``berth`` runs that one."
  }
  Write-Host ""
  Write-Host "Next:"
  Write-Host "  berth agent install      # the laptop agent, started when you sign in"
  Write-Host "  berth add ssh USER@HOST  # make a server a box"
  $status = & (Join-Path $home_ "berth.exe") agent status --json 2>$null
  if ("$status" -match '"running":\s*true') {
    Write-Host "An agent is running: berth agent restart --if-stale moves it to this build once its work is done."
  }
}

Install-VetgoCli
