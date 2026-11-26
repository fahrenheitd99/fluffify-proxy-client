#!/usr/bin/env bash
# Перегенерирует растровые иконки из SVG.
# Требуется rsvg-convert (librsvg) и python3 с Pillow.
set -euo pipefail
cd "$(dirname "$0")/../.."

sed 's/currentColor/#ffffff/g' logo/fluffify-symbolic.svg > /tmp/fluffify-symbolic-white.svg
rsvg-convert -w 256 -h 256 -o logo/fluffify-symbolic.png /tmp/fluffify-symbolic-white.svg
rsvg-convert -w 256 -h 256 -o /tmp/fluffify-256.png logo/fluffify.svg

python3 - <<'PY'
from PIL import Image

SIZES = [(16, 16), (20, 20), (24, 24), (32, 32), (40, 40),
         (48, 48), (64, 64), (128, 128), (256, 256)]

# Полноцветная иконка приложения — для ярлыков и ресурса .exe.
Image.open('/tmp/fluffify-256.png').convert('RGBA').save('logo/fluffify.ico', sizes=SIZES)

# Символические иконки трея под светлую и тёмную тему Windows.
mask = Image.open('logo/fluffify-symbolic.png').convert('RGBA')
for name, rgb in (('white', (255, 255, 255)), ('black', (31, 31, 31))):
    img = Image.new('RGBA', mask.size, rgb + (0,))
    img.putalpha(mask.getchannel('A'))
    img.save(f'logo/fluffify-symbolic-{name}.ico', sizes=SIZES)

print('обновлены logo/fluffify.ico, logo/fluffify-symbolic.png и символические .ico')
PY

# После смены .ico пересобери ресурс:
#   llvm-windres -I logo cmd/xray-tui/app.rc -O coff -o cmd/xray-tui/rsrc_windows_amd64.syso