package ping

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var icmpTimeRe = regexp.MustCompile(`time[=<]\s*([0-9]+(?:\.[0-9]+)?)\s*ms`)

func resolveHostIP(ctx context.Context, host string) (string, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return "", errors.New("пустой адрес")
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.String(), nil
	}
	rCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	ips, err := net.DefaultResolver.LookupIPAddr(rCtx, host)
	if err != nil || len(ips) == 0 {
		return "", fmt.Errorf("dns: %w", err)
	}
	for _, ip := range ips {
		if ip.IP.To4() != nil {
			return ip.IP.String(), nil
		}
	}
	return ips[0].IP.String(), nil
}

// MeasureICMP замеряет ICMP RTT до узла, пытаясь обойти TUN через bypassIface.
func MeasureICMP(ctx context.Context, host string, bypassIface string) (time.Duration, error) {
	ip, err := resolveHostIP(ctx, host)
	if err != nil {
		return 0, err
	}

	runPing := func(iface string) (time.Duration, error) {
		pCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()

		out, err := exec.CommandContext(pCtx, "ping", icmpArgs(ip, iface)...).CombinedOutput()
		if m := icmpTimeRe.FindSubmatch(out); len(m) == 2 {
			if ms, parseErr := strconv.ParseFloat(string(m[1]), 64); parseErr == nil {
				return time.Duration(ms * float64(time.Millisecond)), nil
			}
		}
		if err != nil {
			return 0, fmt.Errorf("icmp: %s", strings.TrimSpace(string(out)))
		}
		return 0, errors.New("icmp: нет ответа")
	}

	if bypassIface != "" {
		if rtt, err := runPing(bypassIface); err == nil {
			return rtt, nil
		}
	}
	return runPing("")
}
