package chttp

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
)

// cdpClient is a simple Chrome DevTools Protocol client
type cdpClient struct {
	conn   *websocket.Conn
	nextID atomic.Int64
}

// createCDPClient connects to Chrome's debugging port
func createCDPClient(ctx context.Context, debugURL string) (*cdpClient, error) {
	// Get WebSocket URL from the debug endpoint
	wsURL, err := getWebSocketURL(ctx, debugURL)
	if err != nil {
		return nil, fmt.Errorf("failed to get websocket URL: %w", err)
	}

	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Chrome: %w", err)
	}

	// Set read limit to 10MB to handle large cookie responses
	conn.SetReadLimit(10 * 1024 * 1024)

	return &cdpClient{conn: conn}, nil
}

// Close closes the WebSocket connection
func (c *cdpClient) Close() error {
	return c.conn.Close(websocket.StatusNormalClosure, "")
}

// execute sends a CDP command and returns the response
func (c *cdpClient) execute(pctx context.Context, method string, params any) (json.RawMessage, error) {
	id := c.nextID.Add(1)

	ctx, cancel := context.WithTimeout(pctx, 10*time.Second)
	defer cancel()

	request := map[string]any{
		"id":     id,
		"method": method,
	}
	if params != nil {
		request["params"] = params
	}

	// Send request
	if err := c.conn.Write(ctx, websocket.MessageText, mustMarshal(request)); err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	// go func() {
	// 	c.conn.Ping(ctx)
	// }

	// Read response
	for {
		_, data, err := c.conn.Read(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to read response: %w", err)
		}

		var response struct {
			ID     int64           `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}

		if err := json.Unmarshal(data, &response); err != nil {
			return nil, fmt.Errorf("failed to parse CDP response: %w", err)
		}

		if response.ID != id {
			continue // Not our response
		}

		if response.Error != nil {
			return nil, fmt.Errorf("CDP error %d: %s", response.Error.Code, response.Error.Message)
		}

		return response.Result, nil
	}
}

// getWebSocketURL queries the Chrome debug endpoint to get the WebSocket URL
func getWebSocketURL(ctx context.Context, urlstr string) (string, error) {
	lctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if strings.Contains(urlstr, "/devtools/browser/") {
		return forceIP(lctx, urlstr)
	}

	// replace the scheme and path to construct a URL like:
	// http://127.0.0.1:9222/json/version
	u, err := url.Parse(urlstr)
	if err != nil {
		return "", err
	}
	u.Scheme = "http"
	host, port, err := net.SplitHostPort(u.Host)
	if err != nil {
		return "", err
	}
	host, err = resolveHost(ctx, host)
	if err != nil {
		return "", err
	}
	u.Host = net.JoinHostPort(host, port)
	u.Path = "/json/version"

	// to get "webSocketDebuggerUrl" in the response
	req, err := http.NewRequestWithContext(lctx, "GET", u.String(), nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	// the browser will construct the debugger URL using the "host" header of
	// the /json/version request. For example, run headless-shell in a container:
	//     docker run -d -p 9000:9222 chromedp/headless-shell:latest
	// then:
	//     curl http://127.0.0.1:9000/json/version
	// and the websocket debugger URL will be something like:
	// ws://127.0.0.1:9000/devtools/browser/...
	wsURL, ok := result["webSocketDebuggerUrl"].(string)
	if !ok {
		return "", fmt.Errorf("webSocketDebuggerUrl not found in response")
	}
	return wsURL, nil
}

func mustMarshal(v interface{}) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
}

func forceIP(ctx context.Context, urlstr string) (string, error) {
	u, err := url.Parse(urlstr)
	if err != nil {
		return "", err
	}
	host, port, err := net.SplitHostPort(u.Host)
	if err != nil {
		return "", err
	}
	host, err = resolveHost(ctx, host)
	if err != nil {
		return "", err
	}
	u.Host = net.JoinHostPort(host, port)
	return u.String(), nil
}

// resolveHost tries to resolve a host to be an IP address. If the host is
// an IP address or "localhost", it returns the host directly.
func resolveHost(ctx context.Context, host string) (string, error) {
	if host == "localhost" {
		return host, nil
	}
	ip := net.ParseIP(host)
	if ip != nil {
		return host, nil
	}

	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return "", err
	}

	return addrs[0].IP.String(), nil
}

func (client *cdpClient) navigate(ctx context.Context, targetURL string) error {
	result, err := client.execute(ctx, "Target.createTarget", map[string]string{"url": targetURL})
	if err != nil {
		return err
	}
	var target struct {
		TargetID string `json:"targetId"`
	}
	if err := json.Unmarshal(result, &target); err != nil {
		return err
	}
	// wait for page to load (httpbin cookie endpoints redirect)
	time.Sleep(3 * time.Second)
	_, err = client.execute(ctx, "Target.closeTarget", map[string]string{"targetId": target.TargetID})
	return err
}

func (client *cdpClient) fetchUserAgent(ctx context.Context) (string, error) {
	result, err := client.execute(ctx, "Browser.getVersion", nil)
	if err != nil {
		return "", fmt.Errorf("failed to get browser version: %w", err)
	}

	var version getVersionResponse
	if err := json.Unmarshal(result, &version); err != nil {
		return "", fmt.Errorf("failed to parse version response: %w", err)
	}

	return version.UserAgent, nil
}

// fetchCookies fetches cookies from Chrome (internal method)
func (client *cdpClient) fetchCookies(ctx context.Context) ([]*cookie, error) {
	result, err := client.execute(ctx, "Storage.getCookies", nil)
	if err != nil {
		return nil, fmt.Errorf("chttp: failed to get cookies: %w", err)
	}

	var response getCookiesResponses
	if err := json.Unmarshal(result, &response); err != nil {
		return nil, fmt.Errorf("chttp: failed to parse cookies response: %w", err)
	}

	return response.Cookies, nil
}
