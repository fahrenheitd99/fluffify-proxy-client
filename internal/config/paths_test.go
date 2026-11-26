package config

import (
	"path/filepath"
	"testing"
)

func TestStorePathMatchesConfigDir(t *testing.T) {
	if got, want := StorePath(), filepath.Join(ConfigDir(), "store.json"); got != want {
		t.Fatalf("StorePath() = %q, ждали %q", got, want)
	}
	if ConfigDir() == "" || GeoBaseDir() == "" {
		t.Fatal("каталоги конфигурации и geo не должны быть пустыми")
	}
}

func TestGeoProviderDirDefault(t *testing.T) {
	if got, want := GeoProviderDir(""), filepath.Join(GeoBaseDir(), DefaultGeoProviderID); got != want {
		t.Fatalf("GeoProviderDir(\"\") = %q, ждали %q", got, want)
	}
}
