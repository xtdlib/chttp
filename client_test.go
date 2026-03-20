package chttp

import (
	"encoding/json"
	"io"
	"testing"
	"time"
)

func TestBasicClient(t *testing.T) {
	cli := NewClient("")
	resp, err := cli.Get("https://httpbin.org/cookies")
	_ = resp
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

type httpBinResponse struct {
	Cookies map[string]string `json:"cookies"`
}

func TestRestart(t *testing.T) {
	cli := NewClient("", WithCookieTimeout(1 * time.Second))

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
