package ping

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var icmpTimeRe = regexp.MustCompile(`time[=<]\s*([0-9]+(?:\.[0-9]+)?)\s*ms`)

// PhysicalIface находит активный физический интерфейс в /proc/net/route, исключая TUN и lo.
func PhysicalIface(exclude string) string {
	f, err := os.Open("/proc/net/route")
	if err != nil {
		return ""
	}
	defer f.Close()

	var fallback string
	sc := bufio.NewScanner(f)
	first := true
	for sc.Scan() {
		if first {
			first = false
			continue
		}
		fields := strings.Fields(sc.Text())
		if len(fields) < 3 {
			continue
		}
		iface, dest, gw := fields[0], fields[1], fields[2]
		if iface == "lo" || (exclude != "" && iface == exclude) {
			continue
		}
		if dest == "00000000" {
			return iface
		}
		if fallback == "" && gw != "00000000" {
			fallback = iface
		}
	}
	return fallback
}

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

		args := []string{"-n", "-c", "1", "-W", "2"}
		if iface != "" {
			args = append(args, "-I", iface)
		}
		args = append(args, ip)

		out, err := exec.CommandContext(pCtx, "ping", args...).CombinedOutput()
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