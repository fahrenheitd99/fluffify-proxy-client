// Package netcheck — быстрая проверка туннеля для меню разработчика:
// видимый внешний IP через туннель и мимо (direct-интерфейс), плюс
// IP резолвера, который реально отвечает на системные DNS-запросы.
//
// Все замеры используют https://1.1.1.1/cdn-cgi/trace — соединение к
// IP-адресу не требует DNS-резолва, поэтому замеры не зависят от самих
// проверяемых механизмов.
package netcheck

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

// Result — результат одной проверки туннеля.
type Result struct {
	TunnelIP   string   `json:"tunnel_ip"`            // внешний IP через маршрут по умолчанию
	DirectIP   string   `json:"direct_ip"`            // внешний IP через физический интерфейс
	DirectIface string  `json:"direct_iface,omitempty"`
	ResolverIP string   `json:"resolver_ip"`          // IP, с которого отвечает DNS
	Notes      []string `json:"notes,omitempty"`      // предупреждения/ошибки замеров
}

// Check выполняет все замеры. Каждый замер самописывает ошибку в Notes —
// один провал не отменяет остальные.
func Check() Result {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()

	var r Result
	iface := defaultIface()
	r.DirectIface = iface

	r.TunnelIP = externalIP(ctx, nil)
	if r.TunnelIP == "" {
		r.Notes = append(r.Notes, "tunnel IP: request failed")
	}

	if iface == "" {
		r.Notes = append(r.Notes, "direct IP: no physical interface found (skip)")
	} else {
		srcIP := ifaceIPv4(iface)
		if srcIP == nil {
			r.Notes = append(r.Notes, fmt.Sprintf("direct IP: %s has no IPv4 address (skip)", iface))
		} else {
			bind := &net.Dialer{LocalAddr: &net.TCPAddr{IP: srcIP}, Timeout: 6 * time.Second}
			r.DirectIP = externalIP(ctx, bind)
			if r.DirectIP == "" {
				r.Notes = append(r.Notes, "direct IP: request failed")
			}
		}
	}

	if txt, err := net.LookupTXT("o-o.myaddr.l.google.com"); err == nil && len(txt) > 0 {
		r.ResolverIP = txt[0]
	} else {
		r.Notes = append(r.Notes, "resolver IP: TXT lookup failed")
	}

	return r
}

// externalIP — один запрос cdn-cgi/trace; dialer == nil — через маршрут
// по умолчанию (туннель, если он активен).
func externalIP(ctx context.Context, dialer *net.Dialer) string {
	tr := &http.Transport{TLSHandshakeTimeout: 5 * time.Second}
	if dialer != nil {
		tr.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialer.DialContext(ctx, network, addr)
		}
	}
	client := &http.Client{Transport: tr, Timeout: 8 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://1.1.1.1/cdn-cgi/trace", nil)
	if err != nil {
		return ""
	}
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	buf := make([]byte, 2048)
	n, _ := resp.Body.Read(buf)
	for _, line := range strings.Split(string(buf[:n]), "\n") {
		if strings.HasPrefix(line, "ip=") {
			return strings.TrimPrefix(line, "ip=")
		}
	}
	return ""
}
