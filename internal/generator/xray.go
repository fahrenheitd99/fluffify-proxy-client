package generator

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"

	"xray-tui/internal/config"
	"xray-tui/internal/geo"
	"xray-tui/internal/parser"
)

type Options struct {
	Listen         string
	SocksPort      int
	HTTPPort       int
	MixedInbound   bool
	Sniffing       bool
	ProxyAuth      bool
	ProxyUser      string
	ProxyPass      string
	LogLevel       string
	TUN            bool
	TUNName        string
	TUNMTU         int
	TUNAutoDNS     bool
	TUNIPv6        bool
	RoutingEnabled bool
	RoutingProfile *config.RoutingProfile
}

type BuildResult struct {
	Config    []byte
	SocksAddr string
	HTTPAddr  string
	Mixed     bool
	ProxyAuth bool
	AssetDir  string
}

func DefaultOptions() Options {
	return Options{
		Listen:    "127.0.0.1",
		SocksPort: 10808,
		HTTPPort:  10809,
		Sniffing:  true,
		LogLevel:  "warning",
		TUNName:   "xray0",
		TUNMTU:    1500,
	}
}

func OptionsFromStore(store *config.Store) Options {
	return Options{
		Listen:         store.Settings.Listen,
		SocksPort:      store.Settings.SocksPort,
		HTTPPort:       store.Settings.GetHTTPPort(),
		MixedInbound:   store.Settings.MixedInbound,
		Sniffing:       store.Settings.SniffingEnabled(),
		ProxyAuth:      store.Settings.ProxyAuth,
		ProxyUser:      store.Settings.ProxyUser,
		ProxyPass:      store.Settings.ProxyPass,
		LogLevel:       store.Settings.LogLevel,
		TUN:            store.Settings.Mode == config.ModeTUN,
		TUNName:        store.Settings.TUNInterface,
		TUNMTU:         store.Settings.TUNMTU,
		TUNAutoDNS:     store.Settings.TUNAutoDNS,
		TUNIPv6:        store.Settings.TUNIPv6Value(),
		RoutingEnabled: store.Routing.Enabled,
		RoutingProfile: store.ActiveRoutingProfile(),
	}
}

// routingNeedsGeoDocs сообщает, ссылается ли профиль на категории geosite/geoip,
// для которых нужны файлы геобаз. Категория «private» встроена в Xray и файлов
// не требует, поэтому с ней сборка конфига не блокируется.
func routingNeedsGeoDocs(rp *config.RoutingProfile) bool {
	for _, rule := range rp.Rules {
		for _, list := range [][]string{rule.Domains, rule.IPs} {
			for _, value := range list {
				prefix, category, ok := strings.Cut(value, ":")
				if !ok {
					continue
				}
				switch strings.ToLower(prefix) {
				case "geosite", "geoip":
					if !strings.EqualFold(category, "private") {
						return true
					}
				}
			}
		}
	}
	return false
}

func BuildForProfile(store *config.Store, prof *config.Profile) (*BuildResult, error) {
	opts := OptionsFromStore(store)

	var assetDir string
	if store.Routing.Enabled && opts.RoutingProfile != nil {
		dir := config.GeoProviderDir(opts.RoutingProfile.GeoProviderID)
		if geo.Check(dir).Ready() {
			assetDir = dir
		} else if routingNeedsGeoDocs(opts.RoutingProfile) {
			return nil, errors.New("геобазы не скачаны: открой профиль роутинга и скачай их (пункт «Файлы геобаз»)")
		}
	}

	var (
		cfg       []byte
		socksAddr string
		httpAddr  string
		err       error
	)

	if prof.Type == config.ProfileTypeRaw {
		cfg, socksAddr, httpAddr, err = PrepareRawJSON(prof.RawJSON, opts)
	} else {
		var node *parser.Node
		node, err = parser.ParseURI(prof.URI)
		if err != nil {
			return nil, fmt.Errorf("ошибка парсинга %q: %w", prof.Name, err)
		}
		cfg, err = BuildFromNode(node, opts)
		if err == nil {
			socksAddr = net.JoinHostPort(opts.Listen, strconv.Itoa(opts.SocksPort))
			if !opts.MixedInbound && opts.HTTPPort > 0 {
				httpAddr = net.JoinHostPort(opts.Listen, strconv.Itoa(opts.HTTPPort))
			}
		}
	}
	if err != nil {
		return nil, err
	}

	return &BuildResult{
		Config:    cfg,
		SocksAddr: socksAddr,
		HTTPAddr:  httpAddr,
		Mixed:     opts.MixedInbound,
		ProxyAuth: opts.authActive(),
		AssetDir:  assetDir,
	}, nil
}

