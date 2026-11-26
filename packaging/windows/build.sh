#!/usr/bin/env bash
# Собирает Fluffify.exe со значком и версией и упаковывает готовый к раздаче
# архив. Ядро Xray и драйвер TUN (wintun.dll) встроены в бинарник
# (github.com/xtls/xray-core + go:embed в internal/osutil/assets), поэтому
# xray.exe и wintun.dll в архиве больше не нужны: exe сам извлекает драйвер.
# Геобазы (geoip.dat/geosite.dat) в архив не попадают: клиент качает их сам,
# когда нужны правила роутинга по доменам/странам.
# Требуется llvm-windres (пакет llvm), Go и python3.
set -euo pipefail
cd "$(dirname "$0")/../.."

app="Fluffify.exe"

if ! command -v llvm-windres >/dev/null 2>&1; then
	echo "нужен llvm-windres (LLVM) — им собирается ресурс с иконкой" >&2
	exit 1
fi
if ! command -v python3 >/dev/null 2>&1; then
	echo "нужен python3 — им собирается zip-архив" >&2
	exit 1
fi

# Лицензии и документацию берём из packaging/windows (распакованы из
# Xray-windows-64.zip) и складываем в архив в папку docs/.
# Встроенная копия драйвера живёт в internal/osutil/assets/wintun.dll —
# при обновлении Xray-windows-64.zip обнови и её (cp поверх, потом коммит).
core=(LICENSE-Xray LICENSE-Wintun README-Xray.md Xray-version.txt)
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

# Верхний уровень архива — только exe, весь текст идёт в docs/.
python3 - <<'PY'
import pathlib, zipfile

base = pathlib.Path("packaging/windows")
target = base / "fluffify-windows-amd64.zip"
top = ["Fluffify.exe"]
docs = ["README-RU.md", "LICENSE-Xray", "LICENSE-Wintun", "README-Xray.md", "Xray-version.txt"]

for name in top + docs:
    if not (base / name).is_file():
        raise SystemExit(f"нет packaging/windows/{name}")

with zipfile.ZipFile(target, "w", zipfile.ZIP_DEFLATED) as z:
    for name in top:
        z.write(base / name, name)
    for name in docs:
        z.write(base / name, "docs/" + name)
print("архив:", target)
PY

echo "готово: packaging/windows/$app и fluffify-windows-amd64.zip"
ls -lh packaging/windows/fluffify-windows-amd64.zip