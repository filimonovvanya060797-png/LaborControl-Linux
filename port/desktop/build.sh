#!/usr/bin/env bash
# Build Linux Go Core and GTK/WebKit window into one isolated folder.
set -euo pipefail
HERE="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd -- "$HERE/../.." && pwd)"
OUT="${1:-$ROOT/dist/laborcontrol-r11-desktop}"
mkdir -p "$OUT"
command -v go >/dev/null
command -v gcc >/dev/null
if ! pkg-config --exists gtk+-3.0 webkit2gtk-4.1; then
  echo "Нужны development-пакеты GTK3 и WebKit2GTK 4.1 (pkg-config)." >&2
  exit 1
fi
(
  cd "$ROOT/port/controller"
  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o "$OUT/laborcontrol-controller" .
)
read -r -a CFLAGS <<<"$(pkg-config --cflags gtk+-3.0 webkit2gtk-4.1)"
read -r -a LIBS <<<"$(pkg-config --libs gtk+-3.0 webkit2gtk-4.1)"
gcc -std=c11 -O2 -Wall -Wextra -o "$OUT/laborcontrol-desktop"   "${CFLAGS[@]}" "$HERE/main.c" "${LIBS[@]}"
install -m 0644 "$HERE/laborcontrol.svg" "$OUT/laborcontrol.svg"
install -m 0755 "$HERE/install-user.sh" "$OUT/install-user.sh"
install -m 0644 "$HERE/README_RU.md" "$OUT/README_RU.md"
"$OUT/laborcontrol-desktop" --check
printf 'Готово: %s\n' "$OUT"
