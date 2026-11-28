#!/usr/bin/env bash
set -euo pipefail
BUNDLE="$(cd -- "$1" && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf -- "$TMP"' EXIT
export HOME="$TMP/home"
DST="$HOME/.local/opt/LaborControl-R11-Experimental"
DATA="$HOME/.local/share/LaborControl-R11-Desktop-Experimental/LaborControl"
DESKTOP="$HOME/.local/share/applications/org.laborcontrol.R11Experimental.desktop"
mkdir -p "$DATA" "$DST" "$(dirname "$DESKTOP")" "$TMP/bin"
printf "test database\n" > "$DATA/labor_control.json"
printf "old binary\n" > "$DST/laborcontrol-desktop"
printf "old menu\n" > "$DESKTOP"
printf '#!/bin/sh\nprintf "libwebkit2gtk-4.1.so.0 => not found\\n"\n' > "$TMP/bin/ldd"
chmod +x "$TMP/bin/ldd"
if PATH="$TMP/bin:$PATH" bash "$BUNDLE/install-user.sh" >"$TMP/failure.log" 2>&1; then
  echo "Installer must reject missing WebKitGTK" >&2; exit 1
fi
grep -q "libwebkit2gtk-4.1.so.0 => not found" "$TMP/failure.log"
grep -qx "old binary" "$DST/laborcontrol-desktop"
grep -qx "old menu" "$DESKTOP"
grep -qx "test database" "$DATA/labor_control.json"
bash "$BUNDLE/install-user.sh"
cmp "$BUNDLE/laborcontrol-desktop" "$DST/laborcontrol-desktop"
cmp "$BUNDLE/laborcontrol-controller" "$DST/laborcontrol-controller"
grep -Fqx "Exec=\"$DST/laborcontrol-desktop\"" "$DESKTOP"
grep -qx "test database" "$DATA/labor_control.json"
"$DST/laborcontrol-desktop" --check
echo "Installer smoke PASS"
