# SPDX-License-Identifier: GPL-3.0-or-later
# Native build on Windows (needs Go only):
#   powershell -ExecutionPolicy Bypass -File scripts\build-windows.ps1
# Set $env:EMBED_ICON = "1" to embed the icon in the .exe with go-winres.
$ErrorActionPreference = "Stop"
Set-Location (Join-Path $PSScriptRoot "..")
$version = if ($env:VERSION) { $env:VERSION } else { "0.1.0" }
New-Item -ItemType Directory -Force build | Out-Null
go run ./cmd/mkicons -ico build/UnisonCue.ico
if ($env:EMBED_ICON -eq "1") {
  go run github.com/tc-hib/go-winres@v0.3.3 simply --icon build/UnisonCue.ico --manifest gui `
    --product-name "Unison Cue" --file-description "Unison Cue" `
    --product-version $version --file-version $version --arch amd64 --out rsrc
}
$env:CGO_ENABLED = "0"
go build -trimpath -ldflags "-s -w -H windowsgui -X main.version=$version" -o build/UnisonCue.exe .
Remove-Item -ErrorAction SilentlyContinue rsrc_windows_*.syso
Write-Host "Built build\UnisonCue.exe ($version)"
