package mihomo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// Client talks to a Mihomo controller via HTTP and WebSocket APIs.
type Client struct {
	HTTPClient *http.Client
	Dialer     *websocket.Dialer
}

// NewClient constructs a Mihomo API client with sensible timeouts.
func NewClient() *Client {
	return &Client{
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
		Dialer: &websocket.Dialer{
			HandshakeTimeout: 10 * time.Second,
			Proxy:            http.ProxyFromEnvironment,
		},
	}
}

func (c *Client) httpClient() *http.Client {
	if c != nil && c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: 10 * time.Second}
}

func (c *Client) wsDialer() *websocket.Dialer {
	if c != nil && c.Dialer != nil {
		return c.Dialer
	}
	return &websocket.Dialer{HandshakeTimeout: 10 * time.Second}
}

func (c *Client) doJSON(ctx context.Context, settings Settings, method, path string, body any, out any) error {
	settings = settings.Normalize()
	if settings.URL == "" {
		return errors.New("mihomo url is not configured")
	}

	var reader *bytes.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(encoded)
	} else {
		reader = bytes.NewReader(nil)
	}

	req, err := http.NewRequestWithContext(ctx, method, settings.URL+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if settings.Secret != "" {
		req.Header.Set("Authorization", "Bearer "+settings.Secret)
	}

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("mihomo %s %s: unexpected status %d", method, path, resp.StatusCode)
	}

	if out != nil && resp.StatusCode != http.StatusNoContent {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return err
		}
	}
	return nil
}

// GetVersion fetches Mihomo kernel version information.
func (c *Client) GetVersion(ctx context.Context, settings Settings) (VersionInfo, error) {
	var info VersionInfo
	err := c.doJSON(ctx, settings, http.MethodGet, "/version", nil, &info)
	return info, err
}

// GetConfigs fetches Mihomo runtime configuration including DNS enhanced-mode and fake-ip-range.
func (c *Client) GetConfigs(ctx context.Context, settings Settings) (KernelConfig, error) {
	var cfg KernelConfig
	err := c.doJSON(ctx, settings, http.MethodGet, "/configs", nil, &cfg)
	return cfg, err
}

// GetConnections polls GET /connections for a single snapshot.
func (c *Client) GetConnections(ctx context.Context, settings Settings) (*ConnectionsSnapshot, error) {
	var snapshot ConnectionsSnapshot
	if err := c.doJSON(ctx, settings, http.MethodGet, "/connections", nil, &snapshot); err != nil {
		return nil, err
	}
	return &snapshot, nil
}

// StreamConnections opens a WebSocket stream to /connections and delivers snapshots
// until ctx is cancelled or the connection fails.
func (c *Client) StreamConnections(ctx context.Context, settings Settings, onSnapshot func(*ConnectionsSnapshot) error) error {
	settings = settings.Normalize()
	if settings.URL == "" {
		return errors.New("mihomo url is not configured")
	}

	wsURL, err := buildConnectionsWSURL(settings)
	if err != nil {
		return err
	}

	headers := http.Header{}
	if settings.Secret != "" {
		headers.Set("Authorization", "Bearer "+settings.Secret)
	}

	conn, resp, err := c.wsDialer().DialContext(ctx, wsURL, headers)
	if err != nil {
		if resp != nil {
			defer resp.Body.Close()
			return fmt.Errorf("ws dial status %d: %w", resp.StatusCode, err)
		}
		return err
	}
	defer conn.Close()

	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.SetReadDeadline(time.Now())
			_ = conn.Close()
		case <-done:
		}
	}()
	defer close(done)

	for {
		_ = conn.SetReadDeadline(time.Now().Add(30 * time.Second))
		_, message, err := conn.ReadMessage()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}

		var snapshot ConnectionsSnapshot
		if err := json.Unmarshal(message, &snapshot); err != nil {
			return fmt.Errorf("decode ws snapshot: %w", err)
		}
		if err := onSnapshot(&snapshot); err != nil {
			return err
		}
	}
}

func buildConnectionsWSURL(settings Settings) (string, error) {
	parsed, err := url.Parse(settings.URL)
	if err != nil {
		return "", err
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http":
		parsed.Scheme = "ws"
	case "https":
		parsed.Scheme = "wss"
	case "ws", "wss":
	default:
		return "", fmt.Errorf("unsupported mihomo url scheme %q", parsed.Scheme)
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/connections"
	if settings.Secret != "" {
		q := parsed.Query()
		q.Set("token", settings.Secret)
		parsed.RawQuery = q.Encode()
	}
	return parsed.String(), nil
}

// CloseConnection terminates a single active connection in Mihomo.
func (c *Client) CloseConnection(ctx context.Context, settings Settings, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("connection id is required")
	}
	return c.doJSON(ctx, settings, http.MethodDelete, "/connections/"+url.PathEscape(id), nil, nil)
}

// CloseAllConnections terminates all active connections in Mihomo.
func (c *Client) CloseAllConnections(ctx context.Context, settings Settings) error {
	return c.doJSON(ctx, settings, http.MethodDelete, "/connections", nil, nil)
}
