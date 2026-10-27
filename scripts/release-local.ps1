# Build the Linux panel package on this machine and upload it to Cloudflare R2
# (bucket siroc-cp / https://get.siroc.dev). Same objects as .github/workflows/release.yml.
#
# Credentials (environment, or a gitignored .r2.env in the repo root):
#   CLOUDFLARE_ACCOUNT_ID
#   R2_ACCESS_KEY_ID
#   R2_SECRET_ACCESS_KEY
#
# Optional: SIROC_RELEASE_PUBLIC_BASE  (default https://get.siroc.dev)
#
# Usage:
#   .\scripts\release-local.ps1
#   .\scripts\release-local.ps1 -SkipBuild
#   .\scripts\release-local.ps1 -SkipUpload
param(
    [switch]$SkipBuild,
    [switch]$SkipUpload,
    [switch]$SkipPrune,
    [string]$Version
)

$ErrorActionPreference = "Stop"
$Root = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
Set-Location $Root

function Load-DotEnv([string]$path) {
    if (-not (Test-Path $path)) { return }
    Get-Content $path | ForEach-Object {
        $line = $_.Trim()
        if (-not $line -or $line.StartsWith("#") -or $line -notmatch "=") { return }
        $i = $line.IndexOf("=")
        $k = $line.Substring(0, $i).Trim()
        $v = $line.Substring($i + 1).Trim().Trim("'").Trim('"')
        if ($k) { Set-Item -Path "Env:$k" -Value $v }
    }
}

function Find-Python {
    foreach ($c in @("python", "py")) {
        $cmd = Get-Command $c -ErrorAction SilentlyContinue
        if ($cmd) { return $cmd.Source }
    }
    throw "Python 3 is required (sha256 metadata and R2 prune)."
}

Load-DotEnv (Join-Path $Root ".r2.env")

$Ver = $Version
if (-not $Ver) { $Ver = (Get-Content (Join-Path $Root "VERSION") -Raw).Trim() }
$Ver = $Ver.TrimStart("v")
if (-not $Ver) { throw "VERSION is empty" }

$Pkg = Join-Path $Root "dist\siroc-linux-amd64.tar.gz"
$Meta = Join-Path $Root "dist\latest.json"
$Base = $env:SIROC_RELEASE_PUBLIC_BASE
if (-not $Base) { $Base = "https://get.siroc.dev" }
$Base = $Base.TrimEnd("/")

if (-not $SkipBuild) {
    Write-Host "=== Build $Ver ==="
    & (Join-Path $Root "scripts\package.ps1") $Pkg
}

if (-not (Test-Path $Pkg)) { throw "missing $Pkg - run without -SkipBuild" }

$py = Find-Python
$pyArgs = @()
if ((Split-Path $py -Leaf) -eq "py.exe" -or (Split-Path $py -Leaf) -eq "py") { $pyArgs = @("-3") }
$sha = (Get-FileHash -Algorithm SHA256 $Pkg).Hash.ToLowerInvariant()
$Url = "$Base/$Ver/siroc-linux-amd64.tar.gz"
$payload = @{ version = $Ver; url = $Url; sha256 = $sha } | ConvertTo-Json
New-Item -ItemType Directory -Force -Path (Split-Path $Meta) | Out-Null
$utf8 = New-Object System.Text.UTF8Encoding $false
[System.IO.File]::WriteAllText($Meta, ($payload.Trim() + "`n"), $utf8)
Write-Host "Published metadata:"
Get-Content $Meta

if ($SkipUpload) {
    Write-Host "Skip upload. Package is at $Pkg"
    return
}

if (-not $env:R2_ACCESS_KEY_ID -or -not $env:R2_SECRET_ACCESS_KEY) {
    throw "Set R2_ACCESS_KEY_ID and R2_SECRET_ACCESS_KEY (or put them in .r2.env)."
}
if (-not $env:CLOUDFLARE_ACCOUNT_ID) {
    throw "Set CLOUDFLARE_ACCOUNT_ID (or put it in .r2.env)."
}
if (-not (Get-Command aws -ErrorAction SilentlyContinue)) {
    throw "AWS CLI v2 is required (aws s3). Install from https://aws.amazon.com/cli/"
}

$Endpoint = "https://$($env:CLOUDFLARE_ACCOUNT_ID).r2.cloudflarestorage.com"
$env:AWS_ACCESS_KEY_ID = $env:R2_ACCESS_KEY_ID
$env:AWS_SECRET_ACCESS_KEY = $env:R2_SECRET_ACCESS_KEY
$env:AWS_DEFAULT_REGION = "auto"

Write-Host "=== Upload to s3://siroc-cp ($Ver) ==="
aws s3 cp $Pkg "s3://siroc-cp/siroc-linux-amd64.tar.gz" --endpoint-url $Endpoint --content-type "application/octet-stream" --cache-control "public, max-age=60"
if ($LASTEXITCODE -ne 0) { throw "aws s3 cp latest tarball failed" }
aws s3 cp $Pkg "s3://siroc-cp/$Ver/siroc-linux-amd64.tar.gz" --endpoint-url $Endpoint --content-type "application/octet-stream" --cache-control "public, max-age=31536000, immutable"
if ($LASTEXITCODE -ne 0) { throw "aws s3 cp versioned tarball failed" }
aws s3 cp $Meta "s3://siroc-cp/latest.json" --endpoint-url $Endpoint --content-type "application/json" --cache-control "no-cache"
if ($LASTEXITCODE -ne 0) { throw "aws s3 cp latest.json failed" }

if ($SkipPrune) {
    Write-Host "Done. https://get.siroc.dev/latest.json"
    return
}

Write-Host "Keeping the 3 newest version prefixes"
$prefixFile = Join-Path $Root "dist\r2-prefixes.txt"
$dropFile = Join-Path $Root "dist\r2-drop.txt"
aws s3api list-objects-v2 --bucket siroc-cp --delimiter "/" --endpoint-url $Endpoint --query "CommonPrefixes[].Prefix" --output text | Set-Content -Path $prefixFile -Encoding ascii
Get-Content $prefixFile | & $py @pyArgs (Join-Path $Root "scripts\prune-r2-versions.py") 3 | Set-Content -Path $dropFile -Encoding ascii
if (-not (Test-Path $dropFile) -or -not (Get-Content $dropFile | Where-Object { $_.Trim() })) {
    Write-Host "Nothing to delete"
    Write-Host "Done. https://get.siroc.dev/latest.json"
    return
}
Get-Content $dropFile | ForEach-Object {
    $prefix = $_.Trim()
    if (-not $prefix) { return }
    Write-Host "Deleting s3://siroc-cp/$prefix"
    aws s3 rm "s3://siroc-cp/$prefix" --recursive --endpoint-url $Endpoint
}
Write-Host "Done. https://get.siroc.dev/latest.json"
