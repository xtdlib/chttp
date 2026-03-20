package chttp

import (
	"context"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"sync"
	"time"

	"go.uber.org/ratelimit"
)

type options struct {
	cacheTTL time.Duration
	rps      int
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

// NewClient returns an *http.Client that mimics a real browser.
// It lazily connects to Chrome via CDP on the first request to fetch
// cookies and user-agent, then caches them for the configured TTL.
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
	lastFetch time.Time
}

func (t *transport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.rl != nil {
		t.rl.Take()
	}

	refreshed, err := t.ensureFresh(req.Context())
	if err != nil {
		return nil, err
	}

	req = req.Clone(req.Context())

	if t.userAgent != "" {
		req.Header.Set("User-Agent", t.userAgent)
	}

	// http.Client adds jar cookies before calling RoundTrip.
	// If we just refreshed the jar, the client missed them — add manually.
	if refreshed {
		for _, c := range t.jar.Cookies(req.URL) {
			req.AddCookie(c)
		}
	}

	return t.base.RoundTrip(req)
}

func (t *transport) ensureFresh(ctx context.Context) (bool, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if time.Since(t.lastFetch) < t.opts.cacheTTL {
		return false, nil
	}

	ua, cookies, err := t.fetchFromCDP(ctx)
	if err != nil {
		// Chrome is down but we have cached data — use stale cache
		// and retry on the next request.
		if !t.lastFetch.IsZero() {
			return false, nil
		}
		return false, err
	}

	t.userAgent = ua
	t.setCookies(cookies)
	t.lastFetch = time.Now()
	return true, nil
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
