[CmdletBinding()]
param(
    [ValidateSet('Install', 'Uninstall')]
    [string]$Action = 'Install',

    [string]$Version = '',

    [string]$InstallDir = '',

    [string[]]$Target = @('auto'),

    [switch]$NoModifyPath,

    [switch]$NoCompletion,

    [string]$CompletionProfile = $PROFILE.CurrentUserAllHosts
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

    Invoke-WebRequest -UseBasicParsing -Uri $Uri -OutFile $Destination -TimeoutSec 120
}

function Test-AuthenticatedGitHubCLI {
    $command = Get-Command 'gh' -ErrorAction SilentlyContinue
    if ($null -eq $command) {
        return $false
    }

    try {
        & $command.Source auth status --hostname github.com *> $null
        return $LASTEXITCODE -eq 0
    }
    catch {
        return $false
    }
}

function Receive-ReleaseAsset {
    param(
        [Parameter(Mandatory = $true)][string]$AssetName,
        [Parameter(Mandatory = $true)][string]$Destination,
        [Parameter(Mandatory = $true)][string]$HttpsBase,
        [string]$Tag = '',
        [bool]$UseGitHubCLI = $false
    )

    Remove-Item -LiteralPath $Destination -Force -ErrorAction SilentlyContinue
    if ($UseGitHubCLI) {
        $arguments = @('release', 'download')
        if (-not [string]::IsNullOrWhiteSpace($Tag)) {
            $arguments += $Tag
        }
        $arguments += @('--repo', $Repository, '--pattern', $AssetName, '--output', $Destination)
        try {
            & gh @arguments *> $null
            if ($LASTEXITCODE -eq 0 -and (Test-Path -LiteralPath $Destination)) {
                return $true
            }
        }
        catch {
            # Continue to the unauthenticated HTTPS fallback.
        }
        Remove-Item -LiteralPath $Destination -Force -ErrorAction SilentlyContinue
    }

    try {
        Invoke-Download -Uri "$HttpsBase/$AssetName" -Destination $Destination
        return $true
    }
    catch {
        Remove-Item -LiteralPath $Destination -Force -ErrorAction SilentlyContinue
        return $false
    }
}

function Get-ChecksumEntries {
    param([Parameter(Mandatory = $true)][string]$ManifestPath)

    if ((Get-Item -LiteralPath $ManifestPath).Length -gt 1048576) {
        throw 'The release checksum manifest exceeds its byte limit.'
    }

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
    $installed = $false
    $hadExisting = Test-Path -LiteralPath $destination

    try {
        Copy-Item -LiteralPath $Source -Destination $staged
        # Renaming, unlike overwriting, also supports the running updater on Windows.
        if ($hadExisting) { [IO.File]::Move($destination, $backup) }
        [IO.File]::Move($staged, $destination)
        $installed = $true
        foreach ($agentTarget in $Target) {
            & $destination agent install --target $agentTarget
            if ($LASTEXITCODE -ne 0) {
                throw 'Guidance installation failed. The binary will be rolled back; any completed Guidance targets are reported above. Retry the installer to complete setup.'
            }
        }
    }
    catch {
        Remove-Item -LiteralPath $staged -Force -ErrorAction SilentlyContinue
        if ($installed -and (Test-Path -LiteralPath $destination)) {
            Remove-Item -LiteralPath $destination -Force
        }
        if (Test-Path -LiteralPath $backup) {
            [IO.File]::Move($backup, $destination)
        }
        throw
    }
    finally {
        Remove-Item -LiteralPath $staged -Force -ErrorAction SilentlyContinue
        # Retain the previous binary for manual recovery, including while it is running.
    }
}

function Read-CompletionProfile {
    param([Parameter(Mandatory = $true)][string]$Path)

    # A BOM makes Unicode hooks readable by both Windows PowerShell and pwsh.
    $utf8 = New-Object Text.UTF8Encoding($true, $true)
    if (-not (Test-Path -LiteralPath $Path)) {
        return [pscustomobject]@{ Text = ''; Encoding = $utf8 }
    }
    $bytes = [IO.File]::ReadAllBytes($Path)
    # Test UTF-32 before UTF-16 because their little-endian BOMs overlap.
    $encodings = @(
        (New-Object Text.UTF32Encoding($false, $true, $true)),
        (New-Object Text.UTF32Encoding($true, $true, $true)),
        $utf8,
        (New-Object Text.UnicodeEncoding($false, $true, $true)),
        (New-Object Text.UnicodeEncoding($true, $true, $true))
    )
    foreach ($encoding in $encodings) {
        $preamble = $encoding.GetPreamble()
        if ($bytes.Length -lt $preamble.Length) { continue }
        $matches = $true
        for ($index = 0; $index -lt $preamble.Length; $index++) {
            if ($bytes[$index] -ne $preamble[$index]) { $matches = $false; break }
        }
        if ($matches) {
            return [pscustomobject]@{
                Text = $encoding.GetString($bytes, $preamble.Length, $bytes.Length - $preamble.Length)
                Encoding = $encoding
            }
        }
    }

    # Windows PowerShell reads BOM-less scripts in the system ANSI code page.
    # pwsh reads UTF-8; legacy ANSI files are still recoverable when UTF-8 fails.
    if ($PSVersionTable.PSVersion.Major -ge 6) {
        try {
            return [pscustomobject]@{ Text = $utf8.GetString($bytes); Encoding = $utf8 }
        }
        catch [Text.DecoderFallbackException] {
            # Decode the original bytes below, without replacement characters.
        }
        [Text.Encoding]::RegisterProvider([Text.CodePagesEncodingProvider]::Instance)
    }
    $ansi = [Text.Encoding]::GetEncoding(0, [Text.EncoderFallback]::ExceptionFallback, [Text.DecoderFallback]::ExceptionFallback)
    return [pscustomobject]@{ Text = $ansi.GetString($bytes); Encoding = $utf8 }
}

