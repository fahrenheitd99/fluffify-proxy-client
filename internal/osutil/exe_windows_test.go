//go:build windows

package osutil

import "testing"

func TestExeNameWindows(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"xray", "xray.exe"},
		{"xray.exe", "xray.exe"},
		{"XRAY.EXE", "XRAY.EXE"},
	} {
		if got := ExeName(tc.in); got != tc.want {
			t.Errorf("ExeName(%q) = %q, ждали %q", tc.in, got, tc.want)
		}
	}
}
