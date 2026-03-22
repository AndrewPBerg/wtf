#Requires -Version 5.1
<#
.SYNOPSIS
    Install wtf (WorkTreeForge) on Windows.
.PARAMETER Version
    Specific version to install (e.g. "0.1.0"). Defaults to latest.
.PARAMETER InstallDir
    Directory to install the binary. Defaults to "$env:LOCALAPPDATA\Programs\wtf".
#>
param(
    [string]$Version,
    [string]$InstallDir = "$env:LOCALAPPDATA\Programs\wtf"
)

$ErrorActionPreference = 'Stop'
$Repo = 'AndrewPBerg/wtf'

# Detect architecture
$arch = switch ([System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture) {
    'X64'   { 'amd64' }
    'Arm64' { 'arm64' }
    default { throw "Unsupported architecture: $_" }
}

$target = "windows_$arch"

# Get latest version if not specified
if (-not $Version) {
    $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest"
    $Version = $release.tag_name -replace '^v', ''
}

Write-Host "Installing wtf v$Version ($target)..."

$zipName = "wtf_${target}.zip"
$url = "https://github.com/$Repo/releases/download/v$Version/$zipName"
$checksumUrl = "https://github.com/$Repo/releases/download/v$Version/SHA256SUMS"

$tmp = New-Item -ItemType Directory -Path (Join-Path ([System.IO.Path]::GetTempPath()) ([System.Guid]::NewGuid()))

try {
    $zipPath = Join-Path $tmp $zipName
    $checksumPath = Join-Path $tmp 'SHA256SUMS'

    Invoke-WebRequest -Uri $url -OutFile $zipPath -UseBasicParsing
    Invoke-WebRequest -Uri $checksumUrl -OutFile $checksumPath -UseBasicParsing

    # Verify checksum
    $expected = (Get-Content $checksumPath | Where-Object { $_ -match $zipName }) -split '\s+' | Select-Object -First 1
    $actual = (Get-FileHash -Path $zipPath -Algorithm SHA256).Hash.ToLower()

    if ($actual -ne $expected) {
        throw "Checksum mismatch: expected $expected, got $actual"
    }
    Write-Host 'Checksum verified.'

    Expand-Archive -Path $zipPath -DestinationPath $tmp -Force

    # Ensure install directory exists
    if (-not (Test-Path $InstallDir)) {
        New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    }

    Move-Item -Path (Join-Path $tmp 'wtf.exe') -Destination (Join-Path $InstallDir 'wtf.exe') -Force

    # Add to PATH if not already present
    $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    if ($userPath -notlike "*$InstallDir*") {
        [Environment]::SetEnvironmentVariable('Path', "$userPath;$InstallDir", 'User')
        Write-Host "Added $InstallDir to your user PATH. Restart your terminal for it to take effect."
    }

    Write-Host "wtf v$Version installed to $InstallDir\wtf.exe"
} finally {
    Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
}
