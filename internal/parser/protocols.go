package parser

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

func parsePort(portStr string) (int, error) {
	if portStr == "" {
		return 0, errors.New("отсутствует порт сервера")
	}
	p, err := strconv.Atoi(portStr)
	if err != nil || p < 1 || p > 65535 {
		return 0, fmt.Errorf("некорректный порт: %q", portStr)
	}
	return p, nil
}

// --- VLESS ---

func parseVLESS(rawLink string) (*Node, error) {
	u, err := url.Parse(rawLink)
	if err != nil {
		return nil, fmt.Errorf("невалидный VLESS URL: %w", err)
	}

	uuid := u.User.Username()
	if uuid == "" {
		return nil, errors.New("отсутствует UUID пользователя")
	}
	host := u.Hostname()
	if host == "" {
		return nil, errors.New("отсутствует адрес сервера")
	}
	port, err := parsePort(u.Port())
	if err != nil {
		return nil, err
	}

	q := u.Query()
	stream, err := parseStreamQuery(q, "none")
	if err != nil {
		return nil, err
	}

	enc := q.Get("encryption")
	if enc == "" {
		enc = "none"
	}

	name := strings.TrimSpace(u.Fragment)
	if name == "" {
		name = net.JoinHostPort(host, strconv.Itoa(port))
	}

	return &Node{
		Protocol: "vless",
		Name:     name,
		Address:  host,
		Port:     port,
		UUID:     uuid,
		Method:   enc,
		Flow:     q.Get("flow"),
		Stream:   stream,
	}, nil
}

// --- VMess (v2rayN Base64 JSON + URL формат) ---

type flexPort int

func (fp *flexPort) UnmarshalJSON(data []byte) error {
	var n int
	if err := json.Unmarshal(data, &n); err == nil {
		*fp = flexPort(n)
		return nil
	}
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		p, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil {
			return err
		}
		*fp = flexPort(p)
		return nil
	}
	return fmt.Errorf("некорректный порт в vmess JSON: %s", string(data))
}

type vmessJSON struct {
	PS   string   `json:"ps"`
	Add  string   `json:"add"`
	Port flexPort `json:"port"`
	ID   string   `json:"id"`
	Scy  string   `json:"scy"`
	Net  string   `json:"net"`
	Type string   `json:"type"`
	Host string   `json:"host"`
	Path string   `json:"path"`
	TLS  string   `json:"tls"`
	SNI  string   `json:"sni"`
	ALPN string   `json:"alpn"`
	FP   string   `json:"fp"`
}

func parseVMess(rawLink string) (*Node, error) {
	body := strings.TrimPrefix(rawLink, "vmess://")
	body = strings.TrimPrefix(body, "VMESS://")

	// Пробуем стандартный формат v2rayN: vmess://<base64-json>
	b64Part, frag, _ := strings.Cut(body, "#")
	if decoded, err := decodeBase64Flex(b64Part); err == nil && json.Valid(decoded) {
		var v vmessJSON
		if err := json.Unmarshal(decoded, &v); err != nil {
			return nil, fmt.Errorf("ошибка разбора vmess JSON: %w", err)
		}
		if v.Add == "" || v.ID == "" || v.Port < 1 || v.Port > 65535 {
			return nil, errors.New("в vmess JSON не хватает add, port или id")
		}

		sec := "none"
		if strings.EqualFold(v.TLS, "tls") {
			sec = "tls"
		}
		sni := v.SNI
		if sni == "" && sec == "tls" && v.Host != "" && !strings.Contains(v.Host, ",") {
			sni = v.Host
		}

		netType := normalizeNetwork(v.Net)
		stream := StreamConfig{
			Network:     netType,
			Security:    sec,
			SNI:         sni,
			Fingerprint: v.FP,
			ALPN:        splitCSV(v.ALPN),
			Host:        v.Host,
			Path:        v.Path,
			HeaderType:  v.Type,
		}
		if netType == "grpc" {
			stream.ServiceName = strings.TrimPrefix(v.Path, "/")
			stream.Mode = v.Type
		}
		if netType == "mkcp" {
			stream.KCPSeed = v.Path
		}

		method := v.Scy
		if method == "" {
			method = "auto"
		}

		name := strings.TrimSpace(v.PS)
		if name == "" && frag != "" {
			if unesc, err := url.PathUnescape(frag); err == nil {
				name = unesc
			}
		}
		if name == "" {
			name = net.JoinHostPort(v.Add, strconv.Itoa(int(v.Port)))
		}

		return &Node{
			Protocol: "vmess",
			Name:     name,
			Address:  v.Add,
			Port:     int(v.Port),
			UUID:     v.ID,
			Method:   method,
			Stream:   stream,
		}, nil
	}

	// Фолбэк на URL-формат: vmess://uuid@host:port?...#name
	u, err := url.Parse(rawLink)
	if err != nil {
		return nil, fmt.Errorf("невалидный VMess URL: %w", err)
	}
	uuid := u.User.Username()
	host := u.Hostname()
	port, err := parsePort(u.Port())
	if err != nil || uuid == "" || host == "" {
		return nil, errors.New("некорректная vmess:// ссылка")
	}

	q := u.Query()
	stream, err := parseStreamQuery(q, "none")
	if err != nil {
		return nil, err
	}

	method := firstNonEmpty(q.Get("encryption"), q.Get("scy"), "auto")
	name := strings.TrimSpace(u.Fragment)
	if name == "" {
		name = net.JoinHostPort(host, strconv.Itoa(port))
	}

	return &Node{
		Protocol: "vmess",
		Name:     name,
		Address:  host,
		Port:     port,
		UUID:     uuid,
		Method:   method,
		Stream:   stream,
	}, nil
}

