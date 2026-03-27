package chttp

import (
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"
)

func browserNavigate(t *testing.T, url string) {
	t.Helper()
	ctx := context.Background()
	cdp, err := createCDPClient(ctx, "ws://localhost:9222")
	if err != nil {
		t.Fatal(err)
	}
	defer cdp.Close()
	if err := cdp.navigate(ctx, url); err != nil {
		t.Fatal(err)
	}
}

func TestBasicClient(t *testing.T) {
	// set cookie via browser
	browserNavigate(t, "https://httpbin.org/cookies/set/x/y")
	t.Cleanup(func() {
		browserNavigate(t, "https://httpbin.org/cookies/delete?x")
	})

	cli := NewClient("ws://localhost:9222", WithCookieTimeout(1*time.Second))

	{
		resp, err := cli.Get("https://httpbin.org/cookies")
		if err != nil {
			t.Fatal(err)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}

		var httpResp httpBinResponse
		if err := json.Unmarshal(body, &httpResp); err != nil {
			t.Fatal(err)
		}

		if httpResp.Cookies["x"] != "y" {
			t.Fatalf("expected cookie x=y, got %v", httpResp.Cookies)
		}
	}

	browserNavigate(t, "https://httpbin.org/cookies/set/x/z")
	time.Sleep(time.Second * 2)

	{
		resp, err := cli.Get("https://httpbin.org/cookies")
		if err != nil {
			t.Fatal(err)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}

		var httpResp httpBinResponse
		if err := json.Unmarshal(body, &httpResp); err != nil {
			t.Fatal(err)
		}

		if httpResp.Cookies["x"] != "z" {
			t.Fatalf("expected cookie x=y, got %v", httpResp.Cookies)
		}
		t.Log(httpResp.Cookies["x"])
	}
}

type httpBinResponse struct {
	Cookies map[string]string `json:"cookies"`
}

func TestRestart(t *testing.T) {
	cli := NewClient("", WithCookieTimeout(1*time.Second))

	{
		resp, err := cli.Get("https://httpbin.org/cookies")
		if err != nil {
			t.Fatal(err)
		}

		// read all body and log with t.Log
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}

		// parse body to httpBinResponse
		var httpResp httpBinResponse
		err = json.Unmarshal(body, &httpResp)
		if err != nil {
			t.Fatal(err)
		}

		found := false
		for k, v := range httpResp.Cookies {
			if k == "x" && v == "y" {
				t.Logf("cookie: %s=%s", k, v)
				found = true
			}
		}

		if !found {
			t.Fatal("cookie x=y not found")
		}
	}

	time.Sleep(time.Second * 10)

	{
		resp, err := cli.Get("https://httpbin.org/cookies")
		if err != nil {
			t.Fatal(err)
		}

		// read all body and log with t.Log
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}

		// parse body to httpBinResponse
		var httpResp httpBinResponse
		err = json.Unmarshal(body, &httpResp)
		if err != nil {
			t.Fatal(err)
		}

		found := false
		for k, v := range httpResp.Cookies {
			if k == "x" && v == "z" {
				t.Logf("cookie: %s=%s", k, v)
				found = true
			}
		}

		if !found {
			t.Fatal("cookie x=y not found")
		}
	}
}
