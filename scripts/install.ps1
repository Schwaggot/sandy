# Install sandy on Windows from the latest GitHub release.
# Usage: iwr -useb https://raw.githubusercontent.com/schwaggot/sandy/main/scripts/install.ps1 | iex

$ErrorActionPreference = "Stop"

$Repo = if ($env:SANDY_REPO) { $env:SANDY_REPO } else { "schwaggot/sandy" }
$Version = if ($env:SANDY_VERSION) { $env:SANDY_VERSION } else { "latest" }
$InstallDir = if ($env:SANDY_INSTALL_DIR) { $env:SANDY_INSTALL_DIR } else { Join-Path $env:USERPROFILE ".sandy\bin" }

$Arch = switch ($env:PROCESSOR_ARCHITECTURE) {
    "AMD64" { "amd64" }
    "ARM64" { "arm64" }
    default { throw "Unsupported architecture: $env:PROCESSOR_ARCHITECTURE" }
}

if ($Version -eq "latest") {
    $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest"
    $Version = $release.tag_name
}
$Version = $Version.TrimStart("v")

$Asset = "sandy_${Version}_windows_${Arch}.zip"
$Url = "https://github.com/$Repo/releases/download/v$Version/$Asset"

$Tmp = New-Item -ItemType Directory -Path (Join-Path $env:TEMP "sandy-install-$([System.Guid]::NewGuid())")
try {
    $ZipPath = Join-Path $Tmp $Asset
    Write-Host "downloading $Url"
    Invoke-WebRequest -Uri $Url -OutFile $ZipPath
    Expand-Archive -Path $ZipPath -DestinationPath $Tmp -Force

    if (-not (Test-Path $InstallDir)) {
        New-Item -ItemType Directory -Path $InstallDir | Out-Null
    }
    Copy-Item -Path (Join-Path $Tmp "sandy.exe") -Destination (Join-Path $InstallDir "sandy.exe") -Force

    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    if ($userPath -notlike "*${InstallDir}*") {
        [Environment]::SetEnvironmentVariable("Path", "$userPath;$InstallDir", "User")
        Write-Host "added $InstallDir to user PATH (restart your shell to pick it up)"
    }

    Write-Host "installed sandy $Version to $InstallDir\sandy.exe"
}
finally {
    Remove-Item -Recurse -Force $Tmp -ErrorAction SilentlyContinue
}
