//go:build !windows

package ping

import (
	"reflect"
	"testing"
)

func TestICMPArgsUnix(t *testing.T) {
	got := icmpArgs("1.2.3.4", "eth0")
	want := []string{"-n", "-c", "1", "-W", "2", "-I", "eth0", "1.2.3.4"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("icmpArgs = %v, ждали %v", got, want)
	}
	if got := icmpArgs("1.2.3.4", ""); len(got) != 6 {
		t.Fatalf("без интерфейса ждали 6 аргументов, получили %v", got)
	}
}
