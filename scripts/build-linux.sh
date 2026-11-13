#!/bin/sh
# SPDX-License-Identifier: GPL-3.0-or-later
# Builds the Linux variant for amd64 and arm64 (pure Go, no cgo):
#   build/UnisonCue-linux-<arch>.tar.gz   binary, menu entry, icons, install.sh
#   build/unison-cue_<arch>.deb           Debian/Ubuntu package (needs dpkg-deb)
set -eu
export COPYFILE_DISABLE=1  # no macOS ._ files in the tarball
cd "$(dirname "$0")/.."

VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo 0.1.0)}"
VERSION="${VERSION#v}"
# Debian versions must start with a digit.
case "$VERSION" in
  [0-9]*) DEBVERSION="$VERSION" ;;
  *)      DEBVERSION="0.0.0+git$(echo "$VERSION" | tr -c 'A-Za-z0-9.+~\n' '.')" ;;
esac

mkdir -p build
rm -rf build/hicolor
go run ./cmd/mkicons -hicolor build/hicolor

for arch in amd64 arm64; do
  stage="build/linux-$arch"
  rm -rf "$stage"
  mkdir -p "$stage"
  GOOS=linux GOARCH=$arch CGO_ENABLED=0 \
    go build -trimpath -ldflags "-s -w -X main.version=$VERSION" -o "$stage/bin/unison-cue" .

  # Tarball for any distribution.
  tdir="$stage/unison-cue"
  mkdir -p "$tdir/icons"
  cp "$stage/bin/unison-cue" packaging/linux/unison-cue.desktop packaging/linux/install.sh LICENSE "$tdir/"
  cp -R build/hicolor "$tdir/icons/"
  chmod 0755 "$tdir/install.sh" "$tdir/unison-cue"
  tar -C "$stage" -czf "build/UnisonCue-linux-$arch.tar.gz" unison-cue

  # Debian package.
  if command -v dpkg-deb >/dev/null; then
    root="$stage/deb"
    mkdir -p "$root/DEBIAN" "$root/usr/bin" "$root/usr/share/applications" \
             "$root/usr/share/icons" "$root/usr/share/doc/unison-cue"
    install -m 0755 "$stage/bin/unison-cue" "$root/usr/bin/unison-cue"
    install -m 0644 packaging/linux/unison-cue.desktop "$root/usr/share/applications/"
    cp -R build/hicolor "$root/usr/share/icons/"
    install -m 0644 packaging/linux/copyright "$root/usr/share/doc/unison-cue/copyright"
    find "$root/usr" -type d -exec chmod 0755 {} +
    find "$root/usr/share" -type f -exec chmod 0644 {} +
    size=$(du -sk "$root/usr" | cut -f1)
    cat > "$root/DEBIAN/control" <<CONTROL
Package: unison-cue
Version: $DEBVERSION
Architecture: $arch
Maintainer: Håvard Dagenborg <haavardj@users.noreply.github.com>
Installed-Size: $size
Recommends: unison, zenity, gnome-shell-extension-appindicator | libayatana-appindicator3-1
Section: utils
Priority: optional
Homepage: https://github.com/haavardj/unison-cue
Description: tray app for the Unison file synchronizer
 Unison Cue sits in the system tray and runs your Unison profiles when you
 ask. It tracks when each profile was last synced and reminds you when a
 sync is due. Conflicts, ssh passwords and settings are handled from the
 tray menu.
CONTROL
    dpkg-deb --root-owner-group -Zxz --build "$root" "build/unison-cue_$arch.deb" >/dev/null
  else
    echo "dpkg-deb not found: skipping the $arch .deb (the tarball was built)"
  fi
done
echo "Built Linux packages ($VERSION)"
