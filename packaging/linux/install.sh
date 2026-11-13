#!/bin/sh
# SPDX-License-Identifier: GPL-3.0-or-later
# Installs Unison Cue for the current user, without root:
#   ~/.local/bin/unison-cue, its menu entry and icons.
# Run ./install.sh --uninstall to remove them again.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
bin="$HOME/.local/bin"
data="${XDG_DATA_HOME:-$HOME/.local/share}"

if [ "${1:-}" = "--uninstall" ]; then
  rm -f "$bin/unison-cue" "$data/applications/unison-cue.desktop"
  find "$data/icons/hicolor" -name 'unison-cue.*' -delete 2>/dev/null || true
  echo "Unison Cue removed. Settings in ~/.config/unison-cue were kept."
  exit 0
fi

mkdir -p "$bin" "$data/applications" "$data/icons"
install -m 0755 "$here/unison-cue" "$bin/unison-cue"
cp -R "$here/icons/hicolor" "$data/icons/"
# The menu entry runs the binary by full path, in case ~/.local/bin is not on PATH.
sed "s|^Exec=unison-cue|Exec=$bin/unison-cue|" "$here/unison-cue.desktop" > "$data/applications/unison-cue.desktop"
command -v update-desktop-database >/dev/null && update-desktop-database "$data/applications" || true
command -v gtk-update-icon-cache >/dev/null && gtk-update-icon-cache -q "$data/icons/hicolor" 2>/dev/null || true
echo "Installed. Start Unison Cue from the applications menu, or run: $bin/unison-cue"
