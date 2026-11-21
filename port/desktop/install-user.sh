#!/usr/bin/env bash
# Current-user-only installation. No sudo. No modifications to other LaborControl profiles.
set -euo pipefail
SRC="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
for file in laborcontrol-desktop laborcontrol-controller laborcontrol.svg; do
  test -f "$SRC/$file" || { echo "Отсутствует $SRC/$file. Распакуйте архив полностью." >&2; exit 1; }
done
DST="$HOME/.local/opt/LaborControl-R11-Experimental"
APPLICATIONS="$HOME/.local/share/applications"
mkdir -p "$DST" "$APPLICATIONS"
install -m 0755 "$SRC/laborcontrol-desktop" "$DST/laborcontrol-desktop"
install -m 0755 "$SRC/laborcontrol-controller" "$DST/laborcontrol-controller"
install -m 0644 "$SRC/laborcontrol.svg" "$DST/laborcontrol.svg"
cat > "$APPLICATIONS/org.laborcontrol.R11Experimental.desktop" <<EOF
[Desktop Entry]
Version=1.0
Type=Application
Name=LaborControl R11 (экспериментальная)
GenericName=Графики и табели
Comment=Изолированная тестовая настольная версия LaborControl
Exec="$DST/laborcontrol-desktop"
Icon=$DST/laborcontrol.svg
Terminal=false
StartupNotify=true
Categories=Office;
EOF
chmod 0644 "$APPLICATIONS/org.laborcontrol.R11Experimental.desktop"
echo "LaborControl R11 (экспериментальная) добавлена в меню приложений."
echo "Данные: $HOME/.local/share/LaborControl-R11-Desktop-Experimental/LaborControl/"
echo "Рабочую базу Windows и старые данные Linux установщик не меняет."
