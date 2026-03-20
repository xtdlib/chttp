package chttp

import (
	"context"
	"testing"
)

func TestDebug(t *testing.T) {
	ctx := context.Background()

	cc, err := createCDPClient(ctx, "ws://localhost:9222")
	if err != nil {
		t.Fatal(err)
	}

	result, err := cc.execute(ctx, "Storage.getCookies", nil)
	if err != nil {
		t.Fatal(err)
	}

	t.Log(string(result))
}
