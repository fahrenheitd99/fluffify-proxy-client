package daemon

import (
	"strings"
	"testing"
)

func TestMaskConfigSecrets(t *testing.T) {
	cfg := []byte(`{
	  "outbounds": [
	    {
	      "settings": {
	        "vnext": [
	          {"address": "vpn.example.com", "port": 443,
	           "users": [{"id": "aabbccddeeff00112233", "password": "supersecret123"}]}
	        ]
	      }
	    }
	  ],
	  "inbounds": [
	    {"settings": {"address": "172.19.0.1"}},
	    {"streamSettings": {"realitySettings": {"privateKey": "PrivKEYabcdef123456"}}},
	    {"settings": {"clients": [{"email": "me@x", "password1": "p4ssw0rd"}]}}
	  ],
	  "log": {"loglevel": "warning"}
	}`)

	out := maskConfigSecrets(cfg)
	s := string(out)

	if !strings.Contains(s, `"id": "aabb…masked"`) {
		t.Fatalf("id not masked:\n%s", s)
	}
	if !strings.Contains(s, `"password": "supe…masked"`) {
		t.Fatalf("password not masked:\n%s", s)
	}
	if !strings.Contains(s, `"privateKey": "Priv…masked"`) {
		t.Fatalf("privateKey not masked:\n%s", s)
	}
	if !strings.Contains(s, `"password1": "p4ss…masked"`) {
		t.Fatalf("password1 not masked:\n%s", s)
	}
	for _, secret := range []string{"aabbccddeeff00112233", "supersecret123", "PrivKEYabcdef123456", "p4ssw0rd"} {
		if strings.Contains(s, secret) {
			t.Fatalf("secret leaked: %q in:\n%s", secret, s)
		}
	}
	// нечувствительные поля остаются
	for _, keep := range []string{"vpn.example.com", "172.19.0.1", "loglevel"} {
		if !strings.Contains(s, keep) {
			t.Fatalf("non-secret %q lost:\n%s", keep, s)
		}
	}

	// битый JSON возвращается как есть
	if string(maskConfigSecrets([]byte("{broken"))) != "{broken" {
		t.Fatal("broken json must pass through unchanged")
	}
}

func TestPatchLogLevel(t *testing.T) {
	cfg := []byte(`{"log":{"loglevel":"warning"},"other":1}`)
	out := string(patchLogLevel(cfg, "info"))
	if !strings.Contains(out, `"loglevel":"info"`) {
		t.Fatalf("loglevel not patched:\n%s", out)
	}
	if strings.Contains(out, "warning") {
		t.Fatalf("old level kept:\n%s", out)
	}
	if !strings.Contains(out, `"other":1`) {
		t.Fatalf("other fields lost:\n%s", out)
	}

	// без блока log он создаётся
	out = string(patchLogLevel([]byte(`{"inbounds":[]}`), "info"))
	if !strings.Contains(out, `"log":{"loglevel":"info"}`) {
		t.Fatalf("log block not created:\n%s", out)
	}

	// битый JSON — как есть
	if s := string(patchLogLevel([]byte("{broken"), "info")); s != "{broken" {
		t.Fatal("broken json must pass through unchanged")
	}
}
