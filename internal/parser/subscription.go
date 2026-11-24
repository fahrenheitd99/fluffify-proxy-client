package parser

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type FetchOptions struct {
	UserAgent   string
	SendHWID    bool
	HWID        string
	DeviceOS    string
	DeviceVer   string
	DeviceModel string
	Timeout     time.Duration
}

type SubItem struct {
	Type     string // "uri" или "raw"
	Name     string
	Protocol string
	Address  string
	Port     int
	URI      string
	RawJSON  json.RawMessage
	Node     *Node
}

type SubUserInfo struct {
	Upload   int64
	Download int64
	Total    int64
	Expire   int64
}

type SubResult struct {
	Title    string
	Items    []SubItem
	Skipped  int
	Warnings []string
	UserInfo *SubUserInfo
}

func FetchSubscription(ctx context.Context, subURL string, opts FetchOptions) (*SubResult, error) {
	if opts.Timeout == 0 {
		opts.Timeout = 15 * time.Second
	}
	if opts.UserAgent == "" {
		opts.UserAgent = "Fluffify/1.0.0"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, subURL, nil)
	if err != nil {
		return nil, fmt.Errorf("невалидный URL подписки: %w", err)
	}

	req.Header.Set("User-Agent", opts.UserAgent)
	req.Header.Set("Accept", "*/*")

	if opts.SendHWID && opts.HWID != "" {
		req.Header.Set("x-hwid", opts.HWID)
		if opts.DeviceOS != "" {
			req.Header.Set("x-device-os", opts.DeviceOS)
		}
		if opts.DeviceVer != "" {
			req.Header.Set("x-ver-os", opts.DeviceVer)
		}
		if opts.DeviceModel != "" {
			req.Header.Set("x-device-model", opts.DeviceModel)
		}
	}

	client := &http.Client{Timeout: opts.Timeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ошибка загрузки подписки: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("сервер подписки вернул HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения ответа подписки: %w", err)
	}

	items, skipped, warnings, err := ParseSubscriptionBody(body)
	if err != nil {
		return nil, err
	}

	return &SubResult{
		Title:    parseProfileTitle(resp.Header.Get("profile-title")),
		Items:    items,
		Skipped:  skipped,
		Warnings: warnings,
		UserInfo: parseSubUserInfo(resp.Header.Get("subscription-userinfo")),
	}, nil
}

func ParseSubscriptionBody(body []byte) ([]SubItem, int, []string, error) {
	// Убираем UTF-8 BOM и лишние пробелы
	trimmed := bytes.TrimSpace(bytes.TrimPrefix(body, []byte{0xEF, 0xBB, 0xBF}))
	if len(trimmed) == 0 {
		return nil, 0, nil, errors.New("подписка вернула пустой ответ")
	}

	// 1. Сырой JSON (один конфиг или массив конфигов Xray)
	if json.Valid(trimmed) {
		items, err := parseJSONSubscription(trimmed)
		if err == nil && len(items) > 0 {
			return items, 0, nil, nil
		}
	}

	// 2. Если в теле нет "://", пробуем декодировать Base64
	text := string(trimmed)
	if !strings.Contains(text, "://") {
		if decoded, err := decodeBase64Flex(text); err == nil {
			decTrim := bytes.TrimSpace(bytes.TrimPrefix(decoded, []byte{0xEF, 0xBB, 0xBF}))
			if json.Valid(decTrim) {
				if items, jErr := parseJSONSubscription(decTrim); jErr == nil && len(items) > 0 {
					return items, 0, nil, nil
				}
			}
			text = string(decTrim)
		}
	}

	// 3. Построчный разбор URI-ссылок
	var (
		items    []SubItem
		skipped  int
		warnings []string
	)

	scanner := bufio.NewScanner(strings.NewReader(text))
	scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		if !strings.Contains(line, "://") {
			continue
		}

		node, err := ParseURI(line)
		if err != nil {
			skipped++
			if len(warnings) < 5 {
				warnings = append(warnings, err.Error())
			}
			continue
		}

		items = append(items, SubItem{
			Type:     "uri",
			Name:     node.Name,
			Protocol: node.Protocol,
			Address:  node.Address,
			Port:     node.Port,
			URI:      line,
			Node:     node,
		})
	}

	if len(items) == 0 {
		if len(warnings) > 0 {
			return nil, skipped, warnings, fmt.Errorf("не удалось разобрать ни одной ноды (первая ошибка: %s)", warnings[0])
		}
		return nil, skipped, warnings, errors.New("в ответе подписки не найдено поддерживаемых ссылок или Xray JSON")
	}

	return items, skipped, warnings, nil
}

