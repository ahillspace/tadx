<# Process-only transport boundary for the exact hosted installer fixture. #>
[CmdletBinding()]
param(
    [ValidateSet('Install', 'Uninstall')][string]$Action = 'Install',
    [string]$Version = '',
    [string]$InstallDir = '',
    [string[]]$Target = @('auto'),
    [switch]$NoModifyPath,
    [switch]$NoCompletion,
    [string]$CompletionProfile = ''
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

function Invoke-WebRequest {
    param(
        [switch]$UseBasicParsing,
        [Parameter(Mandatory = $true)][string]$Uri,
        [Parameter(Mandatory = $true)][string]$OutFile,
        [int]$TimeoutSec
    )

    if (-not $UseBasicParsing -or $TimeoutSec -ne 120) {
        throw 'Installer HTTPS request differs from its bounded fixture contract.'
    }
    $helper = $env:G9_ASSET_HELPER
    if ([string]::IsNullOrWhiteSpace($helper) -or -not (Test-Path -LiteralPath $helper -PathType Leaf)) {
        throw 'Installer HTTPS fixture helper is missing.'
    }
    & python $helper https $Uri $OutFile
    if ($LASTEXITCODE -ne 0) {
        throw 'Installer HTTPS fixture recorded a bounded transport failure.'
    }
}

$bound = Get-Command Invoke-WebRequest -ErrorAction Stop
if ($bound.CommandType -ne 'Function') {
    throw 'Installer HTTPS fixture function is not bound.'
}

$installer = Join-Path (Split-Path -Parent $PSScriptRoot) 'install.ps1'
& $installer @PSBoundParameters
