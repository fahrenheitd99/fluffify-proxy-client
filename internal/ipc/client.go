package ipc

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	socketPath   string
	httpClient   *http.Client
	streamClient *http.Client
	slowClient   *http.Client // для медленных dev-команд (проверка туннеля)
}

func NewClient(socketPath string) *Client {
	if socketPath == "" {
		socketPath = DefaultSocketPath()
	}

	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return Dial(ctx, socketPath)
		},
	}

	return &Client{
		socketPath:   socketPath,
		httpClient:   &http.Client{Transport: transport, Timeout: 10 * time.Second},
		streamClient: &http.Client{Transport: transport}, // без Timeout для SSE
		slowClient:   &http.Client{Transport: transport, Timeout: 30 * time.Second},
	}
}

func (c *Client) Status() (*StatusResponse, error) {
	var resp StatusResponse
	if err := c.doJSON(http.MethodGet, "/status", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) Start(req StartRequest) (*StatusResponse, error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	var resp StatusResponse
	if err := c.doJSON(http.MethodPost, "/start", bytes.NewReader(payload), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) Stop() (*StatusResponse, error) {
	var resp StatusResponse
	if err := c.doJSON(http.MethodPost, "/stop", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Quit просит демон завершиться целиком (нужно для перезапуска с правами админа).
func (c *Client) Quit() error {
	return c.doJSON(http.MethodPost, "/quit", nil, nil)
}

func (c *Client) Logs() ([]string, error) {
	var resp LogsResponse
	if err := c.doJSON(http.MethodGet, "/logs", nil, &resp); err != nil {
		return nil, err
	}
	return resp.Lines, nil
}

func (c *Client) StreamLogs(ctx context.Context, onLine func(string)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://unix/logs/stream", nil)
	if err != nil {
		return err
	}

	resp, err := c.streamClient.Do(req)
	if err != nil {
		return wrapConnErr(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("сервер вернул статус %s", resp.Status)
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if after, ok := strings.CutPrefix(line, "data: "); ok {
			onLine(after)
		}
	}

	if ctx.Err() != nil {
		return nil
	}
	return scanner.Err()
}

func (c *Client) doJSON(method, path string, body io.Reader, out any) error {
	req, err := http.NewRequest(method, "http://unix"+path, body)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return wrapConnErr(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		var apiErr ErrorResponse
		if err := json.NewDecoder(resp.Body).Decode(&apiErr); err == nil && apiErr.Error != "" {
			return errors.New(apiErr.Error)
		}
		return fmt.Errorf("ошибка демона: %s", resp.Status)
	}

	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

// doJSONWithTimeout — то же, но с собственным таймаутом клиента (dev-команды,
// которые могут выполняться несколько секунд).
func (c *Client) doJSONWithTimeout(method, path string, body io.Reader, out any) error {
	req, err := http.NewRequest(method, "http://unix"+path, body)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.slowClient.Do(req)
	if err != nil {
		return wrapConnErr(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		var apiErr ErrorResponse
		if err := json.NewDecoder(resp.Body).Decode(&apiErr); err == nil && apiErr.Error != "" {
			return errors.New(apiErr.Error)
		}
		return fmt.Errorf("ошибка демона: %s", resp.Status)
	}

	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

// DevLog включает/выключает режим отладочных логов ядра (info + зеркало
// в файл во внешнем каталоге журналов).
func (c *Client) DevLog(on bool) error {
	payload, err := json.Marshal(DevLogRequest{On: on})
	if err != nil {
		return err
	}
	return c.doJSON(http.MethodPost, "/devlog", bytes.NewReader(payload), nil)
}

// DevStatus — сведения о запущенном ядре.
func (c *Client) DevStatus() (*DevStatusResponse, error) {
	var resp DevStatusResponse
	if err := c.doJSON(http.MethodGet, "/devstatus", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// DevConfig — дамп последнего конфига ядра (с секретами, замаскированными
// на стороне демона).
func (c *Client) DevConfig() (*DevConfigResponse, error) {
	var resp DevConfigResponse
	if err := c.doJSON(http.MethodGet, "/devconfig", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// DevCheck — проверка туннеля (внешний IP через туннель/мимо, DNS-резолвер).
func (c *Client) DevCheck() (*DevCheckResponse, error) {
	var resp DevCheckResponse
	if err := c.doJSONWithTimeout(http.MethodPost, "/devcheck", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func wrapConnErr(err error) error {
	if IsDaemonDown(err) {
		return errors.New("демон не запущен (выполни в отдельном окне: xray-tui daemon)")
	}
	return err
}
