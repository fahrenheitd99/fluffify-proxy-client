package parser

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

type StreamConfig struct {
	Network  string // tcp, raw, ws, grpc, xhttp, httpupgrade, mkcp, hysteria
	Security string // none, tls, reality

	// TLS
	SNI              string
	Fingerprint      string
	ALPN             []string
	PinnedCertSha256 string // pcs / pinSHA256
	VerifyCertByName string // vcn
	ECHConfigList    string // ech

	// Reality
	PublicKey     string // pbk
	ShortID       string // sid
	SpiderX       string // spx
	MLDSA65Verify string // pqv

	// Транспорты
	Host        string
	Path        string
	Mode        string // xhttp mode / grpc multi
	ServiceName string // grpc
	Authority   string // grpc
	HeaderType  string // tcp/raw http, mkcp-legacy header
	KCPSeed     string // mkcp-legacy seed

	// Сырые JSON-блоки
	Extra     json.RawMessage
	FinalMask json.RawMessage
}

type Node struct {
	Protocol string // vless, vmess, trojan, shadowsocks, hysteria2, wireguard, socks, http
	Name     string
	Address  string
	Port     int
	RawLink  string

	// Общие поля аутентификации / протоколов
	UUID     string // vless, vmess
	Password string // trojan, shadowsocks, hysteria2, socks, http
	Username string // socks, http
	Method   string // shadowsocks cipher, vmess security, vless encryption
	Flow     string // vless

	// Hysteria2
	Obfs         string // salamander
	ObfsPassword string
	PortHop      string // mport или диапазон портов
	UpMbps       string
	DownMbps     string

	// WireGuard
	WGSecretKey    string
	WGPublicKey    string
	WGPreSharedKey string
	WGAddresses    []string
	WGMTU          int
	WGReserved     []int

	Stream StreamConfig
}

func ParseURI(rawLink string) (*Node, error) {
	rawLink = strings.TrimSpace(rawLink)
	if rawLink == "" {
		return nil, errors.New("пустая ссылка")
	}

	scheme, _, ok := strings.Cut(rawLink, "://")
	if !ok {
		return nil, errors.New("невалидная ссылка: отсутствует схема (например, vless://)")
	}

	var (
		node *Node
		err  error
	)

	switch strings.ToLower(scheme) {
	case "vless":
		node, err = parseVLESS(rawLink)
	case "vmess":
		node, err = parseVMess(rawLink)
	case "trojan":
		node, err = parseTrojan(rawLink)
	case "ss":
		node, err = parseShadowsocks(rawLink)
	case "hy2", "hysteria2":
		node, err = parseHysteria2(rawLink)
	case "wireguard", "wg":
		node, err = parseWireGuard(rawLink)
	case "socks", "socks5":
		node, err = parseSocks(rawLink)
	case "http", "https":
		node, err = parseHTTP(rawLink)
	default:
		return nil, fmt.Errorf("неподдерживаемый протокол: %s://", scheme)
	}

	if err != nil {
		return nil, err
	}
	node.RawLink = rawLink
	return node, nil
}

func parseStreamQuery(q url.Values, defaultSecurity string) (StreamConfig, error) {
	network := normalizeNetwork(q.Get("type"))
	security := strings.ToLower(strings.TrimSpace(q.Get("security")))
	if security == "" {
		security = defaultSecurity
	}

	st := StreamConfig{
		Network:          network,
		Security:         security,
		SNI:              firstNonEmpty(q.Get("sni"), q.Get("peer")),
		Fingerprint:      q.Get("fp"),
		ALPN:             splitCSV(q.Get("alpn")),
		PinnedCertSha256: normalizeCertPin(firstNonEmpty(q.Get("pcs"), q.Get("pinSHA256"))),
		VerifyCertByName: q.Get("vcn"),
		ECHConfigList:    q.Get("ech"),
		PublicKey:        q.Get("pbk"),
		ShortID:          q.Get("sid"),
		SpiderX:          q.Get("spx"),
		MLDSA65Verify:    q.Get("pqv"),
		Host:             q.Get("host"),
		Path:             q.Get("path"),
		Mode:             q.Get("mode"),
		ServiceName:      firstNonEmpty(q.Get("serviceName"), q.Get("service_name")),
		Authority:        q.Get("authority"),
		HeaderType:       q.Get("headerType"),
		KCPSeed:          q.Get("seed"),
	}

	if st.Network == "grpc" && st.ServiceName == "" && st.Path != "" {
		st.ServiceName = strings.TrimPrefix(st.Path, "/")
	}

	if extra := strings.TrimSpace(q.Get("extra")); extra != "" {
		if !json.Valid([]byte(extra)) {
			return st, errors.New("параметр extra содержит невалидный JSON")
		}
		st.Extra = json.RawMessage(extra)
	}

	if fm := strings.TrimSpace(q.Get("fm")); fm != "" {
		if !json.Valid([]byte(fm)) {
			return st, errors.New("параметр fm (finalmask) содержит невалидный JSON")
		}
		st.FinalMask = json.RawMessage(fm)
	}

	if st.Security == "reality" {
		if st.PublicKey == "" {
			return st, errors.New("для security=reality обязателен параметр pbk")
		}
		if st.Fingerprint == "" {
			st.Fingerprint = "chrome"
		}
	}

	return st, nil
}

func normalizeNetwork(netType string) string {
	switch strings.ToLower(strings.TrimSpace(netType)) {
	case "":
		return "tcp"
	case "splithttp":
		return "xhttp"
	case "websocket":
		return "ws"
	case "kcp":
		return "mkcp"
	default:
		return strings.ToLower(strings.TrimSpace(netType))
	}
}

func decodeBase64Flex(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\n", "")
	s = strings.TrimRight(s, "=")

	if b, err := base64.RawStdEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	return base64.RawURLEncoding.DecodeString(s)
}

func normalizeCertPin(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		lower := strings.ToLower(p)
		lower = strings.TrimPrefix(lower, "sha256:")
		lower = strings.TrimPrefix(lower, "sha-256:")
		lower = strings.ReplaceAll(lower, ":", "")
		lower = strings.ReplaceAll(lower, " ", "")
		if lower != "" {
			out = append(out, lower)
		}
	}
	return strings.Join(out, ",")
}

func rawUserPass(u *url.Userinfo) string {
	if u == nil {
		return ""
	}
	if p, ok := u.Password(); ok {
		return u.Username() + ":" + p
	}
	return u.Username()
}

func splitCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
