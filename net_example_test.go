package dnscache

import (
	"net/http"
	"time"

	"go.uber.org/zap"
)

func ExampleDialFunc() {
	resolver, _ := New(3*time.Second, 5*time.Second, zap.NewNop())

	// You can create a HTTP client which selects an IP from dnscache
	// ramdomly and dial it.
	client := http.Client{
		Transport: &http.Transport{
			DialContext: DialFunc(resolver, nil),
		},
	}

	// Do what you want.
	_ = client
}