// --- Trojan ---

func parseTrojan(rawLink string) (*Node, error) {
	u, err := url.Parse(rawLink)
	if err != nil {
		return nil, fmt.Errorf("невалидный Trojan URL: %w", err)
	}

	pass := rawUserPass(u.User)
	if pass == "" {
		return nil, errors.New("отсутствует пароль Trojan")
	}
	host := u.Hostname()
	if host == "" {
		return nil, errors.New("отсутствует адрес сервера")
	}
	port, err := parsePort(u.Port())
	if err != nil {
		return nil, err
	}

	// В Trojan TLS включён по умолчанию, если явно не указано иное
	stream, err := parseStreamQuery(u.Query(), "tls")
	if err != nil {
		return nil, err
	}

	name := strings.TrimSpace(u.Fragment)
	if name == "" {
		name = net.JoinHostPort(host, strconv.Itoa(port))
	}

	return &Node{
		Protocol: "trojan",
		Name:     name,
		Address:  host,
		Port:     port,
		Password: pass,
		Stream:   stream,
	}, nil
}

// --- Shadowsocks (SIP002 + Legacy + SS-2022) ---

func parseShadowsocks(rawLink string) (*Node, error) {
	_, rest, _ := strings.Cut(rawLink, "://")

	var name string
	if before, frag, ok := strings.Cut(rest, "#"); ok {
		rest = before
		if unesc, err := url.PathUnescape(frag); err == nil {
			name = strings.TrimSpace(unesc)
		} else {
			name = strings.TrimSpace(frag)
		}
	}

	var q url.Values
	if before, rawQ, ok := strings.Cut(rest, "?"); ok {
		rest = before
		q, _ = url.ParseQuery(rawQ)
	}
	rest = strings.TrimSuffix(rest, "/")

	var userInfo, hostPort string
	if idx := strings.LastIndex(rest, "@"); idx != -1 {
		// SIP002: ss://base64(method:pass)@host:port или ss://method:pass@host:port
		userInfo = rest[:idx]
		hostPort = rest[idx+1:]

		if unesc, err := url.PathUnescape(userInfo); err == nil {
			userInfo = unesc
		}
		if !strings.Contains(userInfo, ":") {
			decoded, err := decodeBase64Flex(userInfo)
			if err != nil {
				return nil, fmt.Errorf("не удалось декодировать userinfo Shadowsocks: %w", err)
			}
			userInfo = string(decoded)
		}
	} else {
		// Legacy: ss://base64(method:pass@host:port)
		decoded, err := decodeBase64Flex(rest)
		if err != nil {
			return nil, fmt.Errorf("не удалось декодировать Shadowsocks ссылку: %w", err)
		}
		s := string(decoded)
		idx := strings.LastIndex(s, "@")
		if idx == -1 {
			return nil, errors.New("некорректный формат Shadowsocks ссылки")
		}
		userInfo = s[:idx]
		hostPort = s[idx+1:]
	}

	// Режем по первому ':', чтобы не сломать SS-2022 ключи вида serverKey:userKey
	method, password, ok := strings.Cut(userInfo, ":")
	if !ok || method == "" || password == "" {
		return nil, errors.New("в Shadowsocks отсутствует метод шифрования или пароль")
	}

	host, portStr, err := net.SplitHostPort(hostPort)
	if err != nil {
		return nil, fmt.Errorf("некорректный адрес Shadowsocks (%q): %w", hostPort, err)
	}
	port, err := parsePort(portStr)
	if err != nil {
		return nil, err
	}

	var stream StreamConfig
	if len(q) > 0 && (q.Get("type") != "" || q.Get("security") != "") {
		stream, err = parseStreamQuery(q, "none")
		if err != nil {
			return nil, err
		}
	} else {
		stream = StreamConfig{Network: "tcp", Security: "none"}
	}

	if name == "" {
		name = net.JoinHostPort(host, strconv.Itoa(port))
	}

	return &Node{
		Protocol: "shadowsocks",
		Name:     name,
		Address:  host,
		Port:     port,
		Method:   method,
		Password: password,
		Stream:   stream,
	}, nil
}

