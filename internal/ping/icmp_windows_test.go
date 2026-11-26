//go:build windows

package ping

import (
	"reflect"
	"testing"
)

func TestICMPArgsWindows(t *testing.T) {
	got := icmpArgs("1.2.3.4", "Ethernet")
	want := []string{"-n", "1", "-w", "2000", "1.2.3.4"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("icmpArgs = %v, ждали %v", got, want)
	}
}
