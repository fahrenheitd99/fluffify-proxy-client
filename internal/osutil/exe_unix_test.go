//go:build !windows

package osutil

import "testing"

func TestExeNameUnix(t *testing.T) {
	if got := ExeName("xray"); got != "xray" {
		t.Fatalf("ExeName(xray) = %q, ждали %q", got, "xray")
	}
}