func parseJSONSubscription(data []byte) ([]SubItem, error) {
	if data[0] == '[' {
		var list []json.RawMessage
		if err := json.Unmarshal(data, &list); err != nil {
			return nil, err
		}
		items := make([]SubItem, 0, len(list))
		for i, raw := range list {
			item, ok := InspectRawConfig(raw, fmt.Sprintf("JSON #%d", i+1))
			if ok {
				items = append(items, item)
			}
		}
		if len(items) == 0 {
			return nil, errors.New("JSON-массив не содержит валидных конфигов Xray")
		}
		return items, nil
	}

	if data[0] == '{' {
		item, ok := InspectRawConfig(json.RawMessage(data), "Raw JSON")
		if ok {
			return []SubItem{item}, nil
		}
	}

	return nil, errors.New("JSON не является конфигом Xray")
}

// InspectRawConfig извлекает имя (remarks) и метаданные первого outbound из Raw JSON.
func InspectRawConfig(raw json.RawMessage, fallbackName string) (SubItem, bool) {
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return SubItem{}, false
	}

	outbounds, ok := doc["outbounds"].([]any)
	if !ok || len(outbounds) == 0 {
		return SubItem{}, false
	}

	name := ""
	for _, key := range []string{"remarks", "remark", "name", "tag"} {
		if v, ok := doc[key].(string); ok && strings.TrimSpace(v) != "" {
			name = strings.TrimSpace(v)
			break
		}
	}

	proto := "raw"
	addr := ""
	port := 0

	if first, ok := outbounds[0].(map[string]any); ok {
		if p, ok := first["protocol"].(string); ok && p != "" {
			proto = "raw/" + p
		}
		if settings, ok := first["settings"].(map[string]any); ok {
			if a, ok := settings["address"].(string); ok {
				addr = a
			}
			if p, ok := settings["port"].(float64); ok {
				port = int(p)
			}
			if vnext, ok := settings["vnext"].([]any); ok && len(vnext) > 0 {
				if vn, ok := vnext[0].(map[string]any); ok {
					if a, ok := vn["address"].(string); ok && addr == "" {
						addr = a
					}
					if p, ok := vn["port"].(float64); ok && port == 0 {
						port = int(p)
					}
				}
			}
		}
	}

	if name == "" {
		if addr != "" {
			name = addr
		} else {
			name = fallbackName
		}
	}

	return SubItem{
		Type:     "raw",
		Name:     name,
		Protocol: proto,
		Address:  addr,
		Port:     port,
		RawJSON:  raw,
	}, true
}

func parseProfileTitle(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if after, ok := strings.CutPrefix(strings.ToLower(raw), "base64:"); ok {
		// Берём срез из исходной строки, чтобы сохранить регистр base64
		b64 := strings.TrimSpace(raw[len(raw)-len(after):])
		if dec, err := decodeBase64Flex(b64); err == nil {
			return strings.TrimSpace(string(dec))
		}
	}
	if unesc, err := url.PathUnescape(raw); err == nil {
		return strings.TrimSpace(unesc)
	}
	return raw
}

func parseSubUserInfo(header string) *SubUserInfo {
	header = strings.TrimSpace(header)
	if header == "" {
		return nil
	}

	info := &SubUserInfo{}
	found := false
	for _, part := range strings.Split(header, ";") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		num, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		if err != nil {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(k)) {
		case "upload":
			info.Upload = num
			found = true
		case "download":
			info.Download = num
			found = true
		case "total":
			info.Total = num
			found = true
		case "expire":
			info.Expire = num
			found = true
		}
	}
	if !found {
		return nil
	}
	return info
}
