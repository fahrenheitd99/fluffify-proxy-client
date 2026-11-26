#!/usr/bin/env bash
# Собирает Fluffify.exe со значком и версией и упаковывает готовый к раздаче
# архив вместе с ядром Xray.
# Геобазы (geoip.dat/geosite.dat) в архив не попадают: клиент качает их сам,
# когда нужны правила роутинга по доменам/странам.
# Требуется llvm-windres (пакет llvm) и Go.
set -euo pipefail
cd "$(dirname "$0")/../.."

app="Fluffify.exe"

if ! command -v llvm-windres >/dev/null 2>&1; then
	echo "нужен llvm-windres (LLVM) — им собирается ресурс с иконкой" >&2
	exit 1
fi

# Ядро и его лицензии кладём в packaging/windows из Xray-windows-64.zip.
core=(xray.exe wintun.dll LICENSE-Xray LICENSE-Wintun README-Xray.md Xray-version.txt)
missing=0
for f in "${core[@]}"; do
	if [ ! -f "packaging/windows/$f" ]; then
		echo "нет packaging/windows/$f — распакуй Xray-windows-64.zip в packaging/windows" >&2
		missing=1
	fi
done
if [ "$missing" -ne 0 ]; then
	exit 1
fi

# Ресурс PE: иконка + VERSIONINFO. Пересобирается из cmd/xray-tui/app.rc.
llvm-windres -I logo cmd/xray-tui/app.rc -O coff -o cmd/xray-tui/rsrc_windows_amd64.syso

GOOS=windows GOARCH=amd64 CGO_ENABLED=0 \
	go build -trimpath -ldflags "-s -w" -o "packaging/windows/$app" ./cmd/xray-tui

# Старое имя бинарника больше не выпускаем, геобазы не распространяем.
rm -f packaging/windows/xray-tui.exe
rm -f packaging/windows/geoip.dat packaging/windows/geosite.dat

files=("$app" README-RU.md "${core[@]}")

if command -v zip >/dev/null 2>&1; then
	rm -f packaging/windows/fluffify-windows-amd64.zip
	(cd packaging/windows && zip -q fluffify-windows-amd64.zip "${files[@]}")
elif command -v python3 >/dev/null 2>&1; then
	python3 - "${files[@]}" <<'PY'
import pathlib, sys, zipfile

base = pathlib.Path("packaging/windows")
target = base / "fluffify-windows-amd64.zip"
with zipfile.ZipFile(target, "w", zipfile.ZIP_DEFLATED) as z:
    for name in sys.argv[1:]:
        z.write(base / name, name)
PY
else
	echo "готово: packaging/windows/$app (zip-утилита не найдена)" >&2
	exit 0
fi

echo "готово: packaging/windows/$app и fluffify-windows-amd64.zip"
ls -lh packaging/windows/fluffify-windows-amd64.zip