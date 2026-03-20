package chttp_test

import (
	"encoding/json"
	"fmt"

	"github.com/xtdlib/chttp"
)

func Example() {
	cli := chttp.NewClient("ws://localhost:9222")
	resp, err := cli.Get("https://httpbin.org/cookies")
	_ = resp
	if err != nil {
		panic(err)
	}

	cookiesResponse := &httpBinResponse{}
	err = json.NewDecoder(resp.Body).Decode(cookiesResponse)
	if err != nil {

	}
	for k, v := range cookiesResponse.Cookies {
		fmt.Println(k, v)
	}
}

type httpBinResponse struct {
	Cookies map[string]string `json:"cookies"`
}
