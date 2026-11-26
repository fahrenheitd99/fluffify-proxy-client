package osutil

import (
	"os"
	"path/filepath"
)

// SiblingAssetDir возвращает каталог бинарника, если рядом с ним лежат
// geoip.dat / geosite.dat. Это делает portable-установку «всё в одной папке»
// рабочей без переменных окружения.
func SiblingAssetDir(binPath string) string {
	if !filepath.IsAbs(binPath) {
		return ""
	}
	dir := filepath.Dir(binPath)
	for _, name := range []string{"geoip.dat", "geosite.dat"} {
		if info, err := os.Stat(filepath.Join(dir, name)); err == nil && !info.IsDir() {
			return dir
		}
	}
	return ""
}