func (o Options) authActive() bool {
	return o.ProxyAuth && strings.TrimSpace(o.ProxyUser) != ""
}

func BuildFromNode(node *parser.Node, opts Options) ([]byte, error) {
	if opts.Listen == "" {
		opts.Listen = "127.0.0.1"
	}
	if opts.SocksPort == 0 {
		opts.SocksPort = 10808
	}
	if opts.LogLevel == "" {
		opts.LogLevel = "warning"
	}

	outbound, err := BuildOutbound(node, "proxy")
	if err != nil {
		return nil, err
	}

	inbounds := buildLocalProxyInbounds(opts)
	if opts.TUN {
		inbounds = append(inbounds, buildTUNInbound(opts))
	}

	outbounds := []any{
		outbound,
		map[string]any{"tag": "direct", "protocol": "freedom"},
		map[string]any{"tag": "block", "protocol": "blackhole"},
	}

	cfg := map[string]any{
		"log":       map[string]any{"loglevel": opts.LogLevel},
		"inbounds":  inbounds,
		"outbounds": outbounds,
	}

	var rules []any
	if opts.TUN && opts.TUNAutoDNS {
		ensureTUNDNS(cfg, "tun-in")
		if r, ok := cfg["routing"].(map[string]any); ok {
			rules, _ = r["rules"].([]any)
		}
	}

	if opts.RoutingEnabled && opts.RoutingProfile != nil {
		userRules := buildRoutingRules(opts.RoutingProfile.Rules, "proxy", "direct", "block")
		rules = append(rules, userRules...)
	}

	if len(rules) > 0 {
		strategy := "IPIfNonMatch"
		if opts.RoutingProfile != nil && opts.RoutingProfile.DomainStrategy != "" {
			strategy = opts.RoutingProfile.DomainStrategy
		}
		cfg["routing"] = map[string]any{
			"domainStrategy": strategy,
			"rules":          rules,
		}
	}

	return json.MarshalIndent(cfg, "", "  ")
}

