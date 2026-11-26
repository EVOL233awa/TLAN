param(
    [switch]$Force
)

$ErrorActionPreference = 'Stop'

$root = Split-Path -Parent $PSScriptRoot
$frpVersion = '0.71.1'
$archiveUrl = "https://github.com/fatedier/frp/releases/download/v$frpVersion/frp_${frpVersion}_windows_amd64.zip"
$expectedFrpcSHA256 = '509F4FA082BF9B85F9AD27B45D37A89A8ACCB011F3B724CFF352DFAC90A94FEB'
$targetDir = Join-Path $root 'assets\frp\windows-amd64'
$targetPath = Join-Path $targetDir 'frpc.exe'

if ((Test-Path $targetPath) -and (-not $Force)) {
    $actual = (Get-FileHash -Algorithm SHA256 $targetPath).Hash.ToUpperInvariant()
    if ($actual -eq $expectedFrpcSHA256) {
        Write-Host "Verified existing frpc.exe: $targetPath"
        return
    }
}

$tmpDir = Join-Path $root '.tmp-build\frp-core-fetch'
Remove-Item -Recurse -Force -ErrorAction SilentlyContinue $tmpDir
New-Item -ItemType Directory -Force -Path $tmpDir, $targetDir | Out-Null

$zipPath = Join-Path $tmpDir "frp_${frpVersion}_windows_amd64.zip"
$extractDir = Join-Path $tmpDir 'extract'
try {
    Write-Host "Downloading frp v$frpVersion from $archiveUrl"
    Invoke-WebRequest -UseBasicParsing -Uri $archiveUrl -OutFile $zipPath
    Expand-Archive -Path $zipPath -DestinationPath $extractDir -Force

    $extracted = Get-ChildItem -Path $extractDir -Filter 'frpc.exe' -Recurse | Select-Object -First 1
    if (-not $extracted) {
        throw 'frpc.exe was not found in downloaded archive.'
    }

    $actual = (Get-FileHash -Algorithm SHA256 $extracted.FullName).Hash.ToUpperInvariant()
    if ($actual -ne $expectedFrpcSHA256) {
        Remove-Item -Force -ErrorAction SilentlyContinue $targetPath
        throw "frpc.exe SHA256 mismatch: got $actual, want $expectedFrpcSHA256"
    }

    Copy-Item -Force $extracted.FullName $targetPath
    Write-Host "Fetched and verified frpc.exe ($actual): $targetPath"
}
finally {
    Remove-Item -Recurse -Force -ErrorAction SilentlyContinue $tmpDir
}
