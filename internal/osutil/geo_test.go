package osutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSiblingAssetDir(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "xray")
	if got := SiblingAssetDir(bin); got != "" {
		t.Fatalf("без geo-файлов ждали пустую строку, получили %q", got)
	}

	if err := os.WriteFile(filepath.Join(dir, "geoip.dat"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := SiblingAssetDir(bin); got != dir {
		t.Fatalf("SiblingAssetDir = %q, ждали %q", got, dir)
	}

	if got := SiblingAssetDir("xray"); got != "" {
		t.Fatalf("для относительного пути ждали пустую строку, получили %q", got)
	}
}