func PrepareRawJSON(raw []byte, opts Options) ([]byte, string, string, error) {
	if opts.Listen == "" {
		opts.Listen = "127.0.0.1"
	}
	if opts.SocksPort == 0 {
		opts.SocksPort = 10808
	}

	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, "", "", fmt.Errorf("невалидный Raw JSON: %w", err)
	}

	if opts.LogLevel != "" {
		logObj, _ := doc["log"].(map[string]any)
		if logObj == nil {
			logObj = map[string]any{}
		}
		logObj["loglevel"] = opts.LogLevel
		doc["log"] = logObj
	}

	inbounds, _ := doc["inbounds"].([]any)
	var (
		hasPrimary bool
		hasHTTP    bool
		hasTUN     bool
		tunTag     = "tun-in"
		socksAddr  = net.JoinHostPort(opts.Listen, strconv.Itoa(opts.SocksPort))
		httpAddr   string
	)

	filtered := make([]any, 0, len(inbounds)+2)
	for _, item := range inbounds {
		inb, ok := item.(map[string]any)
		if !ok {
			filtered = append(filtered, item)
			continue
		}
		proto, _ := inb["protocol"].(string)
		switch proto {
		case "socks", "mixed":
			if !hasPrimary {
				hasPrimary = true
				if opts.MixedInbound {
					inb["protocol"] = "mixed"
				} else {
					inb["protocol"] = "socks"
				}
				inb["listen"] = opts.Listen
				inb["port"] = opts.SocksPort
				inb["settings"] = buildSocksSettings(opts)
				inb["sniffing"] = buildSniffing(opts.Sniffing)
				filtered = append(filtered, inb)
			}
		case "http":
			if opts.MixedInbound {
				// В режиме Mixed отдельный HTTP-порт не нужен
				continue
			}
			if !hasHTTP && opts.HTTPPort > 0 {
				hasHTTP = true
				inb["listen"] = opts.Listen
				inb["port"] = opts.HTTPPort
				inb["settings"] = buildHTTPSettings(opts)
				inb["sniffing"] = buildSniffing(opts.Sniffing)
				httpAddr = net.JoinHostPort(opts.Listen, strconv.Itoa(opts.HTTPPort))
				filtered = append(filtered, inb)
			}
		case "tun":
			if opts.TUN {
				hasTUN = true
				if t, _ := inb["tag"].(string); t != "" {
					tunTag = t
				}
				inb["settings"] = buildTUNSettings(opts)
				inb["sniffing"] = buildSniffing(opts.Sniffing)
				filtered = append(filtered, inb)
			}
		default:
			filtered = append(filtered, inb)
		}
	}

	if !hasPrimary {
		proto := "socks"
		if opts.MixedInbound {
			proto = "mixed"
		}
		filtered = append(filtered, map[string]any{
			"tag":      "socks-in",
			"listen":   opts.Listen,
			"port":     opts.SocksPort,
			"protocol": proto,
			"settings": buildSocksSettings(opts),
			"sniffing": buildSniffing(opts.Sniffing),
		})
	}

	if !opts.MixedInbound && opts.HTTPPort > 0 && !hasHTTP {
		httpAddr = net.JoinHostPort(opts.Listen, strconv.Itoa(opts.HTTPPort))
		filtered = append(filtered, map[string]any{
			"tag":      "http-in",
			"listen":   opts.Listen,
			"port":     opts.HTTPPort,
			"protocol": "http",
			"settings": buildHTTPSettings(opts),
			"sniffing": buildSniffing(opts.Sniffing),
		})
	}

	if opts.TUN && !hasTUN {
		filtered = append(filtered, buildTUNInbound(opts))
	}

	doc["inbounds"] = filtered

	if opts.RoutingEnabled && opts.RoutingProfile != nil && opts.RoutingProfile.RawJSONMode != "off" {
		applyRoutingToRaw(doc, opts.RoutingProfile)
	}

	if opts.TUN && opts.TUNAutoDNS {
		ensureTUNDNS(doc, tunTag)
	}

	patched, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, "", "", err
	}
	return patched, socksAddr, httpAddr, nil
}

func buildLocalProxyInbounds(opts Options) []any {
	if opts.MixedInbound {
		return []any{
			map[string]any{
				"tag":      "mixed-in",
				"listen":   opts.Listen,
				"port":     opts.SocksPort,
				"protocol": "mixed",
				"settings": buildSocksSettings(opts),
				"sniffing": buildSniffing(opts.Sniffing),
			},
		}
	}

	inbounds := []any{
		map[string]any{
			"tag":      "socks-in",
			"listen":   opts.Listen,
			"port":     opts.SocksPort,
			"protocol": "socks",
			"settings": buildSocksSettings(opts),
			"sniffing": buildSniffing(opts.Sniffing),
		},
	}
	if opts.HTTPPort > 0 {
		inbounds = append(inbounds, map[string]any{
			"tag":      "http-in",
			"listen":   opts.Listen,
			"port":     opts.HTTPPort,
			"protocol": "http",
			"settings": buildHTTPSettings(opts),
			"sniffing": buildSniffing(opts.Sniffing),
		})
	}
	return inbounds
}

func buildSocksSettings(opts Options) map[string]any {
	s := map[string]any{
		"udp": true,
	}
	if opts.authActive() {
		s["auth"] = "password"
		s["accounts"] = []map[string]any{
			{"user": opts.ProxyUser, "pass": opts.ProxyPass},
		}
	} else {
		s["auth"] = "noauth"
	}
	return s
}

func buildHTTPSettings(opts Options) map[string]any {
	s := map[string]any{}
	if opts.authActive() {
		s["accounts"] = []map[string]any{
			{"user": opts.ProxyUser, "pass": opts.ProxyPass},
		}
	}
	return s
}

