$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

function Assert-True {
    param(
        [Parameter(Mandatory = $true)][bool]$Condition,
        [Parameter(Mandatory = $true)][string]$Message
    )
    if (-not $Condition) {
        throw $Message
    }
}

$testRoot = Join-Path ([IO.Path]::GetTempPath()) ('tadx-installer-test-' + [Guid]::NewGuid().ToString('N'))
$originalPath = $env:Path
$originalReleaseDirectory = $env:TADX_TEST_RELEASES
$originalLog = $env:TADX_TEST_GH_LOG

try {
    $releaseDirectory = Join-Path $testRoot 'releases'
    $payloadDirectory = Join-Path $testRoot 'payload'
    $fakeBin = Join-Path $testRoot 'fake-bin'
    $installDirectory = Join-Path $testRoot 'install'
    New-Item -ItemType Directory -Path $releaseDirectory, $payloadDirectory, $fakeBin | Out-Null

    $architecture = $env:PROCESSOR_ARCHITEW6432
    if ([string]::IsNullOrWhiteSpace($architecture)) {
        $architecture = $env:PROCESSOR_ARCHITECTURE
    }
    if ($architecture -match '^(ARM64|aarch64)$') {
        $assetArchitecture = 'arm64'
    }
    else {
        $assetArchitecture = 'amd64'
    }

    Set-Content -LiteralPath (Join-Path $payloadDirectory 'tadx.exe') -Value 'test executable' -NoNewline
    $assetName = "tadx_1.2.3_windows_${assetArchitecture}.zip"
    $archivePath = Join-Path $releaseDirectory $assetName
    Compress-Archive -LiteralPath (Join-Path $payloadDirectory 'tadx.exe') -DestinationPath $archivePath
    $hash = (Get-FileHash -Algorithm SHA256 -LiteralPath $archivePath).Hash.ToLowerInvariant()
    Set-Content -LiteralPath (Join-Path $releaseDirectory 'checksums.txt') -Value "$hash  $assetName"

    $fakeGitHubCLI = @'
@echo off
echo %*>>"%TADX_TEST_GH_LOG%"
if "%1"=="auth" exit /b 0
set pattern=
set destination=
:args
if "%1"=="" goto copy
if "%1"=="--pattern" (
  set pattern=%~2
  shift
  shift
  goto args
)
if "%1"=="--output" (
  set destination=%~2
  shift
  shift
  goto args
)
shift
goto args
:copy
copy /Y "%TADX_TEST_RELEASES%\%pattern%" "%destination%" >nul
'@
    Set-Content -LiteralPath (Join-Path $fakeBin 'gh.cmd') -Value $fakeGitHubCLI

    $env:TADX_TEST_RELEASES = $releaseDirectory
    $env:TADX_TEST_GH_LOG = Join-Path $testRoot 'gh.log'
    $env:Path = "$fakeBin;$originalPath"
    $installer = Join-Path (Split-Path -Parent $PSScriptRoot) 'install.ps1'

    & $installer -Version latest -InstallDir $installDirectory -NoModifyPath
    Assert-True -Condition (Test-Path -LiteralPath (Join-Path $installDirectory 'tadx.exe')) -Message 'Latest installation did not write tadx.exe.'

    & $installer -Version 1.2.3 -InstallDir $installDirectory -NoModifyPath
    $log = Get-Content -Raw -LiteralPath $env:TADX_TEST_GH_LOG
    Assert-True -Condition ($log.Contains('auth status --hostname github.com')) -Message 'The installer did not check GitHub CLI authentication.'
    Assert-True -Condition ($log.Contains('release download')) -Message 'The installer did not use GitHub CLI release downloads.'

    Write-Output 'install.ps1 tests passed'
}
finally {
    $env:Path = $originalPath
    $env:TADX_TEST_RELEASES = $originalReleaseDirectory
    $env:TADX_TEST_GH_LOG = $originalLog
    Remove-Item -LiteralPath $testRoot -Recurse -Force -ErrorAction SilentlyContinue
}
