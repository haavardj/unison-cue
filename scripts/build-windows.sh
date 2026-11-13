#!/bin/sh
# SPDX-License-Identifier: GPL-3.0-or-later
# Cross-compiles the Windows variant from macOS or Linux (no cgo needed):
#   build/UnisonCue.exe        (x64)
#   build/UnisonCue-arm64.exe  (ARM64)
#
# The tray icon is drawn at runtime. To also give the .exe file itself an
# icon in Explorer, set EMBED_ICON=1; this runs the third-party tool
# github.com/tc-hib/go-winres to produce a Windows resource file.
set -eu
cd "$(dirname "$0")/.."

VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo 0.1.0)}"
mkdir -p build
go run ./cmd/mkicons -ico build/UnisonCue.ico

if [ "${EMBED_ICON:-0}" = 1 ]; then
  go run github.com/tc-hib/go-winres@v0.3.3 simply \
    --icon build/UnisonCue.ico --manifest gui \
    --product-name "Unison Cue" --file-description "Unison Cue" \
    --product-version "$VERSION" --file-version "$VERSION" \
    --arch amd64,arm64 --out rsrc
fi

for arch in amd64 arm64; do
  out=build/UnisonCue.exe
  [ "$arch" = arm64 ] && out=build/UnisonCue-arm64.exe
  GOOS=windows GOARCH=$arch CGO_ENABLED=0 \
    go build -trimpath -ldflags "-s -w -H windowsgui -X main.version=$VERSION" -o "$out" .
done
rm -f rsrc_windows_*.syso
echo "Built build/UnisonCue.exe and build/UnisonCue-arm64.exe ($VERSION)"