func buildSniffing(enabled bool) map[string]any {
	return map[string]any{
		"enabled":      enabled,
		"destOverride": []string{"http", "tls", "quic"},
	}
}

func buildTUNInbound(opts Options) map[string]any {
	return map[string]any{
		"tag":      "tun-in",
		"protocol": "tun",
		"settings": buildTUNSettings(opts),
		"sniffing": buildSniffing(opts.Sniffing),
	}
}

func buildTUNSettings(opts Options) map[string]any {
	name := opts.TUNName
	if name == "" {
		name = "xray0"
	}
	mtu := opts.TUNMTU
	if mtu <= 0 {
		mtu = 1500
	}

	gateways := []string{"172.19.0.1/30"}
	routes := []string{"0.0.0.0/0"}
	if opts.TUNIPv6 {
		gateways = append(gateways, "fdfe:dcba:9876::1/126")
		routes = append(routes, "::/0")
	}

	s := map[string]any{
		"name":                   name,
		"mtu":                    mtu,
		"gateway":                gateways,
		"autoSystemRoutingTable": routes,
		"autoOutboundsInterface": "auto",
	}
	if opts.TUNAutoDNS {
		s["autoSystemDnsToGateway"] = true
	}
	return s
}

// ensureTUNDNS добавляет минимальный блок dns, dns-outbound и перехват порта 53 с TUN,
// чтобы Xray не отклонял autoSystemDnsToGateway при старте.
func ensureTUNDNS(doc map[string]any, tunTag string) {
	if _, hasDNS := doc["dns"]; !hasDNS {
		doc["dns"] = map[string]any{
			"servers": []string{"1.1.1.1", "8.8.8.8"},
		}
	}

	outbounds, _ := doc["outbounds"].([]any)
	dnsTag := ""
	for _, item := range outbounds {
		if ob, ok := item.(map[string]any); ok {
			if p, _ := ob["protocol"].(string); p == "dns" {
				if t, _ := ob["tag"].(string); t != "" {
					dnsTag = t
					break
				}
			}
		}
	}
	if dnsTag == "" {
		dnsTag = "xray-tui-dns"
		outbounds = append(outbounds, map[string]any{
			"tag":      dnsTag,
			"protocol": "dns",
		})
		doc["outbounds"] = outbounds
	}

	routing, _ := doc["routing"].(map[string]any)
	if routing == nil {
		routing = map[string]any{"domainStrategy": "IPIfNonMatch"}
	}
	rules, _ := routing["rules"].([]any)
	dnsRule := map[string]any{
		"type":        "field",
		"inboundTag":  []string{tunTag},
		"port":        53,
		"outboundTag": dnsTag,
	}
	routing["rules"] = append([]any{dnsRule}, rules...)
	doc["routing"] = routing
}