// --- Hysteria2 (hy2:// и hysteria2://) ---

func parseHysteria2(rawLink string) (*Node, error) {
	_, rest, _ := strings.Cut(rawLink, "://")

	var name string
	if before, frag, ok := strings.Cut(rest, "#"); ok {
		rest = before
		if unesc, err := url.PathUnescape(frag); err == nil {
			name = strings.TrimSpace(unesc)
		} else {
			name = strings.TrimSpace(frag)
		}
	}

	var q url.Values
	if before, rawQ, ok := strings.Cut(rest, "?"); ok {
		rest = before
		var err error
		q, err = url.ParseQuery(rawQ)
		if err != nil {
			return nil, fmt.Errorf("невалидные параметры Hysteria2: %w", err)
		}
	} else {
		q = url.Values{}
	}

	// Отрезаем путь (обычно пустой или '/')
	authority, _, _ := strings.Cut(rest, "/")

	var auth, hostPort string
	if idx := strings.LastIndex(authority, "@"); idx != -1 {
		rawAuth := authority[:idx]
		hostPort = authority[idx+1:]
		if unesc, err := url.PathUnescape(rawAuth); err == nil {
			auth = unesc
		} else {
			auth = rawAuth
		}
	} else {
		hostPort = authority
	}

	// Поддерживаем порт-хоппинг прямо в адресе: host:443,20000-30000 или host:20000-30000
	host, portSpec, err := splitHostPortRaw(hostPort)
	if err != nil {
		return nil, fmt.Errorf("некорректный адрес Hysteria2: %w", err)
	}

	portHop := strings.TrimSpace(q.Get("mport"))
	primaryPortStr := portSpec
	if primaryPortStr == "" {
		primaryPortStr = "443"
	} else if strings.ContainsAny(primaryPortStr, "-,") {
		if portHop == "" {
			portHop = primaryPortStr
		}
		firstChunk := strings.Split(primaryPortStr, ",")[0]
		primaryPortStr = strings.Split(firstChunk, "-")[0]
	}

	port, err := parsePort(primaryPortStr)
	if err != nil {
		return nil, err
	}

	alpn := splitCSV(q.Get("alpn"))
	if len(alpn) == 0 {
		alpn = []string{"h3"}
	}

	stream := StreamConfig{
		Network:          "hysteria",
		Security:         "tls",
		SNI:              firstNonEmpty(q.Get("sni"), q.Get("peer")),
		ALPN:             alpn,
		PinnedCertSha256: normalizeCertPin(firstNonEmpty(q.Get("pinSHA256"), q.Get("pcs"))),
		VerifyCertByName: q.Get("vcn"),
		ECHConfigList:    q.Get("ech"),
	}

	if name == "" {
		name = net.JoinHostPort(host, strconv.Itoa(port))
	}

	return &Node{
		Protocol:     "hysteria2",
		Name:         name,
		Address:      host,
		Port:         port,
		Password:     auth,
		Obfs:         strings.ToLower(strings.TrimSpace(q.Get("obfs"))),
		ObfsPassword: q.Get("obfs-password"),
		PortHop:      portHop,
		UpMbps:       firstNonEmpty(q.Get("upmbps"), q.Get("up")),
		DownMbps:     firstNonEmpty(q.Get("downmbps"), q.Get("down")),
		Stream:       stream,
	}, nil
}

func splitHostPortRaw(hp string) (string, string, error) {
	if hp == "" {
		return "", "", errors.New("пустой адрес")
	}
	if strings.HasPrefix(hp, "[") {
		end := strings.Index(hp, "]")
		if end == -1 {
			return "", "", errors.New("незакрытая скобка IPv6")
		}
		host := hp[1:end]
		rest := hp[end+1:]
		if rest == "" {
			return host, "", nil
		}
		if !strings.HasPrefix(rest, ":") {
			return "", "", errors.New("ожидалось двоеточие после IPv6")
		}
		return host, rest[1:], nil
	}
	if idx := strings.LastIndex(hp, ":"); idx != -1 {
		return hp[:idx], hp[idx+1:], nil
	}
	return hp, "", nil
}

// --- WireGuard (wireguard:// и wg://) ---

