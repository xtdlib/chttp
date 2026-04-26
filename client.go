package chttp

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"sync"
	"time"

	"go.uber.org/ratelimit"
)

type options struct {
	cacheTTL  time.Duration
	rps       int
	editReq   func(*http.Request) error
}

// Option configures a Client.
type Option func(*options)

// WithRateLimit sets the maximum requests per second. 0 means no limit.
func WithRateLimit(rps int) Option {
	return func(o *options) {
		o.rps = rps
	}
}

// WithCookieTimeout sets how long cookies and user-agent are cached
// before refreshing from Chrome. Default is 5 minutes.
func WithCookieTimeout(d time.Duration) Option {
	return func(o *options) {
		o.cacheTTL = d
	}
}

// WithRequestEditor registers a function that is called on every
// outgoing request just before it is sent. Use it to attach headers
// (e.g. Authorization) or otherwise mutate the request. If it returns
// an error, the request is aborted and the error is returned to the
// caller of Client.Do.
func WithRequestEditor(fn func(*http.Request) error) Option {
	return func(o *options) {
		o.editReq = fn
	}
}

// NewClient returns an *http.Client that mimics a real browser.
// It connects to Chrome via CDP to fetch cookies and user-agent,
// then refreshes them in the background on the configured TTL.
func NewClient(cdpAddr string, opts ...Option) *http.Client {
	if cdpAddr == "" {
		cdpAddr = os.Getenv("CHTTP_CDP_ADDR")
	}
	if cdpAddr == "" {
		cdpAddr = "ws://localhost:9222"
	}

	o := &options{
		cacheTTL: 5 * time.Minute,
	}

	for _, opt := range opts {
		opt(o)
	}

	jar, _ := cookiejar.New(nil)

	t := &transport{
		base:    http.DefaultTransport,
		cdpAddr: cdpAddr,
		opts:    o,
		jar:     jar,
	}
	if o.rps > 0 {
		t.rl = ratelimit.New(o.rps)
	}

	if err := t.refresh(context.Background()); err != nil {
		slog.Error(fmt.Sprintf("chttp: %v", err))
	}
	go t.refreshLoop()

	return &http.Client{
		Jar:       jar,
		Transport: t,
	}
}

type transport struct {
	base    http.RoundTripper
	cdpAddr string
	opts    *options
	jar     *cookiejar.Jar
	rl      ratelimit.Limiter

	mu        sync.Mutex
	userAgent string
}

func (t *transport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.rl != nil {
		t.rl.Take()
	}

	if t.userAgent != "" {
		req.Header.Set("User-Agent", t.userAgent)
	}

	if t.opts.editReq != nil {
		if err := t.opts.editReq(req); err != nil {
			return nil, err
		}
	}

	return t.base.RoundTrip(req)
}

func (t *transport) refresh(ctx context.Context) error {
	ua, cookies, err := t.fetchFromCDP(ctx)
	if err != nil {
		return err
	}

	t.mu.Lock()
	t.userAgent = ua
	t.mu.Unlock()
	t.setCookies(cookies)
	return nil
}

func (t *transport) refreshLoop() {
	for {
		time.Sleep(t.opts.cacheTTL)
		t.refresh(context.Background())
	}
}

func (t *transport) fetchFromCDP(ctx context.Context) (string, []*cookie, error) {
	cdp, err := createCDPClient(ctx, t.cdpAddr)
	if err != nil {
		return "", nil, err
	}
	defer cdp.Close()

	ua, err := cdp.fetchUserAgent(ctx)
	if err != nil {
		return "", nil, err
	}

	cookies, err := cdp.fetchCookies(ctx)
	if err != nil {
		return "", nil, err
	}

	return ua, cookies, nil
}

func validCookieValue(v string) bool {
	for i := range len(v) {
		b := v[i]
		if !(0x20 <= b && b < 0x7f && b != '"' && b != ';' && b != '\\') {
			return false
		}
	}
	return true
}

func (t *transport) setCookies(cookies []*cookie) {
	type originKey struct {
		domain string
		path   string
	}
	urls := make(map[originKey]*url.URL)
	grouped := make(map[originKey][]*http.Cookie)

	for _, c := range cookies {
		if !validCookieValue(c.Value) {
			continue
		}
		domain := c.Domain
		if len(domain) > 0 && domain[0] == '.' {
			domain = domain[1:]
		}
		key := originKey{domain, c.Path}
		if _, ok := urls[key]; !ok {
			urls[key] = &url.URL{Scheme: "https", Host: domain, Path: c.Path}
		}
		hc := &http.Cookie{
			Name:     c.Name,
			Value:    c.Value,
			Path:     c.Path,
			Secure:   c.Secure,
			HttpOnly: c.HTTPOnly,
		}

		if len(c.Domain) > 0 && c.Domain[0] == '.' {
			hc.Domain = c.Domain
		}
		grouped[key] = append(grouped[key], hc)
	}

	for key, hc := range grouped {
		t.jar.SetCookies(urls[key], hc)
	}
}
