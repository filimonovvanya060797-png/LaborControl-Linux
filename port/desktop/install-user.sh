#!/usr/bin/env bash
# Current-user-only installation. No sudo. No modifications to other LaborControl profiles.
set -euo pipefail
SRC="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
for file in laborcontrol-desktop laborcontrol-controller laborcontrol.svg; do
  test -f "$SRC/$file" || { echo "Отсутствует $SRC/$file. Распакуйте архив полностью." >&2; exit 1; }
done
if (( EUID == 0 )); then
  echo "Установку LaborControl выполняйте от обычного пользователя, не root. Системные пакеты устанавливает администратор отдельно." >&2
  exit 1
fi
if ! command -v ldd >/dev/null 2>&1; then
  echo "Не найдена системная команда ldd. Обратитесь к администратору ALT Linux." >&2
  exit 1
fi
# Check trusted distribution before modifying the installed application.
if ! deps="$(LC_ALL=C ldd "$SRC/laborcontrol-desktop" 2>&1)" || grep -q "not found" <<<"$deps"; then
  echo "LaborControl не установлен: отсутствуют системные библиотеки или сборка несовместима с этой системой." >&2
  printf "%s\n" "$deps" >&2
  echo "Для ALT Linux проверьте GTK3 и WebKitGTK 4.1 (пакет libwebkit2gtk4.1 в совместимом репозитории). Обратитесь к администратору. Старая установка и данные не изменены." >&2
  exit 1
fi
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
