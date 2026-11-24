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
	"syscall"
	"time"
)

type Client struct {
	socketPath   string
	httpClient   *http.Client
	streamClient *http.Client
}

func NewClient(socketPath string) *Client {
	if socketPath == "" {
		socketPath = DefaultSocketPath()
	}

	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{Timeout: 2 * time.Second}).DialContext(ctx, "unix", socketPath)
		},
	}

	return &Client{
		socketPath:   socketPath,
		httpClient:   &http.Client{Transport: transport, Timeout: 10 * time.Second},
		streamClient: &http.Client{Transport: transport}, // без Timeout для SSE
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

func wrapConnErr(err error) error {
	if errors.Is(err, syscall.ENOENT) || errors.Is(err, syscall.ECONNREFUSED) {
		return errors.New("демон не запущен (выполни в отдельном окне: xray-tui daemon)")
	}
	return err
}
