# Build dist/siroc-linux-amd64.tar.gz on Windows (Linux amd64 binaries + web UI).
$ErrorActionPreference = "Stop"
$Root = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$Out = if ($args.Count -ge 1 -and $args[0]) { $args[0] } else { Join-Path $Root "dist\siroc-linux-amd64.tar.gz" }

function Find-Python {
    foreach ($c in @("python", "py")) {
        $cmd = Get-Command $c -ErrorAction SilentlyContinue
        if ($cmd) { return $cmd.Source }
    }
    throw "Python 3 is required to pack the tarball. Install python.org or `py`."
}

$env:GOTOOLCHAIN = "local"
$Ver = (Get-Content (Join-Path $Root "VERSION") -Raw).Trim()
if (-not $Ver) { $Ver = "0.0.0" }

Write-Host "Building web UI..."
Push-Location (Join-Path $Root "web")
try {
    npm install
    npm run build
} finally {
    Pop-Location
}

Write-Host "Building Linux binaries (v$Ver)..."
New-Item -ItemType Directory -Force -Path (Join-Path $Root "bin") | Out-Null
$ld = "-s -w -X github.com/siroc-dev/siroc/internal/version.Version=$Ver"
$env:GOOS = "linux"
$env:GOARCH = "amd64"
$env:CGO_ENABLED = "0"
Push-Location $Root
try {
    go build -trimpath -ldflags $ld -o (Join-Path $Root "bin\siroc-agent") ./cmd/agent
    go build -trimpath -ldflags $ld -o (Join-Path $Root "bin\siroc-panel") ./cmd/panel
} finally {
    Pop-Location
    Remove-Item Env:GOOS -ErrorAction SilentlyContinue
    Remove-Item Env:GOARCH -ErrorAction SilentlyContinue
    Remove-Item Env:CGO_ENABLED -ErrorAction SilentlyContinue
}

$Stage = Join-Path ([System.IO.Path]::GetTempPath()) ("siroc-pkg-" + [guid]::NewGuid().ToString("n"))
$Pkg = Join-Path $Stage "siroc"
New-Item -ItemType Directory -Force -Path (Join-Path $Pkg "bin"), (Join-Path $Pkg "web") | Out-Null
Copy-Item (Join-Path $Root "bin\siroc-agent") (Join-Path $Pkg "bin\siroc-agent")
Copy-Item (Join-Path $Root "bin\siroc-panel") (Join-Path $Pkg "bin\siroc-panel")
Copy-Item (Join-Path $Root "scripts\siroc") (Join-Path $Pkg "bin\siroc")
Copy-Item (Join-Path $Root "scripts\install.sh") (Join-Path $Pkg "install.sh")
Copy-Item (Join-Path $Root "web\dist") (Join-Path $Pkg "web\dist") -Recurse
Set-Content -Path (Join-Path $Pkg "VERSION") -Value $Ver -NoNewline -Encoding ascii
$lspMods = Join-Path $Root "lsp\node_modules"
if (Test-Path $lspMods) {
    New-Item -ItemType Directory -Force -Path (Join-Path $Pkg "lsp") | Out-Null
    $lspPkg = Join-Path $Root "lsp\package.json"
    if (Test-Path $lspPkg) { Copy-Item $lspPkg (Join-Path $Pkg "lsp\package.json") }
    Copy-Item $lspMods (Join-Path $Pkg "lsp\node_modules") -Recurse
}

$py = Find-Python
$pyArgs = @()
if ((Split-Path $py -Leaf) -eq "py.exe" -or (Split-Path $py -Leaf) -eq "py") { $pyArgs = @("-3") }
& $py @pyArgs (Join-Path $Root "scripts\pack-tarball.py") $Stage $Out
Remove-Item -Recurse -Force $Stage
Write-Host "On the server: tar -xzf $(Split-Path $Out -Leaf) && cd siroc && sudo ./install.sh"
