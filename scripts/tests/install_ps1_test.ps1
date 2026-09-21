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
    $installDirectory = Join-Path $testRoot "install space's"
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

    $fakeSource = @'
package main
import ("os"; "strings"; "os/exec")
func main() {
 if len(os.Args)>1 && os.Args[1]=="selfupdate" {
  c:=exec.Command("powershell.exe",os.Args[2:]...)
  // Match the updater's native boundary: Windows PowerShell rebuilds its module paths.
  for _,entry:=range os.Environ(){key,_,_:=strings.Cut(entry,"=");if !strings.EqualFold(key,"PSModulePath"){c.Env=append(c.Env,entry)}}
  c.Stdout=os.Stdout;c.Stderr=os.Stderr;if e:=c.Run();e!=nil{os.Exit(1)};return
 }
 if len(os.Args)>1 && os.Args[1]=="agent" {
  f,e:=os.OpenFile(os.Getenv("TADX_TEST_GUIDANCE_LOG"),os.O_APPEND|os.O_CREATE|os.O_WRONLY,0600); if e!=nil {panic(e)}
  _,_=f.WriteString(strings.Join(os.Args[1:]," ")+"\n"); _=f.Close()
  if os.Getenv("TADX_TEST_GUIDANCE_FAIL")=="1" {os.Exit(7)}
 }
}
'@
    $fakeSourcePath = Join-Path $testRoot 'fake.go'
    Set-Content -LiteralPath $fakeSourcePath -Value $fakeSource
    & go build -o (Join-Path $payloadDirectory 'tadx.exe') $fakeSourcePath
    Assert-True -Condition ($LASTEXITCODE -eq 0) -Message 'Could not build the isolated installer fixture.'
    $env:TADX_TEST_GUIDANCE_LOG = Join-Path $testRoot 'guidance.log'
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
    $completionProfile = Join-Path $testRoot 'profiles/profile.ps1'
    New-Item -ItemType Directory -Path (Split-Path -Parent $completionProfile) | Out-Null
    [IO.File]::WriteAllText($completionProfile, "# user profile`r`n")

    # This inert fixture is beside the test binary, never at the system policy path.
    New-Item -ItemType Directory -Path $installDirectory | Out-Null
    $policyFixture = Join-Path $installDirectory 'managed-policy.json'
    [IO.File]::WriteAllText($policyFixture, '{"version":1,"allowed_capabilities":[],"remote_mutations":false}')
    $policyHash = (Get-FileHash -LiteralPath $policyFixture -Algorithm SHA256).Hash
    $policySecurity = (Get-Acl -LiteralPath $policyFixture).Sddl
    function Assert-PolicyPreserved {
        Assert-True -Condition ((Get-FileHash -LiteralPath $policyFixture -Algorithm SHA256).Hash -ceq $policyHash) -Message 'Installer changed managed policy fixture bytes.'
        Assert-True -Condition ((Get-Acl -LiteralPath $policyFixture).Sddl -ceq $policySecurity) -Message 'Installer changed managed policy owner or ACL.'
    }

    & $installer -Version latest -InstallDir $installDirectory -NoModifyPath -CompletionProfile $completionProfile
    Assert-PolicyPreserved
    Assert-True -Condition (Test-Path -LiteralPath (Join-Path $installDirectory 'tadx.exe')) -Message 'Latest installation did not write tadx.exe.'
    Assert-True -Condition ([IO.File]::ReadAllText($env:TADX_TEST_GUIDANCE_LOG).Contains('agent install --target auto')) -Message 'Single install did not deploy Guidance.'
    & (Join-Path $installDirectory 'tadx.exe') selfupdate -NoProfile -NonInteractive -ExecutionPolicy Bypass -File $installer -Version 1.2.3 -InstallDir $installDirectory -NoModifyPath -NoCompletion
    Assert-True -Condition ($LASTEXITCODE -eq 0) -Message 'Could not update while the old executable remained running.'
    Assert-PolicyPreserved
    $originalBinaryHash = (Get-FileHash -LiteralPath (Join-Path $installDirectory 'tadx.exe')).Hash
    $env:TADX_TEST_GUIDANCE_FAIL = '1'
    $failed = $false
    try { & $installer -Version latest -InstallDir $installDirectory -NoModifyPath -NoCompletion } catch { $failed = $true }
    Remove-Item Env:TADX_TEST_GUIDANCE_FAIL
    Assert-True -Condition $failed -Message 'Guidance failure was reported as successful installation.'
    Assert-PolicyPreserved
    Assert-True -Condition ((Get-FileHash -LiteralPath (Join-Path $installDirectory 'tadx.exe')).Hash -eq $originalBinaryHash) -Message 'Guidance failure did not restore the prior binary.'
    $firstProfile = [IO.File]::ReadAllText($completionProfile)
    $parseTokens = $null
    $parseErrors = $null
    [Management.Automation.Language.Parser]::ParseInput($firstProfile, [ref]$parseTokens, [ref]$parseErrors) | Out-Null
    Assert-True -Condition ($parseErrors.Count -eq 0) -Message 'Generated PowerShell profile does not parse.'
    Assert-True -Condition ($firstProfile.Contains('completion powershell')) -Message 'Completion was not enabled by default.'
    Assert-True -Condition ([IO.File]::ReadAllText("$completionProfile.tadx-backup") -eq "# user profile`r`n") -Message 'Profile backup is not recoverable.'

    & $installer -Version 1.2.3 -InstallDir $installDirectory -NoModifyPath -CompletionProfile $completionProfile
    Assert-True -Condition ([IO.File]::ReadAllText($completionProfile) -eq $firstProfile) -Message 'Repeated completion setup is not idempotent.'
    $log = Get-Content -Raw -LiteralPath $env:TADX_TEST_GH_LOG
    Assert-True -Condition ($log.Contains('auth status --hostname github.com')) -Message 'The installer did not check GitHub CLI authentication.'
    Assert-True -Condition ($log.Contains('release download')) -Message 'The installer did not use GitHub CLI release downloads.'
    & $installer -Action Uninstall -InstallDir $installDirectory -NoModifyPath -CompletionProfile $completionProfile
    Assert-PolicyPreserved
    Assert-True -Condition ([IO.File]::ReadAllText($completionProfile) -eq "# user profile`r`n") -Message 'Uninstall changed unrelated profile content.'
    & $installer -Version 1.2.3 -InstallDir $installDirectory -NoModifyPath -NoCompletion -CompletionProfile $completionProfile
    Assert-True -Condition ([IO.File]::ReadAllText($completionProfile) -eq "# user profile`r`n") -Message 'Completion opt-out changed the profile.'

    # ParseFile uses the actual host's script decoding, unlike ReadAllText.
    # Keep this fixture source ASCII so Windows PowerShell can load the test.
    $unicodeDirectory = Join-Path $testRoot ("install space's " + [char]0x4e2d + [char]0x6587)
    $unicodeText = 'caf' + [char]0xe9
    $unicodeProfile = "`$tadxFixture = '$unicodeText'`r`n"
    if ($PSVersionTable.PSVersion.Major -ge 6) {
        [Text.Encoding]::RegisterProvider([Text.CodePagesEncodingProvider]::Instance)
    }
    $encodingCases = @(
        @{ Name = 'utf8-bom'; Encoding = New-Object Text.UTF8Encoding($true) },
        @{ Name = 'utf16-le'; Encoding = [Text.Encoding]::Unicode },
        @{ Name = 'utf16-be'; Encoding = [Text.Encoding]::BigEndianUnicode },
        @{ Name = 'utf32-le'; Encoding = [Text.Encoding]::UTF32 },
        @{ Name = 'utf32-be'; Encoding = New-Object Text.UTF32Encoding($true, $true) },
        @{ Name = 'legacy-ansi'; Encoding = [Text.Encoding]::GetEncoding(0) },
        @{ Name = 'new'; Encoding = $null }
    )
    if ($PSVersionTable.PSVersion.Major -ge 6) {
        $encodingCases += @{ Name = 'utf8-no-bom'; Encoding = New-Object Text.UTF8Encoding($false) }
    }
    foreach ($case in $encodingCases) {
        $caseProfile = Join-Path $testRoot ('profiles/' + $case.Name + '.ps1')
        $originalBytes = $null
        if ($null -ne $case.Encoding) {
            [IO.File]::WriteAllText($caseProfile, $unicodeProfile, $case.Encoding)
            $originalBytes = [IO.File]::ReadAllBytes($caseProfile)
        }
        & $installer -Version 1.2.3 -InstallDir $unicodeDirectory -NoModifyPath -CompletionProfile $caseProfile
        $installedBytes = [IO.File]::ReadAllBytes($caseProfile)
        $profileAST = [Management.Automation.Language.Parser]::ParseFile($caseProfile, [ref]$parseTokens, [ref]$parseErrors)
        Assert-True -Condition ($parseErrors.Count -eq 0) -Message ($case.Name + ': installed profile does not parse.')
        $literals = @($profileAST.FindAll({ param($node) $node -is [Management.Automation.Language.StringConstantExpressionAst] }, $true) | ForEach-Object { $_.Value })
        Assert-True -Condition ($literals -ccontains (Join-Path $unicodeDirectory 'tadx.exe')) -Message ($case.Name + ': Unicode completion path was corrupted by host decoding.')
        if ($null -ne $originalBytes) {
            Assert-True -Condition ($literals -ccontains $unicodeText) -Message ($case.Name + ': existing profile text was corrupted.')
            Assert-True -Condition ([Convert]::ToBase64String([IO.File]::ReadAllBytes("$caseProfile.tadx-backup")) -ceq [Convert]::ToBase64String($originalBytes)) -Message ($case.Name + ': original bytes were not backed up.')
            $preamble = $case.Encoding.GetPreamble()
            if ($preamble.Length -gt 0) {
                Assert-True -Condition ([Convert]::ToBase64String($installedBytes[0..($preamble.Length - 1)]) -ceq [Convert]::ToBase64String($preamble)) -Message ($case.Name + ': existing BOM encoding was not preserved.')
            }
        }
        & $installer -Version 1.2.3 -InstallDir $unicodeDirectory -NoModifyPath -CompletionProfile $caseProfile
        Assert-True -Condition ([Convert]::ToBase64String([IO.File]::ReadAllBytes($caseProfile)) -ceq [Convert]::ToBase64String($installedBytes)) -Message ($case.Name + ': reinstall changed profile bytes.')
        $addedText = "# user edit after installing $unicodeText`r`n"
        $updatedEncoding = New-Object Text.UTF8Encoding($true)
        if ($null -ne $case.Encoding -and $case.Encoding.GetPreamble().Length -gt 0) { $updatedEncoding = $case.Encoding }
        [IO.File]::WriteAllText($caseProfile, ([IO.File]::ReadAllText($caseProfile) + $addedText), $updatedEncoding)
        & $installer -Action Uninstall -InstallDir $unicodeDirectory -NoModifyPath -CompletionProfile $caseProfile
        $remaining = [IO.File]::ReadAllText($caseProfile)
        $expected = if ($null -eq $originalBytes) { $addedText } else { $unicodeProfile + $addedText }
        Assert-True -Condition ($remaining -ceq $expected) -Message ($case.Name + ': uninstall changed existing text.')
        $uninstalledBytes = [IO.File]::ReadAllBytes($caseProfile)
        & $installer -Action Uninstall -InstallDir $unicodeDirectory -NoModifyPath -CompletionProfile $caseProfile
        Assert-True -Condition ([Convert]::ToBase64String([IO.File]::ReadAllBytes($caseProfile)) -ceq [Convert]::ToBase64String($uninstalledBytes)) -Message ($case.Name + ': repeated uninstall changed profile bytes.')
    }

    Write-Output 'install.ps1 tests passed'
}
finally {
    $env:Path = $originalPath
    $env:TADX_TEST_RELEASES = $originalReleaseDirectory
    $env:TADX_TEST_GH_LOG = $originalLog
    Remove-Item -LiteralPath $testRoot -Recurse -Force -ErrorAction SilentlyContinue
}
