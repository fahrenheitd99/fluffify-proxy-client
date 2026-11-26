# fluffify-proxy-client
Fluffify is the crossplatform TUI proxy client based on xray-core

## Windows

Готовый архив: [`packaging/windows/fluffify-windows-amd64.zip`](packaging/windows/) —
внутри клиент `Fluffify.exe`, ядро Xray, `wintun.dll` и geo-файлы. Распакуйте
папку целиком и запустите `Fluffify.exe`. Инструкция для пользователя —
[`packaging/windows/README-RU.md`](packaging/windows/README-RU.md).

Сборка: `./packaging/windows/build.sh` (нужен `llvm-windres`; файлы ядра
ожидаются в `packaging/windows/`).