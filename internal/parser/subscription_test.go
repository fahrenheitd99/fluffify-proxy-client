package parser

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchSubscriptionSendsUserAgentExactly(t *testing.T) {
	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		w.Header().Set("profile-title", "demo")
		_, _ = w.Write([]byte("vless://11111111-1111-1111-1111-111111111111@a.example:443#node\n"))
	}))
	defer srv.Close()

	// Так UA приходит из буфера обмена Windows: с \r\n на конце.
	res, err := FetchSubscription(context.Background(), srv.URL, FetchOptions{
		UserAgent: "Happ/4.8.0/Android/17877369741321921609\r\n",
	})
	if err != nil {
		t.Fatalf("FetchSubscription: %v", err)
	}
	if want := "Happ/4.8.0/Android/17877369741321921609"; gotUA != want {
		t.Fatalf("User-Agent на сервере = %q, ждали %q", gotUA, want)
	}
	if len(res.Items) != 1 || res.Title != "demo" {
		t.Fatalf("подписка разобрана неверно: items=%d title=%q", len(res.Items), res.Title)
	}
}

func TestFetchSubscriptionStatusErrorIsInformative(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("Access denied: bad User-Agent\nsecond line"))
	}))
	defer srv.Close()

	_, err := FetchSubscription(context.Background(), srv.URL, FetchOptions{UserAgent: "Happ/1.0"})
	if err == nil {
		t.Fatal("ожидали ошибку на 403")
	}
	msg := err.Error()
	for _, want := range []string{"403", "Forbidden", "Happ/1.0", "Access denied"} {
		if !strings.Contains(msg, want) {
			t.Errorf("в ошибке нет %q: %s", want, msg)
		}
	}
	if strings.Contains(msg, "\n") {
		t.Errorf("тело ответа должно быть в одну строку: %q", msg)
	}
}
