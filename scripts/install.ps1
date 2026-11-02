# Installs the Vetgo app on Windows from the fork's latest release, in one
# line (served at https://tinhthanh.github.io/shipyard/install.ps1 by
# .github/workflows/pages.yml):
#
#   irm https://tinhthanh.github.io/shipyard/install.ps1 | iex
#
# It downloads the installer, checks it against the sha256 GitHub records for
# the release's file, installs it for this user (no administrator) and opens
# the app. A file PowerShell downloads isn't marked as from the internet, so
# Windows doesn't stop it to ask, as it does one a browser downloads.
#
# Another release: $env:VETGO_VERSION = "v0.3.31-fork.3" first.

$ErrorActionPreference = "Stop"
# Windows PowerShell's progress bar slows a download to a crawl.
$ProgressPreference = "SilentlyContinue"

function Install-Vetgo {
  $repo = "tinhthanh/shipyard"
  $asset = "Vetgo-windows-x64-setup.exe"
  if ($env:PROCESSOR_ARCHITECTURE -ne "AMD64" -and $env:PROCESSOR_ARCHITEW6432 -ne "AMD64") {
    throw "Vetgo for Windows is built for x64; this computer is $env:PROCESSOR_ARCHITECTURE."
  }
  $api = if ($env:VETGO_VERSION) { "https://api.github.com/repos/$repo/releases/tags/$($env:VETGO_VERSION)" } else { "https://api.github.com/repos/$repo/releases/latest" }
  $release = Invoke-RestMethod -Uri $api -Headers @{ "User-Agent" = "vetgo-install" }
  $file = $release.assets | Where-Object { $_.name -eq $asset } | Select-Object -First 1
  if (-not $file) { throw "Release $($release.tag_name) has no $asset." }
  if (-not $file.digest -or $file.digest -notmatch "^sha256:([0-9a-f]{64})$") { throw "GitHub gives no sha256 for $asset in $($release.tag_name)." }
  $want = $Matches[1]

  $dir = Join-Path ([IO.Path]::GetTempPath()) ("vetgo-install-" + [Guid]::NewGuid().ToString("N"))
  New-Item -ItemType Directory -Path $dir | Out-Null
  try {
    $exe = Join-Path $dir $asset
    Write-Host "Downloading Vetgo $($release.tag_name)..."
    Invoke-WebRequest -Uri $file.browser_download_url -OutFile $exe -UseBasicParsing
    $have = (Get-FileHash -Algorithm SHA256 -Path $exe).Hash.ToLower()
    if ($have -ne $want) { throw "The download doesn't match the release's sha256 ($have, not $want). Nothing was installed." }

    # An update replaces the app, which can't be done while it runs.
    $installed = Join-Path $env:LOCALAPPDATA "Vetgo"
    Get-Process | Where-Object { $_.Path -and $_.Path -like "$installed\berth.exe" } | ForEach-Object { $_.CloseMainWindow() | Out-Null; Start-Sleep -Milliseconds 500; if (-not $_.HasExited) { Stop-Process -Id $_.Id -Force } }
    # The laptop agent runs from these and keeps running: a running program
    # can be renamed though not overwritten, so each moves aside, and the app
    # restarts the agent from the new one when it opens (as an update does,
    # app/src-tauri/windows/hooks.nsh; that installer may be older).
    $moved = @{}
    foreach ($name in "berth-cli.exe", "berth-agent.exe") {
      $f = Join-Path $installed $name
      if (-not (Test-Path $f)) { continue }
      Remove-Item "$f.old", "$f.old2" -Force -ErrorAction SilentlyContinue
      foreach ($aside in "$name.old", "$name.old2") {
        try { Rename-Item $f $aside -ErrorAction Stop; $moved[$f] = Join-Path $installed $aside; break } catch {}
      }
    }

    Write-Host "Installing..."
    $p = Start-Process -FilePath $exe -ArgumentList "/S" -PassThru -Wait
    if ($p.ExitCode -ne 0) {
      # Put back what was moved aside, so the app as it was still runs.
      foreach ($f in $moved.Keys) { if (-not (Test-Path $f)) { Rename-Item $moved[$f] (Split-Path $f -Leaf) -ErrorAction SilentlyContinue } }
      throw "The installer stopped with exit code $($p.ExitCode)."
    }
  } finally {
    Remove-Item -Recurse -Force $dir -ErrorAction SilentlyContinue
  }

  $app = Join-Path $env:LOCALAPPDATA "Vetgo\berth.exe"
  if (-not (Test-Path $app)) { throw "The installer finished, but $app isn't there." }
  Write-Host "Vetgo $($release.tag_name) is installed. Opening it..."
  Start-Process -FilePath $app
}

Install-Vetgo