func BuildOutbound(n *parser.Node, tag string) (map[string]any, error) {
	switch n.Protocol {
	case "vless":
		user := map[string]any{
			"id":         n.UUID,
			"encryption": n.Method,
		}
		if n.Flow != "" {
			user["flow"] = n.Flow
		}
		return map[string]any{
			"tag":      tag,
			"protocol": "vless",
			"settings": map[string]any{
				"vnext": []map[string]any{
					{
						"address": n.Address,
						"port":    n.Port,
						"users":   []map[string]any{user},
					},
				},
			},
			"streamSettings": buildStreamSettings(n.Stream),
		}, nil

	case "vmess":
		return map[string]any{
			"tag":      tag,
			"protocol": "vmess",
			"settings": map[string]any{
				"address":  n.Address,
				"port":     n.Port,
				"id":       n.UUID,
				"security": n.Method,
			},
			"streamSettings": buildStreamSettings(n.Stream),
		}, nil

	case "trojan":
		return map[string]any{
			"tag":      tag,
			"protocol": "trojan",
			"settings": map[string]any{
				"address":  n.Address,
				"port":     n.Port,
				"password": n.Password,
			},
			"streamSettings": buildStreamSettings(n.Stream),
		}, nil

	case "shadowsocks":
		out := map[string]any{
			"tag":      tag,
			"protocol": "shadowsocks",
			"settings": map[string]any{
				"address":  n.Address,
				"port":     n.Port,
				"method":   n.Method,
				"password": n.Password,
			},
		}
		if (n.Stream.Network != "" && n.Stream.Network != "tcp") ||
			(n.Stream.Security != "" && n.Stream.Security != "none") {
			out["streamSettings"] = buildStreamSettings(n.Stream)
		}
		return out, nil

	case "hysteria2":
		return buildHysteria2Outbound(n, tag), nil

	case "wireguard":
		peer := map[string]any{
			"endpoint":   net.JoinHostPort(n.Address, strconv.Itoa(n.Port)),
			"publicKey":  n.WGPublicKey,
			"allowedIPs": []string{"0.0.0.0/0", "::/0"},
		}
		if n.WGPreSharedKey != "" {
			peer["preSharedKey"] = n.WGPreSharedKey
		}
		settings := map[string]any{
			"secretKey":   n.WGSecretKey,
			"address":     n.WGAddresses,
			"peers":       []map[string]any{peer},
			"noKernelTun": true,
			"mtu":         n.WGMTU,
		}
		if len(n.WGReserved) == 3 {
			settings["reserved"] = n.WGReserved
		}
		return map[string]any{
			"tag":      tag,
			"protocol": "wireguard",
			"settings": settings,
		}, nil

	case "socks":
		settings := map[string]any{
			"address": n.Address,
			"port":    n.Port,
		}
		if n.Username != "" {
			settings["user"] = n.Username
			settings["pass"] = n.Password
		}
		return map[string]any{
			"tag":      tag,
			"protocol": "socks",
			"settings": settings,
		}, nil

	case "http":
		settings := map[string]any{
			"address": n.Address,
			"port":    n.Port,
		}
		if n.Username != "" {
			settings["user"] = n.Username
			settings["pass"] = n.Password
		}
		out := map[string]any{
			"tag":      tag,
			"protocol": "http",
			"settings": settings,
		}
		if n.Stream.Security == "tls" {
			out["streamSettings"] = buildStreamSettings(n.Stream)
		}
		return out, nil

	default:
		return nil, fmt.Errorf("генерация для протокола %q не поддерживается", n.Protocol)
	}
}

func buildHysteria2Outbound(n *parser.Node, tag string) map[string]any {
	stream := map[string]any{
		"network":  "hysteria",
		"security": "tls",
		"hysteriaSettings": map[string]any{
			"version": 2,
			"auth":    n.Password,
		},
	}

	if tlsCfg := buildTLSSettings(n.Stream); len(tlsCfg) > 0 {
		stream["tlsSettings"] = tlsCfg
	}

	finalmask := map[string]any{}
	if n.Obfs == "salamander" && n.ObfsPassword != "" {
		finalmask["udp"] = []map[string]any{
			{
				"type":     "salamander",
				"settings": map[string]any{"password": n.ObfsPassword},
			},
		}
	}

	quicParams := map[string]any{}
	if n.UpMbps != "" {
		quicParams["brutalUp"] = formatBandwidth(n.UpMbps)
	}
	if n.DownMbps != "" {
		quicParams["brutalDown"] = formatBandwidth(n.DownMbps)
	}
	if n.PortHop != "" {
		quicParams["udpHop"] = map[string]any{"ports": n.PortHop, "interval": 30}
	}
	if len(quicParams) > 0 {
		finalmask["quicParams"] = quicParams
	}
	if len(finalmask) > 0 {
		stream["finalmask"] = finalmask
	}

	return map[string]any{
		"tag":      tag,
		"protocol": "hysteria",
		"settings": map[string]any{
			"version": 2,
			"address": n.Address,
			"port":    n.Port,
		},
		"streamSettings": stream,
	}
}

