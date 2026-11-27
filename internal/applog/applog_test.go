package applog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitWriteAndFail(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	Init("test")
	Event("hello", "k", "v")
	Fail("boom", os.ErrNotExist, "n", 1)

	data, err := os.ReadFile(File())
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, want := range []string{"component=test", "hello", "boom", "file does not exist"} {
		if !strings.Contains(s, want) {
			t.Fatalf("log missing %q in:\n%s", want, s)
		}
	}
}

func TestRotation(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	Init("test")

	pad := strings.Repeat("x", 1024)
	for i := 0; i < 2500; i++ { // ~2.7 MiB суммарно — больше maxFileBytes
		Event("fill", "pad", pad)
	}

	st1, err := os.Stat(filepath.Join(Dir(), "app.log.1"))
	if err != nil {
		t.Fatalf("rotation did not create app.log.1: %v", err)
	}
	if st1.Size() > maxFileBytes {
		t.Fatalf("rotated file too big: %d", st1.Size())
	}
	st2, err := os.Stat(File())
	if err != nil {
		t.Fatal(err)
	}
	if st2.Size() > maxFileBytes {
		t.Fatalf("active file too big: %d", st2.Size())
	}
}
