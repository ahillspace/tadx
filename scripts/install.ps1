[CmdletBinding()]
param(
    [ValidateSet('Install', 'Uninstall')]
    [string]$Action = 'Install',

    [string]$Version = '',

    [string]$InstallDir = '',

    [switch]$NoModifyPath
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

# Windows PowerShell 5.1 can otherwise negotiate an obsolete TLS version.
[Net.ServicePointManager]::SecurityProtocol =
    [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

$Repository = 'ahillspace/tadx'

function Get-DefaultInstallDir {
    if (-not [string]::IsNullOrWhiteSpace($env:LOCALAPPDATA)) {
        return Join-Path $env:LOCALAPPDATA 'Programs\tadx\bin'
    }

    return Join-Path ([Environment]::GetFolderPath('LocalApplicationData')) 'Programs\tadx\bin'
}

function Get-WindowsArchitecture {
    if ($env:OS -ne 'Windows_NT') {
        throw 'This installer supports Windows only. Use scripts/install.sh on macOS or Linux.'
    }

    $architecture = $env:PROCESSOR_ARCHITEW6432
    if ([string]::IsNullOrWhiteSpace($architecture)) {
        $architecture = $env:PROCESSOR_ARCHITECTURE
    }

    switch -Regex ($architecture) {
        '^(AMD64|x86_64)$' { return 'amd64' }
        '^(ARM64|aarch64)$' { return 'arm64' }
        default { throw "Unsupported Windows architecture: $architecture. TADX supports amd64 and arm64." }
    }
}

function Invoke-Download {
    param(
        [Parameter(Mandatory = $true)][string]$Uri,
        [Parameter(Mandatory = $true)][string]$Destination
    )

    Invoke-WebRequest -UseBasicParsing -Uri $Uri -OutFile $Destination
}

function Get-ChecksumEntries {
    param([Parameter(Mandatory = $true)][string]$ManifestPath)

    $entries = @()
    foreach ($line in Get-Content -LiteralPath $ManifestPath) {
        if ([string]::IsNullOrWhiteSpace($line)) {
            continue
        }

        if ($line -notmatch '^\s*([0-9a-fA-F]{64})\s+\*?(.+?)\s*$') {
            throw 'The release checksum manifest has an unsupported format.'
        }

        $entries += [pscustomobject]@{
            Hash = $Matches[1].ToLowerInvariant()
            File = $Matches[2]
        }
    }

    return $entries
}

function Assert-ArchiveChecksum {
    param(
        [Parameter(Mandatory = $true)][string]$ArchivePath,
        [Parameter(Mandatory = $true)][string]$AssetName,
        [Parameter(Mandatory = $true)][object[]]$Entries
    )

    $matches = @($Entries | Where-Object { $_.File -ceq $AssetName })
    if ($matches.Count -ne 1) {
        throw "The checksum manifest must contain exactly one entry for $AssetName."
    }

    $actual = (Get-FileHash -Algorithm SHA256 -LiteralPath $ArchivePath).Hash.ToLowerInvariant()
    if ($actual -cne $matches[0].Hash) {
        throw "Checksum verification failed for $AssetName. The archive was not installed."
    }
}

function Add-UserPath {
    param([Parameter(Mandatory = $true)][string]$Directory)

    $normalized = [IO.Path]::GetFullPath($Directory).TrimEnd('\', '/')
    $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    $parts = @()
    if (-not [string]::IsNullOrWhiteSpace($userPath)) {
        $parts = @($userPath -split ';' | Where-Object { -not [string]::IsNullOrWhiteSpace($_) })
    }

    $present = $parts | Where-Object {
        try {
            [IO.Path]::GetFullPath($_).TrimEnd('\', '/') -ieq $normalized
        }
        catch {
            $_.TrimEnd('\', '/') -ieq $normalized
        }
    }

    if (-not $present) {
        $updated = (@($parts) + $normalized) -join ';'
        [Environment]::SetEnvironmentVariable('Path', $updated, 'User')
    }

    $processParts = @($env:Path -split ';')
    if (-not ($processParts | Where-Object { $_.TrimEnd('\', '/') -ieq $normalized })) {
        $env:Path = "$normalized;$env:Path"
    }
}

function Remove-UserPath {
    param([Parameter(Mandatory = $true)][string]$Directory)

    $normalized = [IO.Path]::GetFullPath($Directory).TrimEnd('\', '/')
    $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    if ([string]::IsNullOrWhiteSpace($userPath)) {
        return
    }

    $kept = @($userPath -split ';' | Where-Object {
        if ([string]::IsNullOrWhiteSpace($_)) {
            return $false
        }
        try {
            return [IO.Path]::GetFullPath($_).TrimEnd('\', '/') -ine $normalized
        }
        catch {
            return $_.TrimEnd('\', '/') -ine $normalized
        }
    })
    [Environment]::SetEnvironmentVariable('Path', ($kept -join ';'), 'User')
}

function Install-TadxBinary {
    param(
        [Parameter(Mandatory = $true)][string]$Source,
        [Parameter(Mandatory = $true)][string]$Directory
    )

    New-Item -ItemType Directory -Force -Path $Directory | Out-Null
    $destination = Join-Path $Directory 'tadx.exe'
    $staged = Join-Path $Directory ('.tadx.new.' + [Guid]::NewGuid().ToString('N') + '.exe')
    $backup = Join-Path $Directory ('.tadx.backup.' + [Guid]::NewGuid().ToString('N') + '.exe')

    try {
        Copy-Item -LiteralPath $Source -Destination $staged
        if (Test-Path -LiteralPath $destination) {
            [IO.File]::Replace($staged, $destination, $backup, $true)
            Remove-Item -LiteralPath $backup -Force -ErrorAction SilentlyContinue
        }
        else {
            [IO.File]::Move($staged, $destination)
        }
    }
    catch {
        Remove-Item -LiteralPath $staged -Force -ErrorAction SilentlyContinue
        if ((-not (Test-Path -LiteralPath $destination)) -and (Test-Path -LiteralPath $backup)) {
            [IO.File]::Move($backup, $destination)
        }
        throw
    }
    finally {
        Remove-Item -LiteralPath $staged -Force -ErrorAction SilentlyContinue
        Remove-Item -LiteralPath $backup -Force -ErrorAction SilentlyContinue
    }
}

if ([string]::IsNullOrWhiteSpace($InstallDir)) {
    $InstallDir = Get-DefaultInstallDir
}
$InstallDir = [IO.Path]::GetFullPath($InstallDir)

if ($Action -ieq 'Uninstall') {
    $binaryPath = Join-Path $InstallDir 'tadx.exe'
    if (Test-Path -LiteralPath $binaryPath) {
        Remove-Item -LiteralPath $binaryPath -Force
    }
    Remove-UserPath -Directory $InstallDir
    if ((Test-Path -LiteralPath $InstallDir) -and -not (Get-ChildItem -Force -LiteralPath $InstallDir | Select-Object -First 1)) {
        Remove-Item -LiteralPath $InstallDir -Force
    }
    Write-Host 'TADX was removed. Configuration, workspaces, Guidance, and OS-stored credentials were preserved.'
    return
}

$architecture = Get-WindowsArchitecture
if ([string]::IsNullOrWhiteSpace($Version)) {
    $Version = $env:TADX_VERSION
}
if ([string]::IsNullOrWhiteSpace($Version)) {
    $Version = 'latest'
}

$temporaryDirectory = Join-Path ([IO.Path]::GetTempPath()) ('tadx-install-' + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $temporaryDirectory | Out-Null

try {
    $manifestPath = Join-Path $temporaryDirectory 'checksums.txt'
    $releaseBase = ''

    if ($Version -ieq 'latest') {
        $releaseBase = "https://github.com/$Repository/releases/latest/download"
        Invoke-Download -Uri "$releaseBase/checksums.txt" -Destination $manifestPath
        $entries = @(Get-ChecksumEntries -ManifestPath $manifestPath)
        $assetPattern = '^tadx_([^_]+)_windows_' + [regex]::Escape($architecture) + '\.zip$'
        $candidateEntries = @($entries | Where-Object { $_.File -match $assetPattern })
        if ($candidateEntries.Count -ne 1) {
            throw "The latest stable release does not contain exactly one Windows $architecture archive."
        }
        $assetName = $candidateEntries[0].File
        if ($assetName -notmatch '^tadx_([^_]+)_windows_') {
            throw 'The release asset name does not contain a version.'
        }
        $resolvedVersion = $Matches[1]
        if ($resolvedVersion -notmatch '^[0-9]+\.[0-9]+\.[0-9]+(?:[-+][0-9A-Za-z.-]+)?$') {
            throw "The release contains an invalid TADX version: $resolvedVersion"
        }
    }
    else {
        $resolvedVersion = $Version.TrimStart('v')
        if ($resolvedVersion -notmatch '^[0-9]+\.[0-9]+\.[0-9]+(?:[-+][0-9A-Za-z.-]+)?$') {
            throw "Invalid TADX version: $Version"
        }
        $assetName = "tadx_${resolvedVersion}_windows_${architecture}.zip"
        $tagCandidates = @($Version, "v$resolvedVersion", $resolvedVersion) | Select-Object -Unique
        $downloaded = $false
        foreach ($tag in $tagCandidates) {
            try {
                $releaseBase = "https://github.com/$Repository/releases/download/$tag"
                Invoke-Download -Uri "$releaseBase/checksums.txt" -Destination $manifestPath
                $downloaded = $true
                break
            }
            catch {
                Remove-Item -LiteralPath $manifestPath -Force -ErrorAction SilentlyContinue
            }
        }
        if (-not $downloaded) {
            throw "TADX release $Version was not found."
        }
        $entries = @(Get-ChecksumEntries -ManifestPath $manifestPath)
    }

    $archivePath = Join-Path $temporaryDirectory $assetName
    Invoke-Download -Uri "$releaseBase/$assetName" -Destination $archivePath
    Assert-ArchiveChecksum -ArchivePath $archivePath -AssetName $assetName -Entries $entries

    $extractDirectory = Join-Path $temporaryDirectory 'extract'
    Expand-Archive -LiteralPath $archivePath -DestinationPath $extractDirectory
    $binaries = @(Get-ChildItem -LiteralPath $extractDirectory -Recurse -File -Filter 'tadx.exe')
    if ($binaries.Count -ne 1) {
        throw 'The verified release archive must contain exactly one tadx.exe file.'
    }

    Install-TadxBinary -Source $binaries[0].FullName -Directory $InstallDir
    if (-not $NoModifyPath) {
        Add-UserPath -Directory $InstallDir
    }

    Write-Host "TADX $resolvedVersion was installed at $(Join-Path $InstallDir 'tadx.exe')."
    if ($NoModifyPath) {
        Write-Host "Add $InstallDir to PATH to run tadx from any directory."
    }
    else {
        Write-Host 'Open a new terminal if the tadx command is not available in the current terminal.'
    }
}
finally {
    Remove-Item -LiteralPath $temporaryDirectory -Recurse -Force -ErrorAction SilentlyContinue
}