func buildStreamSettings(s parser.StreamConfig) map[string]any {
	network := s.Network
	if network == "" {
		network = "tcp"
	}
	security := s.Security
	if security == "" {
		security = "none"
	}

	stream := map[string]any{
		"network":  network,
		"security": security,
	}

	switch security {
	case "tls":
		if tlsCfg := buildTLSSettings(s); len(tlsCfg) > 0 {
			stream["tlsSettings"] = tlsCfg
		}
	case "reality":
		realityCfg := map[string]any{
			"fingerprint": s.Fingerprint,
			"publicKey":   s.PublicKey,
		}
		if s.SNI != "" {
			realityCfg["serverName"] = s.SNI
		}
		if s.ShortID != "" {
			realityCfg["shortId"] = s.ShortID
		}
		if s.SpiderX != "" {
			realityCfg["spiderX"] = s.SpiderX
		}
		if s.MLDSA65Verify != "" {
			realityCfg["mldsa65Verify"] = s.MLDSA65Verify
		}
		stream["realitySettings"] = realityCfg
	}

	switch network {
	case "ws":
		ws := map[string]any{}
		if s.Path != "" {
			ws["path"] = s.Path
		}
		if s.Host != "" {
			ws["host"] = s.Host
		}
		if len(ws) > 0 {
			stream["wsSettings"] = ws
		}
	case "grpc":
		grpc := map[string]any{}
		if s.ServiceName != "" {
			grpc["serviceName"] = s.ServiceName
		}
		if strings.EqualFold(s.Mode, "multi") {
			grpc["multiMode"] = true
		}
		if s.Authority != "" {
			grpc["authority"] = s.Authority
		}
		if len(grpc) > 0 {
			stream["grpcSettings"] = grpc
		}
	case "xhttp":
		xhttp := map[string]any{}
		if s.Path != "" {
			xhttp["path"] = s.Path
		}
		if s.Host != "" {
			xhttp["host"] = s.Host
		}
		if s.Mode != "" {
			xhttp["mode"] = s.Mode
		}
		if len(s.Extra) > 0 {
			xhttp["extra"] = s.Extra
		}
		if len(xhttp) > 0 {
			stream["xhttpSettings"] = xhttp
		}
	case "httpupgrade":
		hu := map[string]any{}
		if s.Path != "" {
			hu["path"] = s.Path
		}
		if s.Host != "" {
			hu["host"] = s.Host
		}
		if len(hu) > 0 {
			stream["httpupgradeSettings"] = hu
		}
	case "mkcp":
		stream["kcpSettings"] = map[string]any{}
		if len(s.FinalMask) == 0 && (s.HeaderType != "" || s.KCPSeed != "") {
			legacy := map[string]any{}
			if s.HeaderType != "" && s.HeaderType != "none" {
				legacy["header"] = s.HeaderType
			}
			if s.KCPSeed != "" {
				legacy["value"] = s.KCPSeed
			}
			stream["finalmask"] = map[string]any{
				"udp": []map[string]any{{"type": "mkcp-legacy", "settings": legacy}},
			}
		}
	case "tcp", "raw":
		if strings.EqualFold(s.HeaderType, "http") {
			reqHeaders := map[string]any{}
			if s.Host != "" {
				reqHeaders["Host"] = strings.Split(s.Host, ",")
			}
			paths := []string{"/"}
			if s.Path != "" {
				paths = strings.Split(s.Path, ",")
			}
			key := "tcpSettings"
			if network == "raw" {
				key = "rawSettings"
			}
			stream[key] = map[string]any{
				"header": map[string]any{
					"type": "http",
					"request": map[string]any{
						"path":    paths,
						"headers": reqHeaders,
					},
				},
			}
		}
	}

	if len(s.FinalMask) > 0 {
		stream["finalmask"] = s.FinalMask
	}
	return stream
}

func buildTLSSettings(s parser.StreamConfig) map[string]any {
	tlsCfg := map[string]any{}
	if s.SNI != "" {
		tlsCfg["serverName"] = s.SNI
	}
	if s.Fingerprint != "" {
		tlsCfg["fingerprint"] = s.Fingerprint
	}
	if len(s.ALPN) > 0 {
		tlsCfg["alpn"] = s.ALPN
	}
	if s.PinnedCertSha256 != "" {
		tlsCfg["pinnedPeerCertSha256"] = s.PinnedCertSha256
	}
	if s.VerifyCertByName != "" {
		tlsCfg["verifyPeerCertByName"] = s.VerifyCertByName
	}
	if s.ECHConfigList != "" {
		tlsCfg["echConfigList"] = s.ECHConfigList
	}
	return tlsCfg
}

