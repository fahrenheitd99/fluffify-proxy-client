# fluffify-proxy-client
Fluffify is the crossplatform TUI proxy client based on xray-core

## Windows

Готовый артефакт: [`packaging/windows/Fluffify.exe`](packaging/windows/) —
один самодостаточный файл (ядро Xray, драйвер TUN и лицензии встроены;
exe можно носить отдельно от папки). Инструкция для пользователя —
[`packaging/windows/README-RU.md`](packaging/windows/README-RU.md).

Сборка: `./packaging/windows/build.sh` (нужен `llvm-windres`; файлы ядра
ожидаются в `packaging/windows/`).