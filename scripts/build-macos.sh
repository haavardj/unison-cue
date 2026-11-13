#!/bin/sh
# SPDX-License-Identifier: GPL-3.0-or-later
# Builds "build/Unison Cue.app": a universal (arm64 + x86_64) menu bar app
# with no Dock icon, ad-hoc signed, plus a zip for distribution.
set -eu
cd "$(dirname "$0")/.."

VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo 0.1.0)}"
APP="build/Unison Cue.app"
BIN="UnisonCue"

rm -rf "$APP" build/AppIcon.iconset
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"

for arch in arm64 amd64; do
  CGO_ENABLED=1 GOOS=darwin GOARCH=$arch MACOSX_DEPLOYMENT_TARGET=13.0 \
    go build -trimpath -ldflags "-s -w -X main.version=$VERSION" -o "build/$BIN-$arch" .
done
lipo -create -output "$APP/Contents/MacOS/$BIN" "build/$BIN-arm64" "build/$BIN-amd64"
rm "build/$BIN-arm64" "build/$BIN-amd64"

go run ./cmd/mkicons -iconset build/AppIcon.iconset
iconutil -c icns -o "$APP/Contents/Resources/AppIcon.icns" build/AppIcon.iconset
rm -rf build/AppIcon.iconset

cat > "$APP/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleName</key>               <string>Unison Cue</string>
	<key>CFBundleDisplayName</key>        <string>Unison Cue</string>
	<key>CFBundleIdentifier</key>         <string>io.github.haavardj.unisoncue</string>
	<key>CFBundleExecutable</key>         <string>$BIN</string>
	<key>CFBundleIconFile</key>           <string>AppIcon</string>
	<key>CFBundlePackageType</key>        <string>APPL</string>
	<key>CFBundleShortVersionString</key> <string>$VERSION</string>
	<key>CFBundleVersion</key>            <string>$VERSION</string>
	<key>LSMinimumSystemVersion</key>     <string>13.0</string>
	<key>LSUIElement</key>                <true/>
	<key>NSHighResolutionCapable</key>    <true/>
	<key>NSAppleEventsUsageDescription</key>
	<string>Unison Cue opens Terminal to run interactive syncs.</string>
</dict>
</plist>
PLIST

# Ad-hoc signature. For distribution to other Macs, sign with a Developer
# ID and notarize instead:
#   codesign --force --options runtime --sign "Developer ID Application: ..." "$APP"
codesign --force --deep --sign - "$APP"

(cd build && rm -f UnisonCue-macOS.zip && ditto -c -k --keepParent "Unison Cue.app" UnisonCue-macOS.zip)
echo "Built $APP ($VERSION)"