func parseWireGuard(rawLink string) (*Node, error) {
	u, err := url.Parse(rawLink)
	if err != nil {
		return nil, fmt.Errorf("невалидный WireGuard URL: %w", err)
	}

	secretKey := u.User.Username()
	host := u.Hostname()
	port, err := parsePort(u.Port())
	if err != nil {
		return nil, err
	}

	q := u.Query()
	pubKey := firstNonEmpty(q.Get("publickey"), q.Get("pbk"))
	if secretKey == "" || pubKey == "" {
		return nil, errors.New("для WireGuard обязательны приватный ключ и publickey")
	}

	addrs := splitCSV(firstNonEmpty(q.Get("address"), q.Get("ip")))
	if len(addrs) == 0 {
		addrs = []string{"10.0.0.1/32"}
	}

	mtu := 1420
	if mtuStr := q.Get("mtu"); mtuStr != "" {
		if m, err := strconv.Atoi(mtuStr); err == nil && m >= 576 && m <= 1500 {
			mtu = m
		}
	}

	var reserved []int
	if resStr := q.Get("reserved"); resStr != "" {
		for _, part := range splitCSV(resStr) {
			v, err := strconv.Atoi(part)
			if err != nil || v < 0 || v > 255 {
				return nil, fmt.Errorf("некорректный reserved в WireGuard: %q", resStr)
			}
			reserved = append(reserved, v)
		}
	}

	name := strings.TrimSpace(u.Fragment)
	if name == "" {
		name = net.JoinHostPort(host, strconv.Itoa(port))
	}

	return &Node{
		Protocol:       "wireguard",
		Name:           name,
		Address:        host,
		Port:           port,
		WGSecretKey:    secretKey,
		WGPublicKey:    pubKey,
		WGPreSharedKey: firstNonEmpty(q.Get("presharedkey"), q.Get("psk")),
		WGAddresses:    addrs,
		WGMTU:          mtu,
		WGReserved:     reserved,
	}, nil
}

// --- SOCKS5 ---

func parseSocks(rawLink string) (*Node, error) {
	u, err := url.Parse(rawLink)
	if err != nil {
		return nil, fmt.Errorf("невалидный SOCKS URL: %w", err)
	}

	host := u.Hostname()
	portStr := u.Port()
	var user, pass string

	// Поддержка v2rayN/Shadowrocket base64-формата: socks://base64(user:pass@host:port)
	if portStr == "" && u.User == nil && host != "" {
		if decoded, decErr := decodeBase64Flex(host); decErr == nil {
			if u2, parseErr := url.Parse("socks://" + string(decoded)); parseErr == nil && u2.Port() != "" {
				u = u2
				host = u.Hostname()
				portStr = u.Port()
			}
		}
	}

	port, err := parsePort(portStr)
	if err != nil {
		return nil, err
	}

	if u.User != nil {
		if p, ok := u.User.Password(); ok {
			user = u.User.Username()
			pass = p
		} else if raw := u.User.Username(); raw != "" {
			if decoded, decErr := decodeBase64Flex(raw); decErr == nil && strings.Contains(string(decoded), ":") {
				user, pass, _ = strings.Cut(string(decoded), ":")
			} else {
				user = raw
			}
		}
	}

	name := strings.TrimSpace(u.Fragment)
	if name == "" {
		name = net.JoinHostPort(host, strconv.Itoa(port))
	}

	return &Node{
		Protocol: "socks",
		Name:     name,
		Address:  host,
		Port:     port,
		Username: user,
		Password: pass,
	}, nil
}

// --- HTTP / HTTPS ---

func parseHTTP(rawLink string) (*Node, error) {
	u, err := url.Parse(rawLink)
	if err != nil {
		return nil, fmt.Errorf("невалидный HTTP URL: %w", err)
	}

	host := u.Hostname()
	if host == "" {
		return nil, errors.New("отсутствует адрес HTTP-прокси")
	}

	isTLS := strings.EqualFold(u.Scheme, "https")
	portStr := u.Port()
	if portStr == "" {
		if isTLS {
			portStr = "443"
		} else {
			portStr = "80"
		}
	}
	port, err := parsePort(portStr)
	if err != nil {
		return nil, err
	}

	var user, pass string
	if u.User != nil {
		user = u.User.Username()
		pass, _ = u.User.Password()
	}

	sec := "none"
	if isTLS {
		sec = "tls"
	}

	name := strings.TrimSpace(u.Fragment)
	if name == "" {
		name = net.JoinHostPort(host, strconv.Itoa(port))
	}

	return &Node{
		Protocol: "http",
		Name:     name,
		Address:  host,
		Port:     port,
		Username: user,
		Password: pass,
		Stream: StreamConfig{
			Network:  "tcp",
			Security: sec,
			SNI:      u.Query().Get("sni"),
		},
	}, nil
}
