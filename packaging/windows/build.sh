#!/usr/bin/env bash
# Собирает Fluffify.exe — единственный артефакт для Windows. Ядро Xray,
# драйвер TUN (wintun.dll) и лицензии встроены в бинарник (go:embed):
# пользователь скачивает один файл и запускает его.
# Геобазы (geoip.dat/geosite.dat) в бинарник не встроены: клиент качает их сам,
# когда нужны правила роутинга по доменам/странам.
# Требуется llvm-windres (пакет llvm) и Go.
set -euo pipefail
cd "$(dirname "$0")/../.."

app="Fluffify.exe"

if ! command -v llvm-windres >/dev/null 2>&1; then
	echo "нужен llvm-windres (LLVM) — им собирается ресурс с иконкой" >&2
	exit 1
fi

# Ресурс PE: иконка + VERSIONINFO. Пересобирается из cmd/xray-tui/app.rc.
llvm-windres -I logo cmd/xray-tui/app.rc -O coff -o cmd/xray-tui/rsrc_windows_amd64.syso

GOOS=windows GOARCH=amd64 CGO_ENABLED=0 \
	go build -trimpath -ldflags "-s -w" -o "packaging/windows/$app" ./cmd/xray-tui

# Старые артефакты (zip, xray.exe, отдельные dll/лицензии) больше не выпускаем:
# всё это встроено в exe. Лицензии открываются из приложения
# (Настройки → «О приложении / лицензии») и лежат в internal/about/assets.
rm -f packaging/windows/xray-tui.exe packaging/windows/fluffify-windows-amd64.zip
rm -f packaging/windows/xray.exe packaging/windows/wintun.dll
rm -f packaging/windows/LICENSE-Xray packaging/windows/LICENSE-Wintun
rm -f packaging/windows/README-Xray.md packaging/windows/Xray-version.txt
rm -f packaging/windows/geoip.dat packaging/windows/geosite.dat

echo "готово: packaging/windows/$app — самодостаточный exe (ядро, драйвер TUN и лицензии внутри)"
ls -lh "packaging/windows/$app"