func formatBandwidth(raw string) string {
	raw = strings.TrimSpace(raw)
	if _, err := strconv.Atoi(raw); err == nil {
		return raw + " mbps"
	}
	return raw
}

func buildRoutingRules(rules []config.RoutingRule, proxyTag, directTag, blockTag string) []any {
	out := make([]any, 0, len(rules))
	for _, r := range rules {
		if !r.Enabled {
			continue
		}

		tag := proxyTag
		switch r.Outbound {
		case "direct":
			tag = directTag
		case "block":
			tag = blockTag
		}

		entry := map[string]any{
			"type":        "field",
			"outboundTag": tag,
		}

		hasMatcher := false
		if len(r.Domains) > 0 {
			entry["domain"] = r.Domains
			hasMatcher = true
		}
		if len(r.IPs) > 0 {
			entry["ip"] = r.IPs
			hasMatcher = true
		}
		if p := strings.TrimSpace(r.Port); p != "" {
			entry["port"] = p
			hasMatcher = true
		}
		if p := strings.TrimSpace(r.SourcePort); p != "" {
			entry["sourcePort"] = p
			hasMatcher = true
		}
		if p := strings.TrimSpace(r.LocalPort); p != "" {
			entry["localPort"] = p
			hasMatcher = true
		}
		if n := strings.TrimSpace(r.Network); n != "" {
			entry["network"] = n
			hasMatcher = true
		}
		if len(r.Source) > 0 {
			entry["source"] = r.Source
			hasMatcher = true
		}
		if len(r.LocalIP) > 0 {
			entry["localIP"] = r.LocalIP
			hasMatcher = true
		}
		if len(r.Protocol) > 0 {
			entry["protocol"] = r.Protocol
			hasMatcher = true
		}
		if len(r.InboundTag) > 0 {
			entry["inboundTag"] = r.InboundTag
			hasMatcher = true
		}
		if len(r.Attrs) > 0 {
			entry["attrs"] = r.Attrs
			hasMatcher = true
		}

		if hasMatcher {
			out = append(out, entry)
		}
	}
	return out
}

func applyRoutingToRaw(doc map[string]any, rp *config.RoutingProfile) bool {
	outbounds, _ := doc["outbounds"].([]any)
	var proxyTag, directTag, blockTag string

	for _, item := range outbounds {
		ob, ok := item.(map[string]any)
		if !ok {
			continue
		}
		proto, _ := ob["protocol"].(string)
		tag, _ := ob["tag"].(string)
		switch proto {
		case "freedom":
			if directTag == "" && tag != "" {
				directTag = tag
			}
		case "blackhole":
			if blockTag == "" && tag != "" {
				blockTag = tag
			}
		case "dns":
			continue
		default:
			if proxyTag == "" && tag != "" {
				proxyTag = tag
			}
		}
	}

	if proxyTag == "" {
		proxyTag = "proxy"
	}
	if directTag == "" {
		directTag = "xray-tui-direct"
		outbounds = append(outbounds, map[string]any{"tag": directTag, "protocol": "freedom"})
	}
	if blockTag == "" {
		blockTag = "xray-tui-block"
		outbounds = append(outbounds, map[string]any{"tag": blockTag, "protocol": "blackhole"})
	}
	doc["outbounds"] = outbounds

	userRules := buildRoutingRules(rp.Rules, proxyTag, directTag, blockTag)
	if len(userRules) == 0 && rp.RawJSONMode != "replace" {
		return false
	}

	routing, ok := doc["routing"].(map[string]any)
	if !ok || routing == nil {
		routing = map[string]any{}
	}
	if rp.DomainStrategy != "" {
		if _, hasStrat := routing["domainStrategy"]; !hasStrat || rp.RawJSONMode == "replace" {
			routing["domainStrategy"] = rp.DomainStrategy
		}
	}

	existing, _ := routing["rules"].([]any)
	switch rp.RawJSONMode {
	case "replace":
		routing["rules"] = userRules
	case "append":
		routing["rules"] = append(existing, userRules...)
	default:
		routing["rules"] = append(userRules, existing...)
	}
	doc["routing"] = routing
	return true
}
