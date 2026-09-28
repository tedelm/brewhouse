# Build Brewhouse server + WASM with the version from VERSION baked into the binary.
# Run from app/src:  .\scripts\build.ps1
$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

$versionFile = Join-Path $root "VERSION"
if (-not (Test-Path $versionFile)) {
	throw "VERSION file not found at $versionFile"
}
$version = (Get-Content -Raw $versionFile).Trim()
if (-not $version) {
	throw "VERSION file is empty"
}

$ldflags = "-X brewhouse/internal/config.Version=$version"
Write-Host "Building Brewhouse $version"

Write-Host "Building WASM…"
$env:GOOS = "js"
$env:GOARCH = "wasm"
go build -ldflags $ldflags -o internal/web/static/wasm/app.wasm ./cmd/wasm
Remove-Item Env:GOOS
Remove-Item Env:GOARCH

$wasmExecSrc = Join-Path (go env GOROOT) "lib/wasm/wasm_exec.js"
$wasmExecDst = "internal/web/static/js/wasm_exec.js"
if (Test-Path $wasmExecSrc) {
	Copy-Item $wasmExecSrc $wasmExecDst -Force
}

Write-Host "Building server…"
go build -ldflags $ldflags -o brewhouse.exe ./cmd

Write-Host "Done: brewhouse.exe + app.wasm (version $version)"
