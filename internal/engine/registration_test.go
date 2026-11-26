package engine

import (
	"fmt"
	"testing"

	xcore "github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/infra/conf/serial"
	"strings"
)

// Собирает конфиг без запуска: core.New проверяет, что все протоколы
// и транспорты зарегистрированы (иначе вернёт "... is not registered").
func buildConfig(t *testing.T, cfg string) {
	t.Helper()
	if err := tryBuildConfig(cfg); err != nil {
		t.Fatalf("создание ядра: %v", err)
	}
}

func tryBuildConfig(cfg string) error {
	parsed, err := serial.LoadJSONConfig(strings.NewReader(cfg))
	if err != nil {
		return fmt.Errorf("разбор конфига: %w", err)
	}
	instance, err := xcore.New(parsed)
	if err != nil {
		return err
	}
	_ = instance.Close()
	return nil
}

// Регрессия: набор побочных импортов в register.go должен покрывать все
// протоколы и транспорты, которые умеет писать наш генератор.
func TestRegisteredProtocolsAndTransports(t *testing.T) {
	outbounds := []struct {
		name string
		json string
	}{
		{"vless", `{"tag":"o","protocol":"vless","settings":{"vnext":[{"address":"a.example","port":443,"users":[{"id":"11111111-1111-1111-1111-111111111111","encryption":"none"}]}]}}`},
		{"vmess", `{"tag":"o","protocol":"vmess","settings":{"vnext":[{"address":"a.example","port":443,"users":[{"id":"11111111-1111-1111-1111-111111111111"}]}]}}`},
		{"trojan", `{"tag":"o","protocol":"trojan","settings":{"servers":[{"address":"a.example","port":443,"password":"secret"}]}}`},
		{"shadowsocks", `{"tag":"o","protocol":"shadowsocks","settings":{"servers":[{"address":"a.example","port":8388,"password":"secret","method":"aes-256-gcm"}]}}`},
		{"wireguard", `{"tag":"o","protocol":"wireguard","settings":{"secretKey":"aGVsbG8","address":["10.0.0.2/32"],"peers":[{"publicKey":"B8dsj8zqONCr5K3VF1kjW-6NNwiyUG0UXX5K07-hrAI","endpoint":"a.example:51820"}]}}`},
		{"freedom", `{"tag":"o","protocol":"freedom"}`},
		{"blackhole", `{"tag":"o","protocol":"blackhole"}`},
		{"dns-out", `{"tag":"o","protocol":"dns"}`},
	}
	for _, ob := range outbounds {
		t.Run("outbound/"+ob.name, func(t *testing.T) {
			buildConfig(t, fmt.Sprintf(`{"inbounds":[],"outbounds":[%s]}`, ob.json))
		})
	}

	networks := []struct{ name, stream string }{
		{"tcp", `{"network":"tcp"}`},
		{"ws", `{"network":"ws","wsSettings":{"path":"/x"}}`},
		{"grpc", `{"network":"grpc","grpcSettings":{"serviceName":"svc"}}`},
		{"httpupgrade", `{"network":"httpupgrade","httpupgradeSettings":{"path":"/x","host":"a.example"}}`},
		{"xhttp", `{"network":"xhttp","xhttpSettings":{"path":"/x","host":"a.example"}}`},
		{"kcp", `{"network":"kcp","kcpSettings":{"mtu":1350}}`},
		{"tls", `{"network":"tcp","security":"tls","tlsSettings":{"serverName":"a.example"}}`},
		{"reality", `{"network":"tcp","security":"reality","realitySettings":{"serverName":"a.example","publicKey":"B8dsj8zqONCr5K3VF1kjW-6NNwiyUG0UXX5K07-hrAI","shortId":"aaaa","fingerprint":"chrome"}}`},
	}
	for _, nw := range networks {
		t.Run("stream/"+nw.name, func(t *testing.T) {
			buildConfig(t, fmt.Sprintf(`{"inbounds":[],"outbounds":[{"tag":"o","protocol":"vless","streamSettings":%s,"settings":{"vnext":[{"address":"a.example","port":443,"users":[{"id":"11111111-1111-1111-1111-111111111111","encryption":"none"}]}]}}]}`, nw.stream))
		})
	}
}

func TestRegisteredInbounds(t *testing.T) {
	inbounds := []struct{ name, json string }{
		{"socks", `{"tag":"i","listen":"127.0.0.1","port":1080,"protocol":"socks","settings":{"udp":true}}`},
		{"http", `{"tag":"i","listen":"127.0.0.1","port":8080,"protocol":"http"}`},
		{"tun", `{"tag":"i","protocol":"tun","settings":{"name":"xray0","MTU":1500}}`},
	}
	for _, in := range inbounds {
		t.Run(in.name, func(t *testing.T) {
			cfg := fmt.Sprintf(`{"inbounds":[%s],"outbounds":[{"tag":"o","protocol":"freedom"}]}`, in.json)
			// TUN создаёт адаптер прямо при создании ядра, поэтому без прав
			// ожидаем EPERM — но не «протокол не зарегистрирован».
			if err := tryBuildConfig(cfg); err != nil {
				if strings.Contains(err.Error(), "not registered") {
					t.Fatalf("%s не зарегистрирован: %v", in.name, err)
				}
				t.Logf("%s: ожидаемая ошибка окружения: %v", in.name, err)
			}
		})
	}
}

// Роутинг с geosite/geoip разбирается и создаётся: сами базы при этом не нужны,
// они понадобятся только на старте (генератор проверяет их наличие отдельно).
func TestRegisteredRouterWithGeoRules(t *testing.T) {
	buildConfig(t, `{
      "inbounds": [{"tag":"i","listen":"127.0.0.1","port":1080,"protocol":"socks"}],
      "outbounds": [{"tag":"direct","protocol":"freedom"},{"tag":"proxy","protocol":"freedom"}],
      "routing": {"domainStrategy":"IPIfNonMatch","rules":[
        {"type":"field","domain":["geosite:private"],"outboundTag":"direct"},
        {"type":"field","ip":["geoip:private"],"outboundTag":"direct"},
        {"type":"field","domain":["geosite:category-ads-all"],"outboundTag":"proxy"}
      ]}
    }`)
}
