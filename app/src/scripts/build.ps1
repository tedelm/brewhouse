# Build Brewhouse WASM + Windows/Linux amd64 servers and package release zips.
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
Remove-Item Env:GOOS -ErrorAction SilentlyContinue
Remove-Item Env:GOARCH -ErrorAction SilentlyContinue

$wasmExecSrc = Join-Path (go env GOROOT) "lib/wasm/wasm_exec.js"
$wasmExecDst = "internal/web/static/js/wasm_exec.js"
if (Test-Path $wasmExecSrc) {
	Copy-Item $wasmExecSrc $wasmExecDst -Force
}

Write-Host "Building Windows amd64 server…"
$env:GOOS = "windows"
$env:GOARCH = "amd64"
go build -ldflags $ldflags -o brewhouse.exe ./cmd
Remove-Item Env:GOOS -ErrorAction SilentlyContinue
Remove-Item Env:GOARCH -ErrorAction SilentlyContinue

Write-Host "Building Linux amd64 server…"
$env:GOOS = "linux"
$env:GOARCH = "amd64"
go build -ldflags $ldflags -o brewhouse ./cmd
Remove-Item Env:GOOS -ErrorAction SilentlyContinue
Remove-Item Env:GOARCH -ErrorAction SilentlyContinue

$distRoot = Join-Path $root "dist"
if (Test-Path $distRoot) {
	Remove-Item $distRoot -Recurse -Force
}
New-Item -ItemType Directory -Path $distRoot | Out-Null

function Write-ReleaseReadme {
	param(
		[string]$Path,
		[string]$BinaryName,
		[string]$StartCmd
	)
	@(
		"Brewhouse $version"
		""
		"1. Copy .env.example to .env and set a strong JWT_SECRET (required for real deployments)."
		"2. Start the server: $StartCmd"
		"3. Open http://localhost:8080 (or the PORT you configured)."
		""
		"On first start the default admin account is created; the generated password is shown"
		"on the login page until the first successful sign-in. Save it securely."
		""
		"Environment variables (see .env.example):"
		"  PORT           default 8080"
		"  DATABASE_PATH  default brewhouse.db"
		"  BACKUP_DIR     default backups"
		"  JWT_SECRET     HMAC secret for JWTs (change in production)"
		"  APP_VERSION    optional override when binary still has placeholder dev"
		""
		"Static assets and templates are embedded in $BinaryName; no extra files are required."
	) -join "`r`n" | Set-Content -Path $Path -Encoding utf8
}

function New-ReleaseZip {
	param(
		[string]$Platform,
		[string]$BinaryName,
		[string]$BinarySrc,
		[string]$StartCmd
	)
	$stageName = "brewhouse-$version-$Platform"
	$stageDir = Join-Path $distRoot $stageName
	New-Item -ItemType Directory -Path $stageDir | Out-Null

	Copy-Item $BinarySrc (Join-Path $stageDir $BinaryName) -Force
	Copy-Item (Join-Path $root ".env.example") (Join-Path $stageDir ".env") -Force
	Copy-Item $versionFile (Join-Path $stageDir "VERSION") -Force
	Write-ReleaseReadme -Path (Join-Path $stageDir "README.txt") -BinaryName $BinaryName -StartCmd $StartCmd

	$zipPath = Join-Path $distRoot "$stageName.zip"
	if (Test-Path $zipPath) {
		Remove-Item $zipPath -Force
	}
	Compress-Archive -Path (Join-Path $stageDir "*") -DestinationPath $zipPath -Force
	Write-Host "Release zip: $zipPath"
	return $zipPath
}

New-ReleaseZip `
	-Platform "windows-amd64" `
	-BinaryName "brewhouse.exe" `
	-BinarySrc (Join-Path $root "brewhouse.exe") `
	-StartCmd ".\brewhouse.exe" | Out-Null

New-ReleaseZip `
	-Platform "linux-amd64" `
	-BinaryName "brewhouse" `
	-BinarySrc (Join-Path $root "brewhouse") `
	-StartCmd "./brewhouse" | Out-Null

Write-Host "Done: WASM embedded, brewhouse.exe, brewhouse (linux), and dist/*.zip (version $version)"