function Set-ManagedCompletion {
    param([bool]$Enabled)
    $profilePath = [IO.Path]::GetFullPath($CompletionProfile)
    $marker = '# tadx-installer-completion'
    $profileContent = Read-CompletionProfile -Path $profilePath
    $original = $profileContent.Text
    $updated = [regex]::Replace($original, '(?m)^if \(Test-Path -LiteralPath [^\r\n]* ' + [regex]::Escape($marker) + '\r?$\n?', '')
    if ($Enabled) {
        $binary = (Join-Path $InstallDir 'tadx.exe').Replace("'", "''")
        if ($updated.Length -gt 0 -and -not $updated.EndsWith("`n")) { $updated += "`r`n" }
        $updated += "if (Test-Path -LiteralPath '$binary') { & '$binary' completion powershell | Out-String | Invoke-Expression } $marker`r`n"
    }
    if ($updated -ceq $original) { return }
    New-Item -ItemType Directory -Force -Path (Split-Path -Parent $profilePath) | Out-Null
    if ((Test-Path -LiteralPath $profilePath) -and -not (Test-Path -LiteralPath "$profilePath.tadx-backup")) {
        Copy-Item -LiteralPath $profilePath -Destination "$profilePath.tadx-backup"
    }
    [IO.File]::WriteAllText($profilePath, $updated, $profileContent.Encoding)
}

if ([string]::IsNullOrWhiteSpace($InstallDir)) {
    $InstallDir = Get-DefaultInstallDir
}
$InstallDir = [IO.Path]::GetFullPath($InstallDir)
$Target = @($Target | ForEach-Object { $_ -split ',' })
if ($Target.Count -eq 0 -or @($Target | Where-Object { $_ -notmatch '^[a-z][a-z0-9-]*$' }).Count -gt 0) {
    throw 'Specify one or more valid agent targets.'
}

if ($Action -ieq 'Uninstall') {
    $binaryPath = Join-Path $InstallDir 'tadx.exe'
    if (Test-Path -LiteralPath $binaryPath) {
        Remove-Item -LiteralPath $binaryPath -Force
    }
    if (-not $NoModifyPath) { Remove-UserPath -Directory $InstallDir }
    Set-ManagedCompletion -Enabled $false
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
$installLock = Join-Path $InstallDir '.tadx-install.lock'
$ownsInstallLock = $false

try {
    $manifestPath = Join-Path $temporaryDirectory 'checksums.txt'
    $releaseBase = ''
    $releaseTag = ''
    $useGitHubCLI = Test-AuthenticatedGitHubCLI

    if ($Version -ieq 'latest') {
        $releaseBase = "https://github.com/$Repository/releases/latest/download"
        if (-not (Receive-ReleaseAsset -AssetName 'checksums.txt' -Destination $manifestPath -HttpsBase $releaseBase -UseGitHubCLI $useGitHubCLI)) {
            throw 'The latest stable TADX release could not be downloaded.'
        }
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
            $releaseBase = "https://github.com/$Repository/releases/download/$tag"
            if (Receive-ReleaseAsset -AssetName 'checksums.txt' -Destination $manifestPath -HttpsBase $releaseBase -Tag $tag -UseGitHubCLI $useGitHubCLI) {
                $releaseTag = $tag
                $downloaded = $true
                break
            }
        }
        if (-not $downloaded) {
            throw "TADX release $Version was not found."
        }
        $entries = @(Get-ChecksumEntries -ManifestPath $manifestPath)
    }

    $archivePath = Join-Path $temporaryDirectory $assetName
    if (-not (Receive-ReleaseAsset -AssetName $assetName -Destination $archivePath -HttpsBase $releaseBase -Tag $releaseTag -UseGitHubCLI $useGitHubCLI)) {
        throw "The release asset $assetName could not be downloaded."
    }
    Assert-ArchiveChecksum -ArchivePath $archivePath -AssetName $assetName -Entries $entries
    if ((Get-Item -LiteralPath $archivePath).Length -gt 268435456) {
        throw 'The release archive exceeds its byte limit.'
    }

    $extractDirectory = Join-Path $temporaryDirectory 'extract'
    Expand-Archive -LiteralPath $archivePath -DestinationPath $extractDirectory
    $binaries = @(Get-ChildItem -LiteralPath $extractDirectory -Recurse -File -Filter 'tadx.exe')
    if ($binaries.Count -ne 1) {
        throw 'The verified release archive must contain exactly one tadx.exe file.'
    }

    New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
    try { New-Item -ItemType Directory -Path $installLock -ErrorAction Stop | Out-Null; $ownsInstallLock = $true }
    catch { throw 'Another installer is using this directory. Wait for it to finish; remove .tadx-install.lock only after confirming no installer is running.' }
    Install-TadxBinary -Source $binaries[0].FullName -Directory $InstallDir
    if (-not $NoModifyPath) {
        Add-UserPath -Directory $InstallDir
    }
    if (-not $NoCompletion) {
        Set-ManagedCompletion -Enabled $true
        Write-Host "PowerShell completion is enabled in $CompletionProfile. Open a new PowerShell session to load it."
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
    if ($ownsInstallLock) { Remove-Item -LiteralPath $installLock -Force -ErrorAction SilentlyContinue }
    Remove-Item -LiteralPath $temporaryDirectory -Recurse -Force -ErrorAction SilentlyContinue
}
